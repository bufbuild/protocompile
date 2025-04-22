// Copyright 2020-2025 Buf Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ir

import (
	"fmt"
	"slices"

	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/internal/taxa"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/internal/arena"
	"github.com/bufbuild/protocompile/internal/ext/cmpx"
	"github.com/bufbuild/protocompile/internal/ext/iterx"
	"github.com/bufbuild/protocompile/internal/ext/slicesx"
	"github.com/bufbuild/protocompile/internal/intern"
)

// buildLocalSymbols allocates new symbols for each definition in this file,
// and places them in the local symbol table.
func buildLocalSymbols(f File) {
	c := f.Context()

	sym := c.arenas.symbols.NewCompressed(rawSymbol{
		kind: SymbolKindPackage,
		fqn:  f.InternedPackage(),
	})
	c.exported = append(c.exported, ref[rawSymbol]{ptr: sym})

	for ty := range seq.Values(f.AllTypes()) {
		newTypeSymbol(ty)
		for f := range seq.Values(ty.Fields()) {
			newFieldSymbol(f)
		}
		for f := range seq.Values(ty.Extensions()) {
			newFieldSymbol(f)
		}
		for o := range seq.Values(ty.Oneofs()) {
			newOneofSymbol(o)
		}
	}
	for f := range seq.Values(f.Extensions()) {
		newFieldSymbol(f)
	}

	c.exported.sort(c)
}

func newTypeSymbol(ty Type) {
	c := ty.Context()
	kind := SymbolKindMessage
	if ty.IsEnum() {
		kind = SymbolKindEnum
	}
	sym := c.arenas.symbols.NewCompressed(rawSymbol{
		kind: kind,
		fqn:  ty.InternedFullName(),
		data: arena.Untyped(c.arenas.types.Compress(ty.raw)),
	})
	c.exported = append(c.exported, ref[rawSymbol]{ptr: sym})
}

func newFieldSymbol(f Field) {
	c := f.Context()
	kind := SymbolKindField
	if !f.raw.extendee.Nil() {
		kind = SymbolKindExtension
	} else if f.AST().Classify() == ast.DefKindEnumValue {
		kind = SymbolKindEnumValue
	}
	sym := c.arenas.symbols.NewCompressed(rawSymbol{
		kind: kind,
		fqn:  f.InternedFullName(),
		data: arena.Untyped(c.arenas.fields.Compress(f.raw)),
	})
	c.exported = append(c.exported, ref[rawSymbol]{ptr: sym})
}

func newOneofSymbol(o Oneof) {
	c := o.Context()
	sym := c.arenas.symbols.NewCompressed(rawSymbol{
		kind: SymbolKindOneof,
		fqn:  o.InternedFullName(),
		data: arena.Untyped(c.arenas.oneofs.Compress(o.raw)),
	})
	c.exported = append(c.exported, ref[rawSymbol]{ptr: sym})
}

// mergeImportedSymbolTables builds a symbol table of every imported symbol.
//
// It also enhances the exported symbol table with the exported symbols of each
// public import.
func mergeImportedSymbolTables(f File, r *report.Report) {
	imports := f.Imports()

	var havePublic bool
	for sym := range seq.Values(imports) {
		if sym.Public {
			havePublic = true
			break
		}
	}

	// Form the exported symbol table from the public imports. Not necessary
	// if there are no public imports.
	if havePublic {
		f.Context().exported = symtabMerge(
			f.Context(),
			iterx.Chain(
				iterx.Of(f.Context().exported),
				seq.Map(imports, func(i Import) symtab {
					if !i.Public {
						// Return an empty symbol table so that the table to
						// context mapping can still be an array index.
						return symtab{}
					}
					return i.Context().exported
				}),
			),
			func(i int) File {
				if i == 0 {
					return f
				}
				return f.Context().imports.files[i-1]
			},
		)
	}

	// Form the imported symbol table from the exports list by adding all of
	// the non-public imports.
	f.Context().imported = symtabMerge(
		f.Context(),
		iterx.Chain(
			iterx.Of(f.Context().exported),
			seq.Map(imports, func(i Import) symtab {
				if i.Public {
					// Already processed in the loop above.
					return symtab{}
				}
				return i.Context().exported
			}),
		),
		func(i int) File {
			if i == 0 {
				return f
			}
			return f.Context().imports.files[i-1]
		},
	)

	dedupSymbols(f, &f.Context().exported, nil)
	dedupSymbols(f, &f.Context().imported, r)
}

