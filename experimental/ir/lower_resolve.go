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
	"github.com/bufbuild/protocompile/experimental/internal/taxa"
	"github.com/bufbuild/protocompile/experimental/ir/presence"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/experimental/token/keyword"
	"github.com/bufbuild/protocompile/internal/arena"
	"github.com/bufbuild/protocompile/internal/ext/iterx"
)

// resolveNames resolves all of the names that need resolving in a file.
func resolveNames(f File, r *report.Report) {
	resolveLangSymbols(f.Context())

	for ty := range seq.Values(f.AllTypes()) {
		if ty.IsMessage() {
			for field := range seq.Values(ty.Members()) {
				resolveFieldType(field, r)
			}
		}
	}

	for extendee := range f.Context().arenas.extendees.Values() {
		resolveExtendeeType(f.Context(), extendee, r)
	}

	for field := range seq.Values(f.AllExtensions()) {
		resolveFieldType(field, r)
	}
}

// resolveFieldType fully resolves the type of a field (extension or otherwise).
func resolveFieldType(field Member, r *report.Report) {
	ty := field.AST().Type()
	var path ast.Path
	kind := presence.Explicit
	switch ty.Kind() {
	case ast.TypeKindPath:
		if field.Context().File().Syntax() == syntax.Proto3 {
			kind = presence.Implicit
		}
		// NOTE: Editions features are resolved elsewhere, so we default to
		// explicit presence here.

		path = ty.AsPath().Path

	case ast.TypeKindPrefixed:
		// Unwrap as many prefixed fields as necessary to get to the bottom
		// of this.
		inner := ty
		for {
			if p := inner.AsPath(); !p.IsZero() {
				path = p.Path
				break
			}
			if p := inner.AsPrefixed(); !p.IsZero() {
				inner = p.Type()
				continue
			}
			sorry("map fields")
		}

		switch ty.AsPrefixed().Prefix() {
		case keyword.Optional:
			kind = presence.Explicit
		case keyword.Required:
			kind = presence.Required
		case keyword.Repeated:
			kind = presence.Repeated
		}

	case ast.TypeKindGeneric:
		sorry("map fields")
	}

	if path.IsZero() {
		// Enum value; this is legalized elsewhere.
		return
	}

	if field.raw.oneof < 0 {
		field.raw.oneof = -int32(kind)
	}

	sym := symbolRef{
		Context: field.Context(),
		Report:  r,

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
		field.raw.elem.file = sym.ref.file
		field.raw.elem.ptr = arena.Pointer[rawType](sym.raw.data)
	}
}

func resolveExtendeeType(c *Context, extendee *rawExtendee, r *report.Report) {
	path := extendee.def.Name()
	sym := symbolRef{
		Context: c,
		Report:  r,

		span:  path,
		scope: extendee.Scope(c),
		name:  FullName(path.Canonicalized()),

		accept: func(k SymbolKind) bool { return k == SymbolKindMessage },
		want:   taxa.MessageType,

		allowScalars:  true,
		suggestImport: true,
	}.resolve()

	if sym.Kind().IsType() {
		extendee.ty.file = sym.ref.file
		extendee.ty.ptr = arena.Pointer[rawType](sym.raw.data)
	}
}

func resolveLangSymbols(c *Context) {
	if !c.isDescriptorProto {
		return
	}

	field := func(name string) arena.Pointer[rawMember] {
		return mustResolve[rawMember](c, "google.protobuf."+name, SymbolKindField)
	}

	c.langSymbols = &langSymbols{
		fileOptions: field("FileDescriptorProto.options"),

		messageOptions: field("DescriptorProto.options"),
		fieldOptions:   field("FieldDescriptorProto.options"),
		oneofOptions:   field("OneofDescriptorProto.options"),

		enumOptions:      field("EnumDescriptorProto.options"),
		enumValueOptions: field("EnumValueDescriptorProto.options"),
	}
}

// mustResolve resolves a descriptor.proto name, and panics if it's not found.
func mustResolve[Raw any](c *Context, name string, kind SymbolKind) arena.Pointer[Raw] {
	id := c.session.intern.Intern(name)
	ref := c.exported.lookup(c, id)
	sym := wrapSymbol(c, ref)
	if sym.Kind() != kind {
		panic(fmt.Errorf("missing descriptor.proto symbol: %s `%s`; got kind %s", kind.noun(), name, sym.Kind()))
	}
	return arena.Pointer[Raw](sym.raw.data)
}

// symbolRef is all of the information necessary to resolve a symbol reference.
type symbolRef struct {
	*Context
	*report.Report

	scope, name FullName
	span        report.Spanner

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
		found    ref[rawSymbol]
		expected FullName
	)

	var fullResolve bool
	switch {
	case r.name.Absolute():
		if id, ok := r.session.intern.Query(string(r.name.ToRelative())); ok {
			found = r.imported.lookup(r.Context, id)
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
			return wrapSymbol(r.Context, ref[rawSymbol]{
				file: -1,
				ptr:  arena.Pointer[rawSymbol](prim),
			})
		}

		fallthrough
	default:
		fullResolve = true
		found, expected = r.imported.resolve(r.Context, r.scope, r.name, r.skipIfNot, nil)
	}

	sym := wrapSymbol(r.Context, found)
	d := r.diagnoseLookup(sym, expected)
	if fullResolve && d != nil {
		// Resolve a second time to add debugging information to the diagnostic.
		r.imported.resolve(r.Context, r.scope, r.name, r.skipIfNot, d)
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
	case !sym.Visible():
		// Complain that we need to import a symbol.
		d := r.Errorf("cannot find `%s` in this scope", r.name).Apply(
			report.Snippetf(r.span, "not visible in this scope"),
			report.Snippetf(sym.Definition(), "found in unimported file"),
		)

		if !r.suggestImport {
			return d
		}

		// Find the last import statement and stick the suggestion after it.
		decls := sym.Context().File().AST().Decls()
		_, _, imp := iterx.Find2(seq.Backward(decls), func(_ int, d ast.DeclAny) bool {
			return d.Kind() == ast.DeclKindImport
		})

		var offset int
		if !imp.IsZero() {
			offset = imp.Span().End
		}

		replacement := fmt.Sprintf("\nimport %q;", sym.File().Path())
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
