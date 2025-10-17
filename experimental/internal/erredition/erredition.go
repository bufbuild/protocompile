// Package erredition defines common diagnostics for issuing errors about
// the wrong edition being used.
package erredition

import (
	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/ast/syntax"
	"github.com/bufbuild/protocompile/experimental/report"
)

// TooNew diagnoses an edition that is too old for the feature used.
type TooOld struct {
	Current syntax.Syntax
	Decl    ast.DeclSyntax
	Intro   syntax.Syntax

	What  any
	Where report.Spanner
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
	Where report.Spanner
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
