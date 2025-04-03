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

	sym := c.arenas.symbols.NewCompressed(rawSymbol{
		kind: SymbolKindPackage,
		fqn:  f.InternedPackage(),
	})
	c.symbols = append(c.symbols, ref[rawSymbol]{ptr: sym})

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

	c.symbols.sort(c)
	diagnoseDuplicates(f, &c.symbols, r)
}

func newTypeSymbol(ty Type) {
	c := ty.Context()
	sym := c.arenas.symbols.NewCompressed(rawSymbol{
		kind: SymbolKindType,
		fqn:  ty.InternedFullName(),
		data: arena.Untyped(c.arenas.types.Compress(ty.raw)),
	})
	c.symbols = append(c.symbols, ref[rawSymbol]{ptr: sym})
}

func newFieldSymbol(f Field) {
	c := f.Context()
	sym := c.arenas.symbols.NewCompressed(rawSymbol{
		kind: SymbolKindField,
		fqn:  f.InternedFullName(),
		data: arena.Untyped(c.arenas.fields.Compress(f.raw)),
	})
	c.symbols = append(c.symbols, ref[rawSymbol]{ptr: sym})
}

func newOneofSymbol(o Oneof) {
	c := o.Context()
	sym := c.arenas.symbols.NewCompressed(rawSymbol{
		kind: SymbolKindOneof,
		fqn:  o.InternedFullName(),
		data: arena.Untyped(c.arenas.oneofs.Compress(o.raw)),
	})
	c.symbols = append(c.symbols, ref[rawSymbol]{ptr: sym})
}

// buildImportedSymbols builds a symbol table of every symbol in every
// transitive import. This will contain symbols that are not visible to this
// file, but visibility can be tested with f.visibleImports.
func buildImportedSymbols(f File, r *report.Report) {
	// Only need to merge from the direct imports's tables, since each
	// import table is fully transitive.
	imports := f.Imports()

	f.Context().symbols = slicesx.MergeKeySeq(
		iterx.Chain(
			iterx.Of(f.Context().symbols),
			seq.Map(imports, func(i Import) symtab {
				return i.Context().symbols
			}),
		),

		func(which int, elem ref[rawSymbol]) intern.ID {
			c := f.Context()
			if which > 0 {
				// Need to make sure to use the correct context here for
				// scoping the lookup.
				c = imports.At(which - 1).Context()
			}

			return wrapSymbol(c, elem).InternedFullName()
		},

		func(which int, elem ref[rawSymbol]) ref[rawSymbol] {
			// We need top map the file number from src to the current one.
			if which > 0 {
				src := imports.At(which - 1)
				theirs := wrapSymbol(src.Context(), elem)
				ours := f.Context().imports.byPath[theirs.File().InternedPath()]
				elem.file = int32(ours + 1)
			}

			return elem
		},
	)

	diagnoseDuplicates(f, &f.Context().symbols, r)
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
				if isFirst {
					isFirst = false
					return false
				}
				return wrapSymbol(f.Context(), r).Kind() == SymbolKindPackage
			})

			// Deduplicate references to the same element.
			refs = slicesx.Dedup(refs)

			if len(refs) == 1 {
				return refs[0]
			}

			first := wrapSymbol(f.Context(), refs[0])
			second := wrapSymbol(f.Context(), refs[1])
			d := r.Errorf("`%s` declared multiple times", first.FullName()).Apply(
				report.Tag(tagSymbolRedefined),
				report.Snippetf(first.Definition(), "first encountered here"),
				report.Snippetf(second.Definition(), "...also declared here"),
			)
			for _, r := range refs[2:] {
				s := wrapSymbol(f.Context(), r)
				d.Apply(report.Snippetf(s.Definition(), "...and here"))
			}

			// If at least one of them was an enum value, we note the weird language
			// bug with enum scoping.
			idx := slices.IndexFunc(refs, func(r ref[rawSymbol]) bool {
				f := wrapSymbol(f.Context(), r).AsField()
				// NOTE: Can't use f.IsEnumValue() yet because types have not
				// yet been resolved.
				return !f.IsZero() && f.Container().IsEnum()
			})
			if idx != -1 {
				value := wrapSymbol(f.Context(), refs[idx]).AsField()
				enum := value.Container()
				d.Apply(report.Helpf(
					"the fully-qualified names of enum values do not include the name of the enum; "+
						"`%s` defined inside of enum `%s` has the name `%s`, not `%[2]s.%[1]s",
					value.Name(),
					enum.FullName(),
					value.FullName(),
				))
			}

			return refs[0]
		},
	)
}
