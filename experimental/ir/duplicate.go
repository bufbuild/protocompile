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

	"github.com/bufbuild/protocompile/experimental/internal/taxa"
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

	outer:
		for i, sym := range symbols {
			for j, prev := range symbols[:i] {
				if sym.Context() == prev.Context() || !sym.Context().ImportFor(prev.Context()).Decl.IsZero() {
					// Need to zero out everything between here and i
					for x := j + 1; x < i+1; x++ {
						symbols[x] = Symbol{}
					}
					break outer
				}
			}
		}

		symbols = slices.DeleteFunc(symbols, Symbol.IsZero)

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
		r.Error(errExtensionTagDuplicates{extns: members, extendee: k.typ.FullName(), tag: k.number})
	}
}

type errExtensionTagDuplicates struct {
	extns    []Member
	extendee FullName
	tag      int32
}

func (e errExtensionTagDuplicates) Diagnose(d *report.Diagnostic) {
	what := taxa.FieldNumber
	if e.extns[0].Container().IsEnum() {
		what = taxa.EnumValue
	}

	d.Apply(
		report.Message("%v `%v` used in more than one extension for `%v`", what, e.tag, e.extendee),
		report.Snippet(e.extns[0].AST()),
	)

	for _, extn := range e.extns[1:] {
		d.Apply(report.Snippetf(extn.AST(), "`%d` also used here", e.tag))
	}
}