// dedupSymbols diagnoses duplicate symbols in a sorted symbol table, and
// deletes the duplicates.
//
// Which duplicate is chosen for deletion is deterministic: ties are broken
// according to file names and span starts, in that order. This avoids
// non-determinism around how intern IDs are assigned to names.
func dedupSymbols(f File, symbols *symtab, r *report.Report) {
	*symbols = slicesx.DedupKey(
		*symbols,
		func(r ref[rawSymbol]) intern.ID { return wrapSymbol(f.Context(), r).InternedFullName() },
		func(refs []ref[rawSymbol]) ref[rawSymbol] {
			if len(refs) == 1 {
				return refs[0]
			}

			slices.SortFunc(refs, cmpx.Map(
				func(r ref[rawSymbol]) Symbol { return wrapSymbol(f.Context(), r) },
				cmpx.Key(Symbol.Kind), // Packages sort first, reserved names sort last.
				cmpx.Key(func(s Symbol) string {
					// NOTE: we do not choose a winner based on the path's intern
					// ID, because that is non-deterministic!
					return s.File().Path()
				}),
				// Break ties with whichever came first in the file.
				cmpx.Key(func(s Symbol) int { return s.Definition().Start }),
			))

			// Ignore all refs that are packages except for the first one. This
			// is because a package can be defined in multiple files.
			isFirst := true
			refs = slices.DeleteFunc(refs, func(r ref[rawSymbol]) bool {
				pkg := wrapSymbol(f.Context(), r).Kind() == SymbolKindPackage
				if isFirst {
					isFirst = false
					return false
				}
				return pkg
			})

			// Deduplicate references to the same element.
			refs = slicesx.Dedup(refs)
			if len(refs) > 1 && r != nil {
				r.Error(errDuplicates{f.Context(), refs})
			}

			return refs[0]
		},
	)
}

// errDuplicates diagnoses duplicate symbols.
type errDuplicates struct {
	*Context
	refs []ref[rawSymbol]
}

func (e errDuplicates) symbol(n int) Symbol {
	return wrapSymbol(e.Context, e.refs[n])
}

func (e errDuplicates) Diagnose(d *report.Diagnostic) {
	var havePkg bool
	for i := range e.refs {
		if e.symbol(i).Kind() == SymbolKindPackage {
			havePkg = true
			break
		}
	}

	first, second := e.symbol(0), e.symbol(1)

	name := first.FullName()
	if !havePkg {
		name = FullName(name.Name())
	}

	var inParent string
	if !havePkg && name.Parent() != "" {
		inParent = fmt.Sprintf(" in `%s`", name.Parent())
	}

	// TODO: In the diagnostic construction code below, we can wind up
	// saying nonsense like "a enum". Currently, we chose the article
	// based on whether the noun starts with an e, but this is really
	// icky.
	article := func(n taxa.Noun) string {
		if n.String()[0] == 'e' {
			return "an"
		}
		return "a"
	}

	noun := first.Kind().noun()
	d.Apply(
		report.Message("`%s` declared multiple times%s", name, inParent),
		report.Snippetf(first.Definition(),
			"first here, as %s %s",
			article(noun), noun),
	)

	if next := second.Kind().noun(); next != noun {
		d.Apply(report.Snippetf(second.Definition(),
			"...also declared here, now as %s %s", article(next), next))
		noun = next
	} else {
		d.Apply(report.Snippetf(second.Definition(),
			"...also declared here"))
	}

	for i := range e.refs[2:] {
		s := e.symbol(i + 2)
		next := s.Kind().noun()

		if noun != next {
			d.Apply(report.Snippetf(s.Definition(),
				"...and then here as a %s %s", article(next), next))
			noun = next
		} else {
			d.Apply(report.Snippetf(s.Definition(),
				"...and here"))
		}
	}

	// If at least one duplicated symbol is non-visible, explain
	// that symbol names are global!
	for i := range e.refs {
		s := e.symbol(i)
		if s.Visible() {
			continue
		}

		d.Apply(report.Helpf(
			"symbol names must be unique across all transitive imports; "+
				"for example, %q declares `%s` but is not directly imported",
			s.File().Path(),
			first.FullName(),
		))
		break
	}

	// If at least one of them was an enum value, we note the weird language
	// bug with enum scoping.
	for i := range e.refs {
		s := e.symbol(i)
		v := s.AsField()
		if !v.Container().IsEnum() {
			continue
		}

		enum := v.Container()

		// Avoid unreasonably-nested names where reasonable.
		parentName := enum.FullName().Parent()
		if parent := enum.Parent(); !parent.IsZero() {
			parentName = FullName(parent.Name())
		}

		d.Apply(report.Helpf(
			"the fully-qualified names of enum values do not include the name of the enum; "+
				"`%[3]s` defined inside of enum `%[1]s.%[2]s` has the name `%[1]s.%[3]s`, "+
				"not `%[1]s.%[2]s.%[3]s`",
			parentName,
			enum.Name(),
			v.Name(),
		))
	}
}
