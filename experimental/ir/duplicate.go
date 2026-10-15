// Copyright 2020-2026 Buf Technologies, Inc.
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
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/internal/intern"
)

// DedupExportedSymbols takes a report and the given *[File]s and checks for duplicate
// exported symbols based on [FullName] across the files.
//
// Diagnostics are provided only for the highest level of duplication. For example, given
// the following files and content:
//
//	-- a1.proto --
//	syntax = "proto3";
//	package a;
//
//	message Foo {
//	 optional string bar = 1;
//	}
//
//	-- a2.proto --
//	syntax = "proto3";
//	package a;
//
//	message Foo {
//	 optional string bar = 1;
//	}
//
// A diagnostic for the duplication of `a.Foo` would be surfaced, but it would be redundant
// to surface diagnostics for `a.Foo.bar`..
func DedupExportedSymbols(r *report.Report, files ...*File) {
	nameToSymbols := map[intern.ID][]Symbol{}
	var names []intern.ID

	for _, file := range files {
		for sym := range seq.Values(file.ExportedSymbols()) {
			// We ignore package declarations, since the same package declaration could be made
			// multiple times.
			if sym.Kind() == SymbolKindPackage {
				continue
			}

			// We also ignore exported symbols from public imports.
			if sym.Context() != file {
				continue
			}

			fqn := sym.InternedFullName()
			if _, ok := nameToSymbols[fqn]; !ok {
				names = append(names, fqn)
			}
			nameToSymbols[fqn] = append(nameToSymbols[fqn], sym)
		}
	}

	for _, fqn := range names {
		symbols := nameToSymbols[fqn]
		if len(symbols) < 2 {
			continue
		}

		// Two declarations of a symbol imply two declarations of the type
		// enclosing it, so checking the immediate parent is sufficient to skip
		// every symbol beneath a duplicated one.
		if parent := enclosingName(symbols[0]); parent != 0 && len(nameToSymbols[parent]) > 1 {
			continue
		}

		r.Error(errDuplicates{symbols: symbols})
	}
}

// enclosingName returns the full name of the declaration that syntactically
// encloses sym, or zero if it is declared at the top level of its file.
func enclosingName(sym Symbol) intern.ID {
	switch {
	case sym.Kind().IsType():
		return sym.AsType().Parent().InternedFullName()
	case sym.Kind().IsMember():
		return sym.AsMember().Parent().InternedFullName()
	case sym.Kind() == SymbolKindOneof:
		return sym.AsOneof().Container().InternedFullName()
	case sym.Kind() == SymbolKindMethod:
		return sym.AsMethod().Service().InternedFullName()
	default:
		return 0
	}
}

// DedupExtensionTags takes a report and the given *[File]s and checks for duplicate
// tags for the same extendee. This ensures that a single tag is only used in a single
// extension across the given files.
func DedupExtensions(r *report.Report, files ...*File) {
	type key struct {
		typ    Type
		number int32
	}
	keyToMembers := make(map[key][]Member)

	for _, file := range files {
		for extn := range seq.Values(file.AllExtensions()) {
			// Skip numbers that had errors during evaluation.
			if !extn.numberOK() {
				continue
			}

			// Check if this extension number is already the number of a non-extension member
			// of the container type. If so, then there is already a diagnostic for that
			// overlap and we don't need to surface an additional diagnostic here.
			existing := extn.Container().MemberByNumber(extn.Number())
			if !existing.IsZero() && !existing.IsExtension() {
				continue
			}

			k := key{
				typ:    extn.Container(),
				number: extn.Number(),
			}
			keyToMembers[k] = append(keyToMembers[k], extn)
		}
	}

	// Walk the index and handle the extension duplicates here
	for k, members := range keyToMembers {
		if len(members) == 1 {
			continue
		}

		first := members[0]
		for _, dupe := range members[1:] {
			r.Error(errOverlap{
				ty:     k.typ,
				first:  first.AsTagRange(),
				second: dupe.AsTagRange(),
			})
		}
	}
}
