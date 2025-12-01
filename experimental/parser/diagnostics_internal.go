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
	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/ast/syntax"
	"github.com/bufbuild/protocompile/experimental/internal/just"
	"github.com/bufbuild/protocompile/experimental/internal/taxa"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/internal/ext/iterx"
)

// errMoreThanOne is used to diagnose the occurrence of some construct more
// than one time, when it is expected to occur at most once.
type errMoreThanOne struct {
	first, second source.Spanner
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
		source.Spanner
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
	child        source.Spanner
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
			d.Apply(report.Helpf("this %s can only appear at %s", what, v))
		} else {
			d.Apply(report.Helpf("this %s can only appear within a %s", what, v))
		}
	} else {
		d.Apply(report.Helpf(
			"this %s can only appear within one of %s",
			what, e.validParents.Join("or"),
		))
	}
}

// errRequiresEdition diagnoses that a certain edition is required for a feature.
//
//nolint:govet // Irrelevant alignment padding lint.
type errRequiresEdition struct {
	edition syntax.Syntax
	node    source.Spanner
	what    any
	decl    ast.DeclSyntax

	// If set, this will report that the feature is not implemented instead.
	unimplemented bool
}

func (e errRequiresEdition) Diagnose(d *report.Diagnostic) {
	what := e.what
	if what == nil {
		what = taxa.Classify(e.node)
	}

	if e.unimplemented {
		d.Apply(
			report.Message("sorry, %s is not implemented yet", what),
			report.Snippet(e.node),
			report.Helpf("%s is part of Edition %s, which will be implemented in a future release", what, e.edition),
		)
		return
	}

	d.Apply(
		report.Message("%s requires Edition %s or later", what, e.edition),
		report.Snippet(e.node),
	)

	if !e.decl.IsZero() {
		report.Snippetf(e.decl.Value(), "%s specified here", e.decl.Keyword())
	}
}

// errUnexpectedMod diagnoses a modifier placed in the wrong position.
type errUnexpectedMod struct {
	mod   token.Token
	where taxa.Place

	syntax   syntax.Syntax
	noDelete bool
}

func (e errUnexpectedMod) Diagnose(d *report.Diagnostic) {
	d.Apply(
		report.Message("unexpected `%s` modifier %s", e.mod.Keyword(), e.where),
		report.Snippet(e.mod),
	)

	if !e.noDelete {
		d.Apply(
			just.Justify(e.mod.Context(), e.mod.Span(), "delete it", just.Edit{
				Edit: report.Edit{Start: 0, End: e.mod.Span().Len()},
				Kind: just.Right,
			}))
	}

	switch k := e.mod.Keyword(); {
	case k.IsFieldTypeModifier():
		d.Apply(report.Helpf("`%s` only applies to a %s", k, taxa.Field))

	case k.IsTypeModifier():
		d.Apply(report.Helpf("`%s` only applies to a type definition", k))
	case k.IsImportModifier():
		d.Apply(report.Helpf("`%s` only applies to an %s", k, taxa.Import))

	case k.IsMethodTypeModifier():
		d.Apply(report.Helpf("`%s` only applies to an input or output of a %s", k, taxa.Method))
	}
}
