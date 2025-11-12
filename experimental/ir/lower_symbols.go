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
	"github.com/bufbuild/protocompile/experimental/id"
	"github.com/bufbuild/protocompile/experimental/internal/taxa"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/internal/arena"
	"github.com/bufbuild/protocompile/internal/ext/cmpx"
	"github.com/bufbuild/protocompile/internal/ext/iterx"
	"github.com/bufbuild/protocompile/internal/ext/mapsx"
	"github.com/bufbuild/protocompile/internal/ext/slicesx"
	"github.com/bufbuild/protocompile/internal/intern"
)

// buildLocalSymbols allocates new symbols for each definition in this file,
// and places them in the local symbol table.
func buildLocalSymbols(file *File) {
	sym := file.arenas.symbols.NewCompressed(rawSymbol{
		kind: SymbolKindPackage,
		fqn:  file.InternedPackage(),
	})
	file.exported = append(file.exported, Ref[Symbol]{id: id.ID[Symbol](sym)})

	for ty := range seq.Values(file.AllTypes()) {
		newTypeSymbol(ty)
		for f := range seq.Values(ty.Members()) {
			newFieldSymbol(f)
		}
		for f := range seq.Values(ty.Extensions()) {
			newFieldSymbol(f)
		}
		for o := range seq.Values(ty.Oneofs()) {
			newOneofSymbol(o)
		}
	}
	for f := range seq.Values(file.Extensions()) {
		newFieldSymbol(f)
	}
	for s := range seq.Values(file.Services()) {
		newServiceSymbol(s)
		for m := range seq.Values(s.Methods()) {
			newMethodSymbol(m)
		}
	}

	file.exported.sort(file)
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
		data: arena.Untyped(c.arenas.types.Compress(ty.Raw())),
	})
	c.exported = append(c.exported, Ref[Symbol]{id: id.ID[Symbol](sym)})
}

func newFieldSymbol(f Member) {
	c := f.Context()
	kind := SymbolKindField
	if !f.Extend().IsZero() {
		kind = SymbolKindExtension
	} else if f.AST().Classify() == ast.DefKindEnumValue {
		kind = SymbolKindEnumValue
	}
	sym := c.arenas.symbols.NewCompressed(rawSymbol{
		kind: kind,
		fqn:  f.InternedFullName(),
		data: arena.Untyped(c.arenas.members.Compress(f.Raw())),
	})
	c.exported = append(c.exported, Ref[Symbol]{id: id.ID[Symbol](sym)})
}

func newOneofSymbol(o Oneof) {
	c := o.Context()
	sym := c.arenas.symbols.NewCompressed(rawSymbol{
		kind: SymbolKindOneof,
		fqn:  o.InternedFullName(),
		data: arena.Untyped(c.arenas.oneofs.Compress(o.Raw())),
	})
	c.exported = append(c.exported, Ref[Symbol]{id: id.ID[Symbol](sym)})
}

func newServiceSymbol(s Service) {
	c := s.Context()
	sym := c.arenas.symbols.NewCompressed(rawSymbol{
		kind: SymbolKindService,
		fqn:  s.InternedFullName(),
		data: arena.Untyped(c.arenas.services.Compress(s.Raw())),
	})
	c.exported = append(c.exported, Ref[Symbol]{id: id.ID[Symbol](sym)})
}

func newMethodSymbol(m Method) {
	c := m.Context()
	sym := c.arenas.symbols.NewCompressed(rawSymbol{
		kind: SymbolKindMethod,
		fqn:  m.InternedFullName(),
		data: arena.Untyped(c.arenas.methods.Compress(m.Raw())),
	})
	c.exported = append(c.exported, Ref[Symbol]{id: id.ID[Symbol](sym)})
}

// mergeImportedSymbolTables builds a symbol table of every imported symbol.
//
// It also enhances the exported symbol table with the exported symbols of each
// public import.
func mergeImportedSymbolTables(file *File, r *report.Report) {
	imports := file.Imports()

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
		file.exported = symtabMerge(
			file,
			iterx.Chain(
				iterx.Of(file.exported),
				seq.Map(imports, func(i Import) symtab {
					if !i.Public {
						// Return an empty symbol table so that the table to
						// context mapping can still be an array index.
						return symtab{}
					}
					return i.exported
				}),
			),
			func(i int) *File {
				if i == 0 {
					return file
				}
				return file.imports.files[i-1].file
			},
		)
	}

	// Form the imported symbol table from the exports list by adding all of
	// the non-public imports.
	file.imported = symtabMerge(
		file,
		iterx.Chain(
			iterx.Of(file.exported),
			seq.Map(imports, func(i Import) symtab {
				if i.Public {
					// Already processed in the loop above.
					return symtab{}
				}
				return i.exported
			}),
		),
		func(i int) *File {
			if i == 0 {
				return file
			}
			return file.imports.files[i-1].file
		},
	)

	dedupSymbols(file, &file.exported, nil)
	dedupSymbols(file, &file.imported, r)
}

