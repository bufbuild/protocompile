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
	"github.com/bufbuild/protocompile/experimental/ir/presence"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/experimental/token/keyword"
	"github.com/bufbuild/protocompile/internal/ext/iterx"
)

// validateConstraints validates miscellaneous constraints that depend on the
// whole IR being constructed properly.
func validateConstraints(f File, r *report.Report) {
	builtins := f.Context().builtins()

	for ty := range seq.Values(f.Types()) {
		for member := range seq.Values(ty.Members()) {
			if ty.IsMessageSet() {
				r.Errorf("field declared in %s `%s`", taxa.MessageSet, ty.FullName()).Apply(
					report.Snippet(member.AST()),
					report.Snippetf(ty.Options().Field(builtins.MessageSet).MessageKeys().At(0), "declared as message set here"),
					report.Helpf("message set types may only declare extension ranges"),
				)
			}
		}

		if ty.IsMessageSet() {
			if f.Syntax() == syntax.Proto3 {
				r.Errorf("%s are not supported", taxa.MessageSet).Apply(
					report.Snippet(ty.AST()),
					report.Snippetf(ty.Options().Field(builtins.MessageSet).MessageKeys().At(0), "declared as message set here"),
					report.Snippetf(f.AST().Syntax().Value(), "\"proto3\" specified here"),
					report.Helpf("%ss cannot be defined in \"proto3\" only", taxa.MessageSet),
					report.Helpf("%ss are not implemented correctly in most Protobuf implementations", taxa.MessageSet),
				)
			} else {
				r.Warnf("%ss are deprecated", taxa.MessageSet).Apply(
					report.Snippet(ty.AST()),
					report.Snippetf(ty.Options().Field(builtins.MessageSet).MessageKeys().At(0), "declared as message set here"),
					report.Helpf("%ss are not implemented correctly in most Protobuf implementations", taxa.MessageSet),
				)
			}

			if ty.ExtensionRanges().Len() == 0 {
				r.Errorf("%s `%s` declares no %ss", taxa.MessageSet, ty.FullName(), taxa.Extensions).Apply(
					report.Snippet(ty.AST()),
					report.Snippetf(ty.Options().Field(builtins.MessageSet).MessageKeys().At(0), "declared as message set here"),
				)
			}
		}
	}

	for extn := range seq.Values(f.AllExtensions()) {
		extendee := extn.Container()
		if extendee.IsMessageSet() && !extn.IsMap() {
			// NOTE: extensions already cannot be map fields, so we don't need to diagnose them.

			if extn.Presence() == presence.Repeated {
				_, repeated := iterx.Find(extn.AST().Type().Prefixes(), func(ty ast.TypePrefixed) bool {
					return ty.Prefix() == keyword.Repeated
				})

				r.Errorf("repeated message set extension").Apply(
					report.Snippet(repeated.PrefixToken()),
					report.Snippetf(extendee.Options().Field(builtins.MessageSet).MessageKeys().At(0), "declared as message set here"),
					report.Helpf("message set extensions must be singular message fields"),
				)
			}
			if !extn.Element().IsMessage() {
				r.Errorf("non-message message set extension").Apply(
					report.Snippet(extn.AST().Type().RemovePrefixes()),
					report.Snippetf(extendee.Options().Field(builtins.MessageSet).MessageKeys().At(0), "declared as message set here"),
					report.Helpf("message set extensions must be singular message fields"),
				)
			}
		}
	}
}
