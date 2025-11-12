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

	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/ast/predeclared"
	"github.com/bufbuild/protocompile/experimental/ast/syntax"
	"github.com/bufbuild/protocompile/experimental/id"
	"github.com/bufbuild/protocompile/experimental/internal/taxa"
	"github.com/bufbuild/protocompile/experimental/ir/presence"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/experimental/token/keyword"
	"github.com/bufbuild/protocompile/internal/ext/iterx"
)

// resolveNames resolves all of the names that need resolving in a file.
func resolveNames(file *File, r *report.Report) {
	resolveBuiltins(file)

	for ty := range seq.Values(file.AllTypes()) {
		if ty.IsMessage() {
			for field := range seq.Values(ty.Members()) {
				resolveFieldType(field, r)
			}
		}
	}

	for extend := range seq.Values(file.AllExtends()) {
		resolveExtendeeType(extend, r)
	}

	for field := range seq.Values(file.AllExtensions()) {
		resolveFieldType(field, r)
	}

	for service := range seq.Values(file.Services()) {
		for method := range seq.Values(service.Methods()) {
			resolveMethodTypes(method, r)
		}
	}
}

// resolveFieldType fully resolves the type of a field (extension or otherwise).
func resolveFieldType(field Member, r *report.Report) {
	ty := field.TypeAST()
	var path ast.Path
	kind := presence.Explicit
	switch ty.Kind() {
	case ast.TypeKindPath:
		if field.Context().Syntax() == syntax.Proto3 {
			kind = presence.Implicit
		}
		// NOTE: Editions features are resolved elsewhere, so we default to
		// explicit presence here.

		path = ty.AsPath().Path

	case ast.TypeKindPrefixed:
		switch ty.AsPrefixed().Prefix() {
		case keyword.Optional:
			kind = presence.Explicit
		case keyword.Required:
			kind = presence.Required
		case keyword.Repeated:
			kind = presence.Repeated
		}

		// Unwrap as many prefixed fields as necessary to get to the bottom
		// of this.
		ty = ty.RemovePrefixes()
		if p := ty.AsPath().Path; !p.IsZero() {
			path = p
			break
		}

		fallthrough

	case ast.TypeKindGeneric:
		// Resolved elsewhere.
		return
	}

	if path.IsZero() {
		// Enum value; this is legalized elsewhere.
		return
	}

	if field.Raw().oneof < 0 {
		field.Raw().oneof = -int32(kind)
	}

	sym := symbolRef{
		File:   field.Context(),
		Report: r,

		span:  path,
		scope: field.Scope(),
		name:  FullName(path.Canonicalized()),

		skipIfNot: SymbolKind.IsType,
		accept:    SymbolKind.IsType,
		want:      taxa.Type,

		allowScalars:  true,
		suggestImport: true,
	}.resolve()

	if sym.Kind().IsType() {
		ty := sym.AsType()
		field.Raw().elem = ty.toRef(field.Context())

		if mf := sym.AsType().MapField(); !mf.IsZero() {
			r.Errorf("use of synthetic map entry type").Apply(
				report.Snippetf(path, "referenced here"),
				report.Snippetf(mf.TypeAST(), "synthesized by this type"),
				report.Helpf("despite having a user-visible symbol, map entry "+
					"types cannot be used as field types"),
			)
		}

		if !field.Container().MapField().IsZero() && field.Number() == 1 {
			// Legalize that the key type must be comparable.
			ty := sym.AsType()
			if !ty.Predeclared().IsMapKey() {
				d := r.Error(errTypeConstraint{
					want: "map key type",
					got:  sym.AsType(),
					decl: field.TypeAST(),
				}).Apply(
					report.Helpf("valid map key types are integer types, `string`, and `bool`"),
				)

				if ty.IsEnum() {
					d.Apply(report.Helpf(
						"counterintuitively, user-defined enum types " +
							"cannot be used as keys"))
				}
			}
		}
	}
}

func resolveExtendeeType(extend Extend, r *report.Report) {
	path := extend.AST().Name()
	sym := symbolRef{
		File:   extend.Context(),
		Report: r,

		span:  path,
		scope: extend.Scope(),
		name:  FullName(path.Canonicalized()),

		accept: func(k SymbolKind) bool { return k == SymbolKindMessage },
		want:   taxa.MessageType,

		allowScalars:  true,
		suggestImport: true,
	}.resolve()

	if sym.Kind().IsType() {
		extend.Raw().ty = sym.AsType().toRef(extend.Context())
	}
}

