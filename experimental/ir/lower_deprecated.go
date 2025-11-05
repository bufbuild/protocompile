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
	"github.com/bufbuild/protocompile/experimental/source"
)

// checkDeprecated checks for deprecation warnings in the given file.
func checkDeprecated(file *File, r *report.Report) {
	for imp := range seq.Values(file.Imports()) {
		if d := imp.Deprecated(); !d.IsZero() {
			r.Warn(errDeprecated{
				ref:   imp.Decl.ImportPath(),
				name:  imp.Path(),
				cause: d.OptionSpan(),
			})
		}
	}

	checkDeprecatedOptions(file.Options(), r)

	for ty := range seq.Values(file.AllTypes()) {
		checkDeprecatedOptions(ty.Options(), r)
		for o := range seq.Values(ty.Oneofs()) {
			checkDeprecatedOptions(o.Options(), r)
		}
	}

	for m := range file.AllMembers() {
		checkDeprecatedOptions(m.Options(), r)

		ty := m.Element()
		// We do not emit deprecation warnings for references to a type
		// defined in the same file, because this is a relatively common case.
		if m.Context() != ty.Context() {
			if d := ty.Deprecated(); !d.IsZero() {
				r.Warn(errDeprecated{
					ref:   m.TypeAST().RemovePrefixes(),
					name:  string(ty.FullName()),
					cause: d.OptionSpan(),
				})
			}
		}
	}

	for s := range seq.Values(file.Services()) {
		checkDeprecatedOptions(s.Options(), r)

		for m := range seq.Values(s.Methods()) {
			checkDeprecatedOptions(m.Options(), r)

			in, _ := m.Input()
			if m.Context() != in.Context() {
				if d := in.Deprecated(); !d.IsZero() {
					r.Warn(errDeprecated{
						ref:   m.AST().Signature().Inputs().At(0).RemovePrefixes(),
						name:  string(in.FullName()),
						cause: d.OptionSpan(),
					})
				}
			}

			out, _ := m.Input()
			if m.Context() != out.Context() {
				if d := out.Deprecated(); !d.IsZero() {
					r.Warn(errDeprecated{
						ref:   m.AST().Signature().Outputs().At(0).RemovePrefixes(),
						name:  string(out.FullName()),
						cause: d.OptionSpan(),
					})
				}
			}
		}
	}
}

func checkDeprecatedOptions(value MessageValue, r *report.Report) {
	for field := range value.Fields() {
		if d := field.Field().Deprecated(); !d.IsZero() {
			for key := range seq.Values(field.KeyASTs()) {
				r.Warn(errDeprecated{
					ref:   key,
					name:  string(field.Field().FullName()),
					cause: d.OptionSpan(),
				})
			}
		}

		for elem := range seq.Values(field.Elements()) {
			if enum := elem.AsEnum(); !enum.IsZero() {
				if d := enum.Deprecated(); !d.IsZero() {
					r.Warn(errDeprecated{
						ref:   elem.AST(),
						name:  string(enum.FullName()),
						cause: d.OptionSpan(),
					})
				}
			} else if msg := elem.AsMessage(); !msg.IsZero() {
				checkDeprecatedOptions(msg, r)
			}
		}
	}
}

// errDeprecated diagnoses a deprecation.
type errDeprecated struct {
	ref, cause source.Spanner
	name       string
}

func (e errDeprecated) Diagnose(d *report.Diagnostic) {
	d.Apply(
		report.Message("`%s` is deprecated", e.name),
		report.Snippet(e.ref),
		report.Snippetf(e.cause, "deprecated here"),
	)
}
