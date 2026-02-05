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

// Package erredition defines common diagnostics for issuing errors about
// the wrong edition being used.
package erredition

import (
	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/ast/syntax"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/source"
)

// TooOld diagnoses an edition that is too old for the feature used.
type TooOld struct {
	What    any
	Where   source.Spanner
	Decl    ast.DeclSyntax
	Current syntax.Syntax
	Intro   syntax.Syntax
}

// Diagnose implements [report.Diagnoser].
func (e TooOld) Diagnose(d *report.Diagnostic) {
	kind := "syntax"
	if e.Current.IsEdition() {
		kind = "edition"
	}

	d.Apply(
		report.Message("`%s` is not supported in %s", e.What, e.Current.Name()),
		report.Snippet(e.Where),
		report.Snippetf(e.Decl.Value(), "%s specified here", kind),
	)

	if e.Intro != syntax.Unknown {
		d.Apply(report.Helpf("`%s` requires at least %s", e.What, e.Intro.Name()))
	}
}

// TooNew diagnoses an edition that is too new for the feature used.
type TooNew struct {
	Current syntax.Syntax
	Decl    ast.DeclSyntax

	Deprecated, Removed             syntax.Syntax
	DeprecatedReason, RemovedReason string

	What  any
	Where source.Spanner
}

// Diagnose implements [report.Diagnoser].
func (e TooNew) Diagnose(d *report.Diagnostic) {
	kind := "syntax"
	if e.Current.IsEdition() {
		kind = "edition"
	}

	err := "not supported"
	if !e.isRemoved() {
		err = "deprecated"
	}

	d.Apply(
		report.Message("`%s` is %s in %s", e.What, err, e.Current.Name()),
		report.Snippet(e.Where),
		report.Snippetf(e.Decl.Value(), "%s specified here", kind),
	)

	if e.isRemoved() {
		if e.isDeprecated() {
			d.Apply(report.Helpf("deprecated since %s, removed in %s", e.Deprecated.Name(), e.Removed.Name()))
		} else {
			d.Apply(report.Helpf("removed in %s", e.Removed.Name()))
		}

		if e.RemovedReason != "" {
			d.Apply(report.Helpf("%s", normalizeReason(e.RemovedReason)))
			return
		}
	} else if e.isDeprecated() {
		if e.Removed != syntax.Unknown {
			d.Apply(report.Helpf("deprecated since %s, to be removed in %s", e.Deprecated.Name(), e.Removed.Name()))
		} else {
			d.Apply(report.Helpf("deprecated since %s", e.Deprecated.Name()))
		}
	}

	if e.DeprecatedReason != "" {
		d.Apply(report.Helpf("%s", normalizeReason(e.DeprecatedReason)))
	}
}

func (e TooNew) isDeprecated() bool {
	return e.Deprecated != syntax.Unknown && e.Deprecated <= e.Current
}

func (e TooNew) isRemoved() bool {
	return e.Removed != syntax.Unknown && e.Removed <= e.Current
}
