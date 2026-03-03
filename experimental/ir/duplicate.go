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
	"strings"

	"github.com/bufbuild/protocompile/experimental/internal/taxa"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/internal/ext/iterx"
	"github.com/bufbuild/protocompile/internal/ext/slicesx"
)

// DedupExportedSymbols takes a report and the given *[File]s and checks for duplicate
// exported symbols based on [FullName] across the files.
func DedupExportedSymbols(r *report.Report, files ...*File) {
	all := slicesx.MergeKeySeq(
		iterx.Chain(slicesx.Map(files, func(file *File) symtab {
			return file.exported
		})),

		func(which int, elem Ref[Symbol]) FullName {
			file := files[which]
			return GetRef(file, elem).FullName()
		},

		func(which int, elem Ref[Symbol]) Symbol {
			src := files[which]
			sym := GetRef(src, elem)
			// We ignore package declarations, since the same package declaration could be made
			// multiple times.
			if sym.Kind() == SymbolKindPackage {
				return Symbol{}
			}
			return sym
		},
	)

	slices.SortStableFunc(all, func(a, b Symbol) int {
		return strings.Compare(string(a.FullName()), string(b.FullName()))
	})

	slicesx.DedupKey(
		all,
		func(s Symbol) FullName { return s.FullName() },
		func(symbols []Symbol) Symbol {
			if len(symbols) > 1 && !symbols[0].IsZero() {
				r.Error(errDuplicates{symbols: symbols})
			}
			return symbols[0]
		},
	)
}

// DedupExtensionTags takes a report and the given *[File]s and checks for duplicate
// tags for the same extendee. This ensures that a single tag is only used in a single
// extension across the given files.
func DedupExtensions(r *report.Report, files ...*File) {
	extendeeToTagToMembers := map[FullName]map[int32][]Member{}

	for _, file := range files {
		for extn := range seq.Values(file.AllExtensions()) {
			extendee := extn.Container().FullName()
			tagsToMembers := extendeeToTagToMembers[extendee]

			if tagsToMembers == nil {
				extendeeToTagToMembers[extendee] = map[int32][]Member{
					extn.Number(): {extn},
				}
				continue
			}

			members := tagsToMembers[extn.Number()]
			tagsToMembers[extn.Number()] = append(members, extn)
		}
	}

	// Walk the index and handle the extension duplicates here
	for extendee, tagToMembers := range extendeeToTagToMembers {
		for tag, members := range tagToMembers {
			if len(members) == 1 {
				continue
			}
			r.Error(errExtensionTagDuplicates{extns: members, extendee: extendee, tag: tag})
		}
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

	options := []report.DiagnosticOption{
		report.Message("%v `%v` used in more than one extension for `%v`", what, e.tag, e.extendee),
		report.Snippet(e.extns[0].AST()),
	}

	options = append(
		options,
		slices.Collect(slicesx.Map(e.extns[1:], func(extn Member) report.DiagnosticOption {
			return report.Snippetf(extn.AST(), "`%d` also used here", e.tag)
		}))...,
	)

	d.Apply(
		options...,
	)
}
