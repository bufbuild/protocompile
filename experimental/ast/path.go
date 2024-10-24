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

import "github.com/bufbuild/protocompile/experimental/ast/predeclared"

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
type rawPath [2]rawToken

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
func (p Path) AsIdent() Token {
	var tok Token
	var count int
	p.Components(func(c PathComponent) bool {
		if count > 0 {
			tok = Token{}
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

// Span implements [Spanner].
func (p Path) Span() Span {
	return JoinSpans(p.raw[0].With(p), p.raw[1].With(p))
}

// Components is an [iter.Seq] that ranges over each component in this path. Specifically,
// it yields the (nilable) dot that precedes the component, and the identifier token.
func (p Path) Components(yield func(PathComponent) bool) {
	if p.Nil() {
		return
	}

	first := p.raw[0].With(p)
	if synth := first.synthetic(); synth != nil {
		panic("synthetic paths are not implemented yet")
	}

	cursor := Cursor{
		withContext: p.withContext,
		start:       p.raw[0],
		end:         p.raw[1] + 1, // Remember, Cursor.end is exclusive!
	}

	var sep Token
	var broken bool
	cursor.Iter(func(tok Token) bool {
		if tok.Text() == "." || tok.Text() == "/" {
			if !sep.Nil() {
				// Uh-oh, empty path component!
				if !yield(PathComponent{sep, Token{}}) {
					broken = true
					return false
				}
			}
			sep = tok
			return true
		}

		if !yield(PathComponent{sep, tok}) {
			broken = true
			return false
		}
		sep = Token{}
		return true
	})
	if !broken && !sep.Nil() {
		yield(PathComponent{sep, Token{}})
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
	separator, name Token
}

// Separator is the token that separates this component from the previous one, if
// any. This may be a dot or a slash.
func (p PathComponent) Separator() Token {
	return p.separator
}

// Name is the token that represents this component's name. THis is either an
// identifier or a (...) token containing a path.
func (p PathComponent) Name() Token {
	return p.name
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
	if synth := p.Name().synthetic(); synth != nil {
		return Path{withContext{p.Name().Context()}, rawPath{p.Name().raw, 0}}
	}

	// Find the first and last non-skippable tokens to be the bounds.
	var first, last Token
	p.Name().Children().Iter(func(token Token) bool {
		if first.Nil() {
			first = token
		}
		last = token
		return true
	})

	return Path{withContext{p.Name().Context()}, rawPath{first.raw, last.raw}}
}

// AsIdent returns the single identifier that makes up this path component, if
// it is not an extension path component.
func (p PathComponent) AsIdent() Token {
	if p.IsExtension() {
		return Token{}
	}
	return p.name
}

// Wrap wraps this rawPath with a context to present to the user.
func (p rawPath) With(c Contextual) Path {
	if p[0] == 0 {
		return Path{}
	}

	return Path{withContext{c.Context()}, p}
}
