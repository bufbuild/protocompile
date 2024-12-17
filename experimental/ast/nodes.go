// Copyright 2020-2024 Buf Technologies, Inc.
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
	"runtime"
	"strings"

	"github.com/bufbuild/protocompile/experimental/internal"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/internal/arena"
	"github.com/bufbuild/protocompile/internal/ext/unsafex"
)

// Nodes provides storage for the various AST node types, and can be used
// to construct new ones.
type Nodes struct {
	// The context for these nodes.
	Context Context

	// If set, this will cause any call that constructs a new AST node to log
	// the stack trace of the caller. Those stack traces can later be recalled
	// by calling the Trace method on an AST node.
	//
	// Tracing is best effort: some node types are currently unable to record
	// a creation site.
	//
	// Enabling this feature will result in significant parser slowdown; it is
	// intended for debugging only.
	EnableTracing bool

	decls   decls
	types   types
	exprs   exprs
	options arena.Arena[rawCompactOptions]

	// Map of arena pointer addresses to recorded stack traces. We use a
	// uintptr because all of the nodes associated with this context live
	// have the same lifetime as this map: they are not freed (allowing their
	// address to be reused) until traces is also freed.
	traces  map[uintptr]string
	scratch []uintptr // Reusable scratch space for traceNode.
}

// Root returns the root AST node for this context.
func (n *Nodes) Root() File {
	// NewContext() sticks the root at the beginning of decls.body for us, so
	// there is always a DeclBody at index 0, which corresponds to the whole
	// file. We use a 1 here, not a 0, because arena.Arena's indices are
	// off-by-one to accommodate the nil representation.
	return File{wrapDeclBody(n.Context, 1)}
}

// NewDeclEmpty creates a new DeclEmpty node.
func (n *Nodes) NewDeclEmpty(semicolon token.Token) DeclEmpty {
	n.panicIfNotOurs(semicolon)

	decl := wrapDeclEmpty(n.Context, newNode(n, &n.decls.empties, rawDeclEmpty{
		semi: semicolon.ID(),
	}))

	return decl
}

// NewDeclSyntax creates a new DeclSyntax node.
func (n *Nodes) NewDeclSyntax(args DeclSyntaxArgs) DeclSyntax {
	n.panicIfNotOurs(args.Keyword, args.Equals, args.Value, args.Options, args.Semicolon)

	return wrapDeclSyntax(n.Context, newNode(n, &n.decls.syntaxes, rawDeclSyntax{
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

	return wrapDeclPackage(n.Context, newNode(n, &n.decls.packages, rawDeclPackage{
		keyword: args.Keyword.ID(),
		path:    args.Path.raw,
		options: n.options.Compress(args.Options.raw),
		semi:    args.Semicolon.ID(),
	}))
}

// NewDeclImport creates a new DeclImport node.
func (n *Nodes) NewDeclImport(args DeclImportArgs) DeclImport {
	n.panicIfNotOurs(args.Keyword, args.Modifier, args.ImportPath, args.Options, args.Semicolon)

	return wrapDeclImport(n.Context, newNode(n, &n.decls.imports, rawDeclImport{
		keyword:    args.Keyword.ID(),
		modifier:   args.Modifier.ID(),
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
	if !args.Type.Nil() {
		raw.ty = args.Type.raw
	} else {
		kw := rawPath{args.Keyword.ID(), args.Keyword.ID()}.With(n.Context)
		raw.ty = wrapPath[TypeKind](kw.raw)
	}
	if !args.Returns.Nil() {
		raw.signature = &rawSignature{
			returns: args.Returns.ID(),
		}
	}

	return wrapDeclDef(n.Context, newNode(n, &n.decls.defs, raw))
}

// NewDeclBody creates a new DeclBody node.
//
// To add declarations to the returned body, use [DeclBody.Append].
func (n *Nodes) NewDeclBody(braces token.Token) DeclBody {
	n.panicIfNotOurs(braces)

	return wrapDeclBody(n.Context, newNode(n, &n.decls.bodies, rawDeclBody{
		braces: braces.ID(),
	}))
}

// NewDeclRange creates a new DeclRange node.
//
// To add ranges to the returned declaration, use [DeclRange.Append].
func (n *Nodes) NewDeclRange(args DeclRangeArgs) DeclRange {
	n.panicIfNotOurs(args.Keyword, args.Options, args.Semicolon)

	return wrapDeclRange(n.Context, newNode(n, &n.decls.ranges, rawDeclRange{
		keyword: args.Keyword.ID(),
		options: n.options.Compress(args.Options.raw),
		semi:    args.Semicolon.ID(),
	}))
}

// NewExprPrefixed creates a new ExprPrefixed node.
func (n *Nodes) NewExprPrefixed(args ExprPrefixedArgs) ExprPrefixed {
	n.panicIfNotOurs(args.Prefix, args.Expr)

	ptr := newNode(n, &n.exprs.prefixes, rawExprPrefixed{
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

	ptr := newNode(n, &n.exprs.ranges, rawExprRange{
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

	ptr := newNode(n, &n.exprs.arrays, rawExprArray{
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

	ptr := newNode(n, &n.exprs.dicts, rawExprDict{
		braces: braces.ID(),
	})
	return ExprDict{exprImpl[rawExprDict]{
		internal.NewWith(n.Context),
		n.exprs.dicts.Deref(ptr),
	}}
}

// NewExprPrefixed creates a new ExprPrefixed node.
func (n *Nodes) NewExprKV(args ExprFieldArgs) ExprField {
	n.panicIfNotOurs(args.Key, args.Colon, args.Value)

	ptr := newNode(n, &n.exprs.fields, rawExprField{
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

	ptr := newNode(n, &n.types.prefixes, rawTypePrefixed{
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

	ptr := newNode(n, &n.types.generics, rawTypeGeneric{
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

	return wrapOptions(n.Context, newNode(n, &n.options, rawCompactOptions{
		brackets: brackets.ID(),
	}))
}

// panicIfNotOurs checks that a contextual value is owned by this context, and panics if not.
//
// Does not panic if that is nil or has a nil context. Panics if n is nil.
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

// newNode creates a new node in the given arena, recording debugging information
// on n as it does so.
//
// This function wants to be a method of Nodes, but can't because it's generic.
func newNode[T any](n *Nodes, arena *arena.Arena[T], value T) arena.Pointer[T] {
	p := arena.NewCompressed(value)
	if n.EnableTracing {
		traceNode(n, arena, p) // Outlined to promote inlining of newNode.
	}
	return p
}

// traceNode inserts a backtrace to the caller of newNode as the backtrace for
// the node at p.
func traceNode[T any](n *Nodes, arena *arena.Arena[T], p arena.Pointer[T]) {
	if n.scratch == nil {
		// NOTE: If spending four words on traces + scratch turns out to be
		// wasteful, we can instead store this slice in traces itself, behind
		// the uintptr value 1, which no pointer uses as its address.
		n.scratch = make([]uintptr, 256)
	}

	var buf strings.Builder
	// 0 means runtime.Callers, 1 means traceNode, and 2 means newNode. Thus,
	// we want 3 for the caller of newNode.
	trace := n.scratch[:runtime.Callers(3, n.scratch)]
	frames := runtime.CallersFrames(trace)
	for {
		frame, more := frames.Next()
		fmt.Fprintf(&buf, "at %s\n  %s:%d\n", frame.Function, frame.File, frame.Line)
		if !more {
			break
		}
	}

	if n.traces == nil {
		n.traces = make(map[uintptr]string)
	}

	n.traces[unsafex.Addr(arena.Deref(p))] = buf.String()
}
