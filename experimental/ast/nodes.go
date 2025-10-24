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

package ast

import (
	"fmt"
	"math"
	"slices"

	"github.com/bufbuild/protocompile/experimental/internal"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/internal/arena"
	"github.com/bufbuild/protocompile/internal/ext/iterx"
)

// Nodes provides storage for the various AST node types, and can be used
// to construct new ones.
type Nodes struct {
	// The context for these nodes.
	Context Context

	decls   decls
	types   types
	exprs   exprs
	options arena.Arena[rawCompactOptions]

	// A cache of raw paths that have been converted into parenthesized
	// components in NewExtensionComponent.
	extnPathCache map[rawPath]token.ID
}

// Root returns the root AST node for this context.
func (n *Nodes) Root() File {
	// NewContext() sticks the root at the beginning of decls.body for us, so
	// there is always a DeclBody at index 0, which corresponds to the whole
	// file. We use a 1 here, not a 0, because arena.Arena's indices are
	// off-by-one to accommodate the zero representation.
	return File{wrapDeclBody(n.Context, 1)}
}

// NewPathComponent returns a new path component with the given separator and
// name.
//
// sep must be a [token.Punct] whose value is either '.' or '/'. name must be
// a [token.Ident]. This function will panic if either condition does not
// hold.
//
// To create a path component with an extension value, see [Nodes.NewExtensionComponent].
func (n *Nodes) NewPathComponent(separator, name token.Token) PathComponent {
	n.panicIfNotOurs(separator, name)
	if !separator.IsZero() {
		if separator.Kind() != token.Punct || (separator.Text() != "." && separator.Text() != "/") {
			panic(fmt.Sprintf("protocompile/ast: passed non '.' or '/' separator to NewPathComponent: %s", separator))
		}
	}
	if name.Kind() != token.Ident {
		panic("protocompile/ast: passed non-identifier name to NewPathComponent")
	}

	return PathComponent{
		withContext: internal.NewWith(n.Context),
		separator:   separator.ID(),
		name:        name.ID(),
	}
}

// NewExtensionComponent returns a new extension path component containing the
// given path.
func (n *Nodes) NewExtensionComponent(separator token.Token, path Path) PathComponent {
	n.panicIfNotOurs(separator, path)
	if !separator.IsZero() {
		if separator.Kind() != token.Punct || (separator.Text() != "." && separator.Text() != "/") {
			panic(fmt.Sprintf("protocompile/ast: passed non '.' or '/' separator to NewPathComponent: %s", separator))
		}
	}

	name, ok := n.extnPathCache[path.raw]
	if !ok {
		stream := n.Context.Stream()
		start := stream.NewPunct("(")
		end := stream.NewPunct(")")
		var children []token.Token
		path.Components(func(pc PathComponent) bool {
			if !pc.Separator().IsZero() {
				children = append(children, pc.Separator())
			}
			if !pc.Name().IsZero() {
				children = append(children, pc.Name())
			}
			return true
		})
		stream.NewFused(start, end, children...)

		name = start.ID()
		if n.extnPathCache == nil {
			n.extnPathCache = make(map[rawPath]token.ID)
		}
		n.extnPathCache[path.raw] = name
	}

	return PathComponent{
		withContext: internal.NewWith(n.Context),
		separator:   separator.ID(),
		name:        name,
	}
}

// NewPath creates a new synthetic Path.
func (n *Nodes) NewPath(components ...PathComponent) Path {
	if len(components) > math.MaxInt16 {
		panic("protocompile/ast: cannot build path with more than 2^15 components")
	}

	for _, t := range components {
		n.panicIfNotOurs(t)
	}

	stream := n.Context.Stream()

	// Every synthetic path looks like a (a.b.c) token tree. Users can't see the
	// parens here.
	start := stream.NewPunct("(")
	end := stream.NewPunct(")")
	var children []token.Token
	for _, pc := range components {
		if !pc.Separator().IsZero() {
			children = append(children, pc.Separator())
		}
		if !pc.Name().IsZero() {
			children = append(children, pc.Name())
		}
	}
	stream.NewFused(start, end, children...)

	path := rawPath{Start: start.ID()}.withSynthRange(0, len(children))

	if n.extnPathCache == nil {
		n.extnPathCache = make(map[rawPath]token.ID)
	}
	n.extnPathCache[path] = path.Start

	return path.With(n.Context)
}

// NewDeclEmpty creates a new DeclEmpty node.
func (n *Nodes) NewDeclEmpty(semicolon token.Token) DeclEmpty {
	n.panicIfNotOurs(semicolon)

	decl := wrapDeclEmpty(n.Context, n.decls.empties.NewCompressed(rawDeclEmpty{
		semi: semicolon.ID(),
	}))

	return decl
}