// dedupSymbols diagnoses duplicate symbols in a sorted symbol table, and
// deletes the duplicates.
//
// Which duplicate is chosen for deletion is deterministic: ties are broken
// according to file names and span starts, in that order. This avoids
// non-determinism around how intern IDs are assigned to names.
func dedupSymbols(file *File, symbols *symtab, r *report.Report) {
	*symbols = slicesx.DedupKey(
		*symbols,
		func(r Ref[Symbol]) intern.ID { return GetRef(file, r).InternedFullName() },
		func(refs []Ref[Symbol]) Ref[Symbol] {
			if len(refs) == 1 {
				return refs[0]
			}

			slices.SortFunc(refs, cmpx.Map(
				func(r Ref[Symbol]) Symbol { return GetRef(file, r) },
				cmpx.Key(Symbol.Kind), // Packages sort first, reserved names sort last.
				cmpx.Key(func(s Symbol) string {
					// NOTE: we do not choose a winner based on the path's intern
					// ID, because that is non-deterministic!
					return s.Context().Path()
				}),
				// Break ties with whichever came first in the file.
				cmpx.Key(func(s Symbol) int { return s.Definition().Start }),
			))

			types := mapsx.CollectSet(iterx.FilterMap(slices.Values(refs), func(r Ref[Symbol]) (ast.DeclDef, bool) {
				s := GetRef(file, r)
				ty := s.AsType()
				return ty.AST(), !ty.IsZero()
			}))
			isFirst := true
			refs = slices.DeleteFunc(refs, func(r Ref[Symbol]) bool {
				s := GetRef(file, r)
				if !isFirst && !s.AsMember().Container().MapField().IsZero() {
					// Ignore all symbols that are map entry fields, because those
					// can only be duplicated when two map entry messages' names
					// collide, so diagnosing them just creates a mess.
					return true
				}
				if !isFirst && s.AsMember().IsGroup() && mapsx.Contains(types, s.AsMember().AST()) {
					// If a group field collides with its own message type, remove it;
					// groups with names that might collide with their fields are already
					// diagnosed in the parser.
					return true
				}
				if !isFirst && s.Kind() == SymbolKindPackage {
					// Ignore all refs that are packages except for the first one. This
					// is because a package can be defined in multiple files.
					return true
				}

				isFirst = false
				return false
			})

			// Deduplicate references to the same element.
			refs = slicesx.Dedup(refs)
			if len(refs) > 1 && r != nil {
				r.Error(errDuplicates{file, refs})
			}

			return refs[0]
		},
	)
}

// errDuplicates diagnoses duplicate symbols.
type errDuplicates struct {
	*File
	refs []Ref[Symbol]
}

func (e errDuplicates) symbol(n int) Symbol {
	return GetRef(e.File, e.refs[n])
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

	spans := make(map[source.Span]struct{})
	for i := range e.refs[2:] {
		s := e.symbol(i + 2)
		next := s.Kind().noun()

		span := s.Definition()
		if !mapsx.AddZero(spans, span) && i > 1 {
			// Avoid overlapping spans.
			continue
		}

		if noun != next {
			d.Apply(report.Snippetf(span,
				"...and then here as a %s %s", article(next), next))
			noun = next
		} else {
			d.Apply(report.Snippetf(span, "...and here"))
		}
	}

	// If at least one duplicated symbol is non-visible, explain
	// that symbol names are global!
	for i := range e.refs {
		s := e.symbol(i)
		if s.Visible(e.File) {
			continue
		}

		d.Apply(report.Helpf(
			"symbol names must be unique across all transitive imports; "+
				"for example, %q declares `%s` but is not directly imported",
			s.Context().Path(),
			first.FullName(),
		))
		break
	}

	// If at least one of them was an enum value, we note the weird language
	// bug with enum scoping.
	for i := range e.refs {
		s := e.symbol(i)
		v := s.AsMember()
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

	for i := range e.refs {
		ty := e.symbol(i).AsType()
		if mf := ty.MapField(); !mf.IsZero() {
			d.Apply(
				report.Snippetf(mf.AST().Name(), "implies `repeated %s`", ty.Name()),
				report.Helpf(
					"map-typed fields implicitly declare a nested message type: "+
						"field `%s` produces a map entry type `%s`",
					mf.Name(), ty.Name(),
				),
			)
			break
		}
	}
}
