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

	"github.com/bufbuild/protocompile/experimental/internal/taxa"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/internal"
	"github.com/bufbuild/protocompile/internal/cases"
	"github.com/bufbuild/protocompile/internal/intern"
)

func populateJSONNames(file *File, r *report.Report) {
	builtins := file.builtins()
	names := intern.Map[Member]{}

	for ty := range seq.Values(file.AllTypes()) {
		clear(names)

		jsonFormat, _ := ty.FeatureSet().Lookup(builtins.FeatureJSON).Value().AsInt()
		strict := jsonFormat == 1

		// First, populate the default names, and check for collisions among
		// them.
		for field := range seq.Values(ty.Members()) {
			var name string
			if ty.IsEnum() {
				name = internal.TrimPrefix(field.Name(), ty.Name())
				name = cases.Enum.Convert(name)
			} else {
				name = internal.JSONName(field.Name())
			}

			field.Raw().jsonName = file.session.intern.Intern(name)

			prev, ok := names.AddID(field.Raw().jsonName, field)
			if prev.Number() == field.Number() {
				// This handles the case where enum numbers coincide in an
				// allow_alias enum. In all other cases where numbers coincide,
				// this has been diagnosed elsewhere already.
				continue
			}

			if !ok {
				r.SoftError(strict, errJSONConflict{
					first: prev, second: field,
				})
			}
		}

		if ty.IsEnum() {
			// Don't bother iterating again, since enums cannot have custom
			// JSON names.
			continue
		}

		clear(names)

		// Now do custom names. These are always an error if they conflict.
		for field := range seq.Values(ty.Members()) {
			option := field.PseudoOptions().JSONName

			name, custom := option.AsString()
			if custom {
				field.Raw().jsonName = file.session.intern.Intern(name)
			}

			prev, ok := names.AddID(field.Raw().jsonName, field)
			if !ok && (custom || !prev.PseudoOptions().JSONName.IsZero()) {
				r.Error(errJSONConflict{
					first: prev, second: field,
					involvesCustomName: true,
				})
			}
		}
	}

	for extn := range seq.Values(file.AllExtensions()) {
		want := internal.JSONName(extn.Name())
		option := extn.PseudoOptions().JSONName
		got, custom := option.AsString()

		name := want
		if custom {
			name = got
		}
		extn.Raw().jsonName = file.session.intern.Intern(name)

		if custom {
			d := r.SoftErrorf(want != got, "%s cannot specify `json_name`", taxa.Extension).Apply(
				report.Snippet(option.OptionSpan()),
				report.Notef("JSON format for extensions always uses the extension's fully-qualified name"),
			)
			if want == got {
				d.Apply(report.Helpf("protoc erroneously accepts `json_name` on an extension " +
					"if it happens to match the default JSON name exactly"))
			}
		}
	}
}

type errJSONConflict struct {
	first, second      Member
	involvesCustomName bool
}

func (e errJSONConflict) Diagnose(d *report.Diagnostic) {
	eitherIsCustom := !e.first.PseudoOptions().JSONName.IsZero() ||
		!e.second.PseudoOptions().JSONName.IsZero()

	if !e.involvesCustomName && eitherIsCustom {
		d.Apply(report.Message("%ss have the same (default) JSON name", e.first.noun()))
	} else {
		d.Apply(report.Message("%ss have the same JSON name", e.first.noun()))
	}

	snippet := func(m Member) report.DiagnosticOption {
		option := m.PseudoOptions().JSONName
		if _, custom := option.AsString(); custom {
			if e.involvesCustomName {
				return report.Snippetf(option.ValueAST(), "`%s` specifies custom name here", m.Name())
			}
			return report.Snippetf(m.AST().Name(), "this implies (default) JSON name `%s`", m.JSONName())
		}

		if m == e.second {
			return report.Snippetf(m.AST().Name(), "this also implies that name")
		}

		return report.Snippetf(m.AST().Name(), "this implies JSON name `%s`", m.JSONName())
	}
	d.Apply(snippet(e.second), snippet(e.first))

	if !e.involvesCustomName {
		_, firstCustom := e.first.PseudoOptions().JSONName.AsString()
		_, secondCustom := e.second.PseudoOptions().JSONName.AsString()

		var what string
		switch {
		case firstCustom && secondCustom:
			what = "both fields set"
		case firstCustom:
			what = fmt.Sprintf("`%s` sets", e.first.Name())
		case secondCustom:
			what = fmt.Sprintf("`%s` sets", e.second.Name())
		default:
			return
		}

		d.Apply(report.Helpf("even though %s `json_name`, their default "+
			"JSON names must not conflict, because `google.protobuf.FieldMask`'s "+
			"JSON syntax erroneously does not account for custom JSON names", what))
	}
}
