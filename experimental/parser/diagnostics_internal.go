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

package parser

import (
	"fmt"

	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/internal/taxa"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/internal/ext/iterx"
)

// errUnexpected is a low-level parser error for when we hit a token we don't
// know how to handle.
type errUnexpected struct {
	// The unexpected thing (may be a token or AST node).
	what report.Spanner

	// The context we're in. Should be format-able with %v.
	where taxa.Place
	// Useful when where is an "after" position: if non-nil, this will be
	// highlighted as "previous where.Object is here"
	prev report.Spanner

	// What we wanted vs. what we got. Got can be used to customize what gets
	// shown, but if it's not set, we call describe(what) to get a user-visible
	// description.
	want taxa.Set
	// If set and want is empty, the snippet will repeat the "unexpected foo"
	// text under the snippet.
	repeatUnexpected bool
	got              any
}

func (e errUnexpected) Diagnose(d *report.Diagnostic) {
	got := e.got
	if got == nil {
		got = taxa.Classify(e.what)
		if got == taxa.Unknown {
			got = "tokens"
		}
	}

	var message string
	if e.where.Subject() == taxa.Unknown {
		message = fmt.Sprintf("unexpected %v", got)
	} else {
		message = fmt.Sprintf("unexpected %v %v", got, e.where)
	}

	snippet := report.Snippet(e.what)
	if e.want.Len() > 0 {
		snippet = report.Snippetf(e.what, "expected %v", e.want.Join("or"))
	} else if e.repeatUnexpected {
		snippet = report.Snippetf(e.what, "%v", message)
	}

	d.Apply(
		report.Message("%v", message),
		snippet,
		report.Snippetf(e.prev, "previous %v is here", e.where.Subject()),
	)
}

// errMoreThanOne is used to diagnose the occurrence of some construct more
// than one time, when it is expected to occur at most once.
type errMoreThanOne struct {
	first, second report.Spanner
	what          taxa.Noun
}

func (e errMoreThanOne) Diagnose(d *report.Diagnostic) {
	what := e.what
	if what == taxa.Unknown {
		what = taxa.Classify(e.first)
	}

	d.Apply(
		report.Message("encountered more than one %v", what),
		report.Snippetf(e.second, "help: consider removing this"),
		report.Snippetf(e.first, "first one is here"),
	)
}

// errHasOptions diagnoses the presence of compact options on a construct that
// does not permit them.
type errHasOptions struct {
	what interface {
		report.Spanner
		Options() ast.CompactOptions
	}
}

func (e errHasOptions) Diagnose(d *report.Diagnostic) {
	d.Apply(
		report.Message("%s cannot specify %s", taxa.Classify(e.what), taxa.CompactOptions),
		report.Snippetf(e.what.Options(), "help: remove this"),
	)
}

// errHasSignature diagnoses the presence of a method signature on a non-method.
type errHasSignature struct {
	what ast.DeclDef
}

func (e errHasSignature) Diagnose(d *report.Diagnostic) {
	d.Apply(
		report.Message("%s appears to have %s", taxa.Classify(e.what), taxa.Signature),
		report.Snippetf(e.what.Signature(), "help: remove this"),
	)
}

// errBadNest diagnoses bad nesting: parent should not contain child.
type errBadNest struct {
	parent       classified
	child        report.Spanner
	validParents taxa.Set
}

func (e errBadNest) Diagnose(d *report.Diagnostic) {
	what := taxa.Classify(e.child)
	if e.parent.what == taxa.TopLevel {
		d.Apply(
			report.Message("unexpected %s at %s", what, e.parent.what),
			report.Snippetf(e.child, "this %s cannot be declared here", what),
		)
	} else {
		d.Apply(
			report.Message("unexpected %s within %s", what, e.parent.what),
			report.Snippetf(e.child, "this %s...", what),
			report.Snippetf(e.parent, "...cannot be declared within this %s", e.parent.what),
		)
	}

	if e.validParents.Len() == 1 {
		v, _ := iterx.First(e.validParents.All())
		if v == taxa.TopLevel {
			// This case is just to avoid printing "within a top-level scope",
			// which looks wrong.
			d.Apply(report.Helpf("a %s can only appear at %s", what, v))
		} else {
			d.Apply(report.Helpf("a %s can only appear within a %s", what, v))
		}
	} else {
		d.Apply(report.Helpf(
			"a %s can only appear within one of %s",
			what, e.validParents.Join("or"),
		))
	}
}
