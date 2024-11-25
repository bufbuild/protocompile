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
	"github.com/bufbuild/protocompile/experimental/ast/predeclared"
	"github.com/bufbuild/protocompile/experimental/internal"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/token"
)

// Path represents a multi-part identifier.
//
// This includes single identifiers like foo, references like foo.bar,
// and fully-qualified names like .foo.bar.
type Path struct {
	withContext

	raw rawPath
}

// rawPath is the raw contents of a Path without its Context.
//
// This has one of the following configurations.
//
//  1. Two zero tokens. This is the nil path.
//
//  2. Two natural tokens. This means the path is all tokens between them including
//     the end-point
//
//  3. A single synthetic token and a nil token. If this token has children, those are
//     the path components. Otherwise, the token itself is the sole token.
//
// NOTE: Multiple compressed representations in this package depend on the fact that
// if raw[0] < 0, then raw[1] == 0 for all valid paths.
type rawPath [2]token.ID

// Absolute returns whether this path starts with a dot.
func (p Path) Absolute() bool {
	var abs bool
	p.Components(func(c PathComponent) bool {
		abs = !c.Separator().Nil()
		return false
	})
	return abs
}

// AsIdent returns the single identifier that comprises this path, or
// the nil token.
func (p Path) AsIdent() token.Token {
	var tok token.Token
	var count int
	p.Components(func(c PathComponent) bool {
		if count > 0 {
			tok = token.Nil
			return false
		}

		if c.Separator().Nil() {
			tok = c.AsIdent()
		}

		count++
		return true
	})
	return tok
}

// AsPredeclared returns the [predeclared.Name] that this path represents.
//
// If this path does not represent a builtin, returns [BuiltinUnknown].
func (p Path) AsPredeclared() predeclared.Name {
	return predeclared.Lookup(p.AsIdent().Text())
}

// report.Span implements [report.Spanner].
func (p Path) Span() report.Span {
	return report.Join(p.raw[0].In(p.Context()), p.raw[1].In(p.Context()))
}

// Components is an [iter.Seq] that ranges over each component in this path. Specifically,
// it yields the (nilable) dot that precedes the component, and the identifier token.
func (p Path) Components(yield func(PathComponent) bool) {
	if p.Nil() {
		return
	}

	first := p.raw[0].In(p.Context())
	if first.IsSynthetic() {
		panic("synthetic paths are not implemented yet")
	}

	var sep token.Token
	var broken bool
	token.NewCursor(first, p.raw[1].In(p.Context())).Rest()(func(tok token.Token) bool {
		if tok.Text() == "." || tok.Text() == "/" {
			if !sep.Nil() {
				// Uh-oh, empty path component!
				if !yield(PathComponent{p.withContext, sep.ID(), 0}) {
					broken = true
					return false
				}
			}
			sep = tok
			return true
		}

		if !yield(PathComponent{p.withContext, sep.ID(), tok.ID()}) {
			broken = true
			return false
		}
		sep = token.Nil
		return true
	})
	if !broken && !sep.Nil() {
		yield(PathComponent{p.withContext, sep.ID(), 0})
	}
}

// TypePath is a simple path reference as a type.
type TypePath struct {
	// The path that refers to this type.
	Path
}

// AsAny type-erases this type value.
//
// See [TypeAny] for more information.
func (t TypePath) AsAny() TypeAny {
	return TypeAny{
		t.Path.withContext,
		rawType(t.Path.raw),
	}
}

// TypePath is a simple path reference in expression position.
type ExprPath struct {
	// The path backing this expression.
	Path
}

// AsAny type-erases this type value.
//
// See [TypeAny] for more information.
func (e ExprPath) AsAny() ExprAny {
	return ExprAny{
		e.Path.withContext,
		rawExpr(e.Path.raw),
	}
}

// PathComponent is a piece of a path. This is either an identifier or a nested path
// (for an extension name).
type PathComponent struct {
	withContext
	separator, name token.ID
}

// Separator is the token that separates this component from the previous one, if
// any. This may be a dot or a slash.
func (p PathComponent) Separator() token.Token {
	return p.separator.In(p.Context())
}

// Name is the token that represents this component's name. THis is either an
// identifier or a (...) token containing a path.
func (p PathComponent) Name() token.Token {
	return p.name.In(p.Context())
}

// Returns whether this is an empty path component. Such components are not allowed
// in the grammar but may occur in invalid inputs nonetheless.
func (p PathComponent) IsEmpty() bool {
	return p.Name().Nil()
}

// IsExtension returns whether this path component is an extension component, i.e.
// (a.b.c).
//
// This is unrelated to the [foo.bar/my.Type] URL-like Any paths that appear in
// some expressions. Those are represented by allowing / as an alternative
// separator to . in paths.
func (p PathComponent) IsExtension() bool {
	return !p.Name().IsLeaf()
}

// AsExtension returns the Path inside of this path component, if it is an extension
// path component.
func (p PathComponent) AsExtension() Path {
	if !p.IsExtension() {
		return Path{}
	}

	// If this is a synthetic token, its children are already precisely a path,
	// so we can use the "synthetic with children" form of Path.
	if p.Name().IsSynthetic() {
		return Path{p.withContext, rawPath{p.Name().ID(), 0}}
	}

	// Find the first and last non-skippable tokens to be the bounds.
	var first, last token.Token
	p.Name().Children().Rest()(func(token token.Token) bool {
		if first.Nil() {
			first = token
		}
		last = token
		return true
	})

	return Path{p.withContext, rawPath{first.ID(), last.ID()}}
}

// AsIdent returns the single identifier that makes up this path component, if
// it is not an extension path component.
func (p PathComponent) AsIdent() token.Token {
	if p.IsExtension() {
		return token.Nil
	}
	return p.name.In(p.Context())
}

// Wrap wraps this rawPath with a context to present to the user.
func (p rawPath) With(c Context) Path {
	if p[0] == 0 {
		return Path{}
	}

	return Path{internal.NewWith(c), p}
}