// NewDeclSyntax creates a new DeclSyntax node.
func (n *Nodes) NewDeclSyntax(args DeclSyntaxArgs) DeclSyntax {
	n.panicIfNotOurs(args.Keyword, args.Equals, args.Value, args.Options, args.Semicolon)

	return wrapDeclSyntax(n.Context, n.decls.syntaxes.NewCompressed(rawDeclSyntax{
		keyword: args.Keyword.ID(),
		equals:  args.Equals.ID(),
		value:   args.Value.raw,
		options: n.options.Compress(args.Options.raw),
		semi:    args.Semicolon.ID(),
	}))
}

// NewDeclPackage creates a new DeclPackage node.
func (n *Nodes) NewDeclPackage(args DeclPackageArgs) DeclPackage {
	n.panicIfNotOurs(args.Keyword, args.Path, args.Options, args.Semicolon)

	return wrapDeclPackage(n.Context, n.decls.packages.NewCompressed(rawDeclPackage{
		keyword: args.Keyword.ID(),
		path:    args.Path.raw,
		options: n.options.Compress(args.Options.raw),
		semi:    args.Semicolon.ID(),
	}))
}

// NewDeclImport creates a new DeclImport node.
func (n *Nodes) NewDeclImport(args DeclImportArgs) DeclImport {
	n.panicIfNotOurs(args.Keyword, args.ImportPath, args.Options, args.Semicolon)

	return wrapDeclImport(n.Context, n.decls.imports.NewCompressed(rawDeclImport{
		keyword: args.Keyword.ID(),
		modifiers: slices.Collect(iterx.Map(
			slices.Values(args.Modifiers),
			func(t token.Token) token.ID {
				n.panicIfNotOurs(t)
				return t.ID()
			}),
		),
		importPath: args.ImportPath.raw,
		options:    n.options.Compress(args.Options.raw),
		semi:       args.Semicolon.ID(),
	}))
}

// NewDeclDef creates a new DeclDef node.
func (n *Nodes) NewDeclDef(args DeclDefArgs) DeclDef {
	n.panicIfNotOurs(
		args.Keyword, args.Type, args.Name, args.Returns,
		args.Equals, args.Value, args.Options, args.Body, args.Semicolon)

	raw := rawDeclDef{
		name:    args.Name.raw,
		equals:  args.Equals.ID(),
		value:   args.Value.raw,
		options: n.options.Compress(args.Options.raw),
		body:    n.decls.bodies.Compress(args.Body.raw),
		semi:    args.Semicolon.ID(),
	}
	if !args.Type.IsZero() {
		raw.ty = args.Type.raw
	} else {
		kw := rawPath{args.Keyword.ID(), args.Keyword.ID()}.With(n.Context)
		raw.ty = wrapPath[TypeKind](kw.raw)
	}
	if !args.Returns.IsZero() {
		raw.signature = &rawSignature{
			returns: args.Returns.ID(),
		}
	}

	return wrapDeclDef(n.Context, n.decls.defs.NewCompressed(raw))
}

// NewDeclBody creates a new DeclBody node.
//
// To add declarations to the returned body, use [DeclBody.Append].
func (n *Nodes) NewDeclBody(braces token.Token) DeclBody {
	n.panicIfNotOurs(braces)

	return wrapDeclBody(n.Context, n.decls.bodies.NewCompressed(rawDeclBody{
		braces: braces.ID(),
	}))
}

// NewDeclRange creates a new DeclRange node.
//
// To add ranges to the returned declaration, use [DeclRange.Append].
func (n *Nodes) NewDeclRange(args DeclRangeArgs) DeclRange {
	n.panicIfNotOurs(args.Keyword, args.Options, args.Semicolon)

	return wrapDeclRange(n.Context, n.decls.ranges.NewCompressed(rawDeclRange{
		keyword: args.Keyword.ID(),
		options: n.options.Compress(args.Options.raw),
		semi:    args.Semicolon.ID(),
	}))
}

// NewExprPrefixed creates a new ExprPrefixed node.
func (n *Nodes) NewExprPrefixed(args ExprPrefixedArgs) ExprPrefixed {
	n.panicIfNotOurs(args.Prefix, args.Expr)

	ptr := n.exprs.prefixes.NewCompressed(rawExprPrefixed{
		prefix: args.Prefix.ID(),
		expr:   args.Expr.raw,
	})
	return ExprPrefixed{exprImpl[rawExprPrefixed]{
		internal.NewWith(n.Context),
		n.exprs.prefixes.Deref(ptr),
	}}
}

