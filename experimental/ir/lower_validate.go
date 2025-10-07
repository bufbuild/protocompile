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
	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/ast/syntax"
	"github.com/bufbuild/protocompile/experimental/internal/taxa"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/experimental/token/keyword"
	"github.com/bufbuild/protocompile/internal/ext/iterx"
)

// diagnoseUnusedImports generates diagnostics for each unused import.
func diagnoseUnusedImports(f File, r *report.Report) {
	for imp := range seq.Values(f.Imports()) {
		if imp.Used {
			continue
		}

		r.Warnf("unused import \"%s\"", f.Path()).Apply(
			report.Snippet(imp.Decl.ImportPath()),
			report.SuggestEdits(imp.Decl, "delete it", report.Edit{
				Start: 0, End: imp.Decl.Span().Len(),
			}),
			report.Helpf("no symbols from this file are referenced"),
		)
	}
}

// validateConstraints validates miscellaneous constraints that depend on the
// whole IR being constructed properly.
func validateConstraints(f File, r *report.Report) {
	for ty := range seq.Values(f.AllTypes()) {
		if ty.IsMessageSet() {
			validateMessageSet(ty, r)
		}
	}

	for m := range f.AllMembers() {
		// NOTE: extensions already cannot be map fields, so we don't need to
		// validate them.
		if m.IsExtension() && !m.IsMap() {
			extendee := m.Container()
			if extendee.IsMessageSet() {
				validateMessageSetExtension(m, r)
			}
		}
	}
}

func validateMessageSet(ty Type, r *report.Report) {
	if !ty.IsMessageSet() {
		return
	}

	f := ty.Context().File()
	builtins := ty.Context().builtins()

	if f.Syntax() == syntax.Proto3 {
		r.Errorf("%s are not supported", taxa.MessageSet).Apply(
			report.Snippetf(ty.Options().Field(builtins.MessageSet).KeyAST(), "declared as message set here"),
			report.Snippet(ty.AST().Stem()),
			report.PageBreak,
			report.Snippetf(f.AST().Syntax().Value(), "\"proto3\" specified here"),
			report.Helpf("%ss cannot be defined in \"proto3\" only", taxa.MessageSet),
			report.Helpf("%ss are not implemented correctly in most Protobuf implementations", taxa.MessageSet),
		)
		return
	}

	ok := true

	for member := range seq.Values(ty.Members()) {
		ok = false
		r.Errorf("field declared in %s `%s`", taxa.MessageSet, ty.FullName()).Apply(
			report.Snippet(member.AST()),
			report.PageBreak,
			report.Snippet(ty.AST().Stem()),
			report.Snippetf(ty.Options().Field(builtins.MessageSet).KeyAST(), "declared as message set here"),
			report.Helpf("message set types may only declare extension ranges"),
		)
	}

	for oneof := range seq.Values(ty.Oneofs()) {
		ok = false
		r.Errorf("field declared in %s `%s`", taxa.MessageSet, ty.FullName()).Apply(
			report.Snippet(oneof.AST()),
			report.PageBreak,
			report.Snippetf(ty.Options().Field(builtins.MessageSet).KeyAST(), "declared as message set here"),
			report.Snippet(ty.AST().Stem()),
			report.Helpf("message set types may only declare extension ranges"),
		)
	}

	if ty.ExtensionRanges().Len() == 0 {
		ok = false
		r.Errorf("%s `%s` declares no %ss", taxa.MessageSet, ty.FullName(), taxa.Extensions).Apply(
			report.Snippetf(ty.Options().Field(builtins.MessageSet).KeyAST(), "declared as message set here"),
			report.Snippet(ty.AST().Stem()),
		)
	}

	if ok {
		r.Warnf("%ss are deprecated", taxa.MessageSet).Apply(
			report.Snippetf(ty.Options().Field(builtins.MessageSet).KeyAST(), "declared as message set here"),
			report.Snippet(ty.AST().Stem()),
			report.Helpf("%ss are not implemented correctly in most Protobuf implementations", taxa.MessageSet),
		)
	}
}

func validateMessageSetExtension(extn Member, r *report.Report) {
	builtins := extn.Context().builtins()
	extendee := extn.Container()
	if extn.IsRepeated() {
		_, repeated := iterx.Find(extn.AST().Type().Prefixes(), func(ty ast.TypePrefixed) bool {
			return ty.Prefix() == keyword.Repeated
		})

		r.Errorf("repeated message set extension").Apply(
			report.Snippet(repeated.PrefixToken()),
			report.PageBreak,
			report.Snippetf(extendee.Options().Field(builtins.MessageSet).KeyAST(), "declared as message set here"),
			report.Snippet(extendee.AST().Stem()),
			report.Helpf("message set extensions must be singular message fields"),
		)
	}

	if !extn.Element().IsMessage() {
		r.Errorf("non-message message set extension").Apply(
			report.Snippet(extn.AST().Type().RemovePrefixes()),
			report.PageBreak,
			report.Snippetf(extendee.Options().Field(builtins.MessageSet).KeyAST(), "declared as message set here"),
			report.Snippet(extendee.AST().Stem()),
			report.Helpf("message set extensions must be singular message fields"),
		)
	}
}