func resolveMethodTypes(m Method, r *report.Report) {
	resolve := func(ty ast.TypeAny) (out Ref[Type], stream bool) {
		var path ast.Path
		for path.IsZero() {
			switch ty.Kind() {
			case ast.TypeKindPath:
				path = ty.AsPath().Path
			case ast.TypeKindPrefixed:
				prefixed := ty.AsPrefixed()
				if prefixed.Prefix() == keyword.Stream {
					stream = true
				}
				ty = prefixed.Type()
			default:
				// This is already diagnosed in the parser for us.
				return out, stream
			}
		}

		sym := symbolRef{
			File:   m.Context(),
			Report: r,

			span:  path,
			scope: m.Service().FullName(),
			name:  FullName(path.Canonicalized()),

			accept: func(k SymbolKind) bool { return k == SymbolKindMessage },
			want:   taxa.MessageType,

			allowScalars:  true,
			suggestImport: true,
		}.resolve()

		if sym.Kind().IsType() {
			out = sym.AsType().toRef(m.Context())
		}

		return out, stream
	}

	signature := m.AST().Signature()
	if signature.Inputs().Len() > 0 {
		m.Raw().input, m.Raw().inputStream = resolve(m.AST().Signature().Inputs().At(0))
	}
	if signature.Outputs().Len() > 0 {
		m.Raw().output, m.Raw().outputStream = resolve(m.AST().Signature().Outputs().At(0))
	}
}

// symbolRef is all of the information necessary to resolve a symbol reference.
type symbolRef struct {
	*File
	*report.Report

	scope, name FullName
	span        source.Spanner

	skipIfNot, accept func(SymbolKind) bool
	want              taxa.Noun

	// If true, the names of scalars will be resolved as potential symbols.
	allowScalars bool

	// If true, diagnostics will not suggest adding an import.
	suggestImport bool
}

// resolve performs symbol resolution.
func (r symbolRef) resolve() Symbol {
	var (
		found    Ref[Symbol]
		expected FullName
	)

	var fullResolve bool
	switch {
	case r.name.Absolute():
		if id, ok := r.session.intern.Query(string(r.name.ToRelative())); ok {
			found = r.imported.lookup(r.File, id)
		}
	case r.allowScalars:
		// TODO: if symbol resolution would provide a different answer for
		// looking up this primitive, we should consider diagnosing it. We don't
		// currently because:
		//
		// 1. Diagnosing every use would be extremely noisy.
		//
		// 2. Diagnosing only the first might be a false positive, which would
		//    make this warning user-hostile.

		prim := predeclared.Lookup(string(r.name))
		if prim.IsScalar() {
			sym := GetRef(r.File, Ref[Symbol]{
				file: -1,
				id:   id.ID[Symbol](prim),
			})
			r.diagnoseLookup(sym, expected)
			return sym
		}

		fallthrough
	default:
		fullResolve = true
		found, expected = r.imported.resolve(r.File, r.scope, r.name, r.skipIfNot, nil)
	}

	sym := GetRef(r.File, found)
	if r.Report != nil {
		d := r.diagnoseLookup(sym, expected)
		if fullResolve && d != nil {
			// Resolve a second time to add debugging information to the diagnostic.
			r.imported.resolve(r.File, r.scope, r.name, r.skipIfNot, d)
		}
	}

	return sym
}

// diagnoseLookup generates diagnostics for a possibly-failed symbol resolution
// operation.
func (r symbolRef) diagnoseLookup(sym Symbol, expectedName FullName) *report.Diagnostic {
	if sym.IsZero() {
		return r.Errorf("cannot find `%s` in this scope", r.name).Apply(
			report.Snippetf(r.span, "not found in this scope"),
			report.Helpf("the full name of this scope is `%s`", r.scope),
		)
	}

	if k := sym.Kind(); r.accept != nil && !r.accept(k) {
		return r.Errorf("expected %s, found %s `%s`", r.want, k.noun(), sym.FullName()).Apply(
			report.Snippetf(r.span, "expected %s", r.want),
			report.Snippetf(sym.Definition(), "defined here"),
		)
	}

	switch {
	case expectedName != "":
		// Complain if we found the "wrong" type.
		return r.Errorf("cannot find `%s` in this scope", r.name).Apply(
			report.Snippetf(r.span, "not found in this scope"),
			report.Snippetf(sym.Definition(),
				"found possibly related symbol `%s`", sym.FullName()),
			report.Notef(
				"Protobuf's name lookup rules expected a symbol `%s`, "+
					"rather than the one we found",
				expectedName),
		)
	case !sym.Visible(r.File):
		// Complain that we need to import a symbol.
		d := r.Errorf("cannot find `%s` in this scope", r.name).Apply(
			report.Snippetf(r.span, "not visible in this scope"),
			report.Snippetf(sym.Definition(), "found in unimported file"),
		)

		if !r.suggestImport {
			return d
		}

		// Find the last import statement and stick the suggestion after it.
		decls := sym.Context().AST().Decls()
		_, _, imp := iterx.Find2(seq.Backward(decls), func(_ int, d ast.DeclAny) bool {
			return d.Kind() == ast.DeclKindImport
		})

		var offset int
		if !imp.IsZero() {
			offset = imp.Span().End
		}

		replacement := fmt.Sprintf("\nimport %q;", sym.Context().Path())
		if offset == 0 {
			replacement = replacement[1:] + "\n"
		}

		d.Apply(report.SuggestEdits(
			imp.Span().File.Span(offset, offset),
			fmt.Sprintf("bring `%s` into scope", r.name),
			report.Edit{Replace: replacement},
		))

		return d
	}

	return nil
}