// NewExprRange creates a new ExprRange node.
func (n *Nodes) NewExprRange(args ExprRangeArgs) ExprRange {
	n.panicIfNotOurs(args.Start, args.To, args.End)

	ptr := n.exprs.ranges.NewCompressed(rawExprRange{
		to:    args.To.ID(),
		start: args.Start.raw,
		end:   args.End.raw,
	})
	return ExprRange{exprImpl[rawExprRange]{
		internal.NewWith(n.Context),
		n.exprs.ranges.Deref(ptr),
	}}
}

// NewExprArray creates a new ExprArray node.
//
// To add elements to the returned expression, use [ExprArray.Append].
func (n *Nodes) NewExprArray(brackets token.Token) ExprArray {
	n.panicIfNotOurs(brackets)

	ptr := n.exprs.arrays.NewCompressed(rawExprArray{
		brackets: brackets.ID(),
	})
	return ExprArray{exprImpl[rawExprArray]{
		internal.NewWith(n.Context),
		n.exprs.arrays.Deref(ptr),
	}}
}

// NewExprDict creates a new ExprDict node.
//
// To add elements to the returned expression, use [ExprDict.Append].
func (n *Nodes) NewExprDict(braces token.Token) ExprDict {
	n.panicIfNotOurs(braces)

	ptr := n.exprs.dicts.NewCompressed(rawExprDict{
		braces: braces.ID(),
	})
	return ExprDict{exprImpl[rawExprDict]{
		internal.NewWith(n.Context),
		n.exprs.dicts.Deref(ptr),
	}}
}

// NewExprField creates a new ExprPrefixed node.
func (n *Nodes) NewExprField(args ExprFieldArgs) ExprField {
	n.panicIfNotOurs(args.Key, args.Colon, args.Value)

	ptr := n.exprs.fields.NewCompressed(rawExprField{
		key:   args.Key.raw,
		colon: args.Colon.ID(),
		value: args.Value.raw,
	})
	return ExprField{exprImpl[rawExprField]{
		internal.NewWith(n.Context),
		n.exprs.fields.Deref(ptr),
	}}
}

// NewTypePrefixed creates a new TypePrefixed node.
func (n *Nodes) NewTypePrefixed(args TypePrefixedArgs) TypePrefixed {
	n.panicIfNotOurs(args.Prefix, args.Type)

	ptr := n.types.prefixes.NewCompressed(rawTypePrefixed{
		prefix: args.Prefix.ID(),
		ty:     args.Type.raw,
	})
	return TypePrefixed{typeImpl[rawTypePrefixed]{
		internal.NewWith(n.Context),
		n.types.prefixes.Deref(ptr),
	}}
}

// NewTypeGeneric creates a new TypeGeneric node.
//
// To add arguments to the returned type, use [TypeGeneric.Append].
func (n *Nodes) NewTypeGeneric(args TypeGenericArgs) TypeGeneric {
	n.panicIfNotOurs(args.Path, args.AngleBrackets)

	ptr := n.types.generics.NewCompressed(rawTypeGeneric{
		path: args.Path.raw,
		args: rawTypeList{brackets: args.AngleBrackets.ID()},
	})
	return TypeGeneric{typeImpl[rawTypeGeneric]{
		internal.NewWith(n.Context),
		n.types.generics.Deref(ptr),
	}}
}

// NewCompactOptions creates a new CompactOptions node.
func (n *Nodes) NewCompactOptions(brackets token.Token) CompactOptions {
	n.panicIfNotOurs(brackets)

	return wrapOptions(n.Context, n.options.NewCompressed(rawCompactOptions{
		brackets: brackets.ID(),
	}))
}

// panicIfNotOurs checks that a contextual value is owned by this context, and panics if not.
//
// Does not panic if that is zero or has a zero context. Panics if n is zero.
func (n *Nodes) panicIfNotOurs(that ...any) {
	for _, that := range that {
		if that == nil {
			continue
		}

		var thatCtx token.Context
		switch that := that.(type) {
		case interface{ Context() token.Context }:
			thatCtx = that.Context()
			if thatCtx == nil || thatCtx == n.Context {
				continue
			}
		case interface{ Context() Context }:
			thatCtx = that.Context()
			if thatCtx == nil || thatCtx == n.Context {
				continue
			}
		default:
			continue
		}

		panic(fmt.Sprintf(
			"protocompile/ast: attempt to mix different contexts: %q vs %q",
			n.Context.Stream().Path(),
			thatCtx.Stream().Path(),
		))
	}
}
