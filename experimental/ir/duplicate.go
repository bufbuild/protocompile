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
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/seq"
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
	nameToSymbols := map[FullName][]Symbol{}

	for _, file := range files {
	symCheck:
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

			// To avoid unnecessary diagnostics, we recursively check that the parent of the
			// current symbol is not already duplicated. As the docs for [symtab] indicate, the
			// symbol tables are sorted by the [intern.ID] of their FQN during the lowering step,
			// so any duplication for a parent symbol will already have been found.
			parent := sym.FullName().Parent()
			for parent != "" {
				if len(nameToSymbols[parent]) > 1 {
					break symCheck
				}
				parent = parent.Parent()
			}
			nameToSymbols[sym.FullName()] = append(nameToSymbols[sym.FullName()], sym)
		}
	}

	for _, symbols := range nameToSymbols {
		if len(symbols) == 1 {
			continue
		}

		if len(symbols) > 1 {
			r.Error(errDuplicates{symbols: symbols})
		}
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
			// First check if this extension number is already the number of a non-extension
			// member of the container type. If so, then there is already a diagnostic for that
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
