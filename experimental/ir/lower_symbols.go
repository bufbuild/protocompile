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
	"slices"

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
func buildLocalSymbols(f File, r *report.Report) {
	c := f.Context()
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

	c.symbols.locals.sort(c)
	diagnoseDuplicates(f, &c.symbols.locals, r)
}

func newTypeSymbol(ty Type) {
	c := ty.Context()
	sym := c.arenas.symbols.NewCompressed(rawSymbol{
		kind: SymbolKindType,
		fqn:  ty.InternedFullName(),
		data: arena.Untyped(c.arenas.types.Compress(ty.raw)),
	})
	c.symbols.locals = append(c.symbols.locals, ref[rawSymbol]{ptr: sym})
}

func newFieldSymbol(f Field) {
	c := f.Context()
	sym := c.arenas.symbols.NewCompressed(rawSymbol{
		kind: SymbolKindField,
		fqn:  f.InternedFullName(),
		data: arena.Untyped(c.arenas.fields.Compress(f.raw)),
	})
	c.symbols.locals = append(c.symbols.locals, ref[rawSymbol]{ptr: sym})
}

func newOneofSymbol(o Oneof) {
	c := o.Context()
	sym := c.arenas.symbols.NewCompressed(rawSymbol{
		kind: SymbolKindOneof,
		fqn:  o.InternedFullName(),
		data: arena.Untyped(c.arenas.oneofs.Compress(o.raw)),
	})
	c.symbols.locals = append(c.symbols.locals, ref[rawSymbol]{ptr: sym})
}

// buildImportedSymbols builds a symbol table of every symbol in every
// transitive import. This will contain symbols that are not visible to this
// file, but visibility can be tested with f.visibleImports.
func buildImportedSymbols(f File, r *report.Report) {
	imports := f.TransitiveImports()
	symbols := &f.Context().symbols
	symbols.imports = slicesx.MergeKeySeq(
		iterx.Chain(
			iterx.Of(f.Context().symbols.locals),
			seq.Map(imports, func(i Import) symtab {
				return i.Context().symbols.locals
			}),
		),
		func(which int, elem ref[rawSymbol]) intern.ID {
			elem.file = int32(which)
			return wrapSymbol(f.Context(), elem).InternedFullName()
		},
		func(which int, elem ref[rawSymbol]) ref[rawSymbol] {
			elem.file = int32(which)
			return elem
		},
	)
	diagnoseDuplicates(f, &symbols.imports, r)
}

// diagnoseDuplicates diagnoses duplicate symbols in a sorted symbol table, and
// deletes the duplicates.
//
// Which duplicate is chosen for deletion is deterministic: ties are broken
// according to file names and span starts, in that order. This avoids
// non-determinism around how intern IDs are assigned to names.
func diagnoseDuplicates(f File, symbols *symtab, r *report.Report) {
	*symbols = slicesx.DedupKey(
		*symbols,
		func(r ref[rawSymbol]) intern.ID {
			return wrapSymbol(f.Context(), r).InternedFullName()
		},
		func(refs []ref[rawSymbol]) ref[rawSymbol] {
			if len(refs) == 1 {
				return refs[0]
			}

			type entry struct {
				ref[rawSymbol]
				Symbol
			}

			// TODO: It is possible to avoid this copy, but doing so seems
			// quite painful.
			syms := slices.Collect(slicesx.Map(refs, func(r ref[rawSymbol]) entry {
				return entry{r, wrapSymbol(f.Context(), r)}
			}))

			slices.SortFunc(syms, cmpx.Join(
				cmpx.Key(entry.Kind), // Packages sort first, reserved names sort last.
				cmpx.Key(func(e entry) string {
					// NOTE: we do not choose a winner based on the path's intern
					// ID, because that is non-deterministic!
					return e.Context().File().Path()
				}),
				cmpx.Key(func(e entry) int { return e.Definition().Start }),
			))

			// If every element of syms is a package symbol, we don't diagnose.
			allPackages := !iterx.Every(slices.Values(syms), func(e entry) bool {
				return e.Kind() == SymbolKindPackage
			})

			if !allPackages {
				d := r.Errorf("`%s` declared multiple times", syms[0].FullName()).Apply(
					report.Tag(tagSymbolRedefined),
					report.Snippetf(syms[0].Definition(), "first encountered here"),
					report.Snippetf(syms[1].Definition(), "...also declared here"),
				)
				for _, e := range syms[2:] {
					d.Apply(report.Snippetf(e.Definition(), "...and here"))
				}

				// If at least one of them was an enum value, we note the weird language
				// bug with enum scoping.
				idx := slices.IndexFunc(syms, func(e entry) bool {
					f := e.AsField()
					if f.IsZero() {
						return false
					}
					return f.IsEnumValue()
				})
				if idx != -1 {
					value := syms[idx].AsField()
					enum := value.Container()
					d.Apply(report.Helpf(
						"the fully-qualified names of enum values do not include the name of the enum; "+
							"`%s` defined inside of enum `%s` has the name `%s`, not `%[2]s.%[1]s",
						value.Name(),
						enum.FullName(),
						value.FullName(),
					))
				}
			}

			return syms[0].ref
		},
	)
}
