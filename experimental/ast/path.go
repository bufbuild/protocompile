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

	"github.com/bufbuild/protocompile/experimental/ast/predeclared"
	"github.com/bufbuild/protocompile/experimental/internal"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/experimental/token/keyword"
	"github.com/bufbuild/protocompile/internal/ext/iterx"
)

// Path represents a multi-part identifier.
//
// This includes single identifiers like foo, references like foo.bar,
// and fully-qualified names like .foo.bar.
//
// # Grammar
//
//	Path      := `.`? component (sep component)*
//
//	component := token.Ident | `(` Path `)`
//	sep       := `.` | `/`
type Path struct {
	// The layout of this type is depended on in ast2/path.go
	withContext

	raw rawPath
}

// Absolute returns whether this path starts with a dot.
func (p Path) Absolute() bool {
	first, ok := iterx.First(p.Components)
	return ok && !first.Separator().IsZero()
}

// ToRelative converts this path into a relative path, by deleting all leading
// separators. In particular, the path "..foo", which contains empty components,
// will be converted into "foo".
//
// If called on zero or a relative path, returns p.
func (p Path) ToRelative() Path {
	for pc := range p.Components {
		if !pc.IsEmpty() {
			p.raw.Start = pc.name
			break
		}
	}
	return p
}

// AsIdent returns the single identifier that comprises this path, or
// the zero token.
func (p Path) AsIdent() token.Token {
	first, _ := iterx.OnlyOne(p.Components)
	if !first.Separator().IsZero() {
		return token.Zero
	}
	return first.AsIdent()
}

// AsPredeclared returns the [predeclared.Name] that this path represents.
//
// If this path does not represent a builtin, returns [predeclared.Unknown].
func (p Path) AsPredeclared() predeclared.Name {
	return predeclared.FromKeyword(p.AsKeyword())
}

// AsKeyword returns the [keyword.Keyword] that this path represents.
//
// If this path does not represent a builtin, returns [keyword.Unknown].
func (p Path) AsKeyword() keyword.Keyword {
	return p.AsIdent().Keyword()
}

// report.Span implements [report.Spanner].
func (p Path) Span() report.Span {
	// No need to check for zero here, if p is zero both start and end will be
	// zero tokens.
	return report.Join(p.raw.Start.In(p.Context()), p.raw.End.In(p.Context()))
}

// Components is an [iter.Seq] that ranges over each component in this path.
// Specifically, it yields the (possibly zero) dot that precedes the component,
// and the identifier token.
func (p Path) Components(yield func(PathComponent) bool) {
	if p.IsZero() {
		return
	}

	first := p.raw.Start.In(p.Context())
	if first.IsSynthetic() {
		panic("synthetic paths are not implemented yet")
	}

	var sep token.Token
	var broken bool
	for tok := range token.NewCursorAt(first).Rest() {
		if tok.ID() > p.raw.End {
			// We've reached the end of the path.
			break
		}

		if tok.Text() == "." || tok.Text() == "/" {
			if !sep.IsZero() {
				// Uh-oh, empty path component!
				if !yield(PathComponent{p.withContext, sep.ID(), 0}) {
					broken = true
					break
				}
			}
			sep = tok
			continue
		}

		if !yield(PathComponent{p.withContext, sep.ID(), tok.ID()}) {
			broken = true
			break
		}
		sep = token.Zero
	}
	if !broken && !sep.IsZero() {
		yield(PathComponent{p.withContext, sep.ID(), 0})
	}
}

// Split splits a path at the given path component index, producing two
// new paths where the first contains the first n components and the second
// contains the rest. If n is negative or greater than the number of components
// in p, both returned paths will be zero.
//
// The suffix will be absolute, except in the following cases:
// 1. n == 0 and p is not absolute (prefix will be zero and suffix will be p).
// 2. n is equal to the length of p (suffix will be zero and prefix will be p).
//
// This operation runs in O(n) time.
func (p Path) Split(n int) (prefix, suffix Path) {
	if n < 0 || p.IsZero() {
		return Path{}, Path{}
	}
	if n == 0 {
		return Path{}, p
	}

	var prev PathComponent
	for pc := range p.Components {
		if n > 0 {
			prev = pc
			n--
			continue
		}

		prefix = p
		if !pc.name.IsZero() {
			prefix.raw.End = prev.name
		} else {
			prefix.raw.End = prev.separator
		}

		suffix = p
		if !pc.separator.IsZero() {
			suffix.raw.Start = pc.separator
		} else {
			suffix.raw.Start = pc.name
		}

		break
	}

	return prefix, suffix
}

// TypePath is a simple path reference as a type.
//
// # Grammar
//
//	TypePath := Path
type TypePath struct {
	// The path that refers to this type.
	Path
}

// AsAny type-erases this type value.
//
// See [TypeAny] for more information.
func (t TypePath) AsAny() TypeAny {
	return newTypeAny(t.Context(), wrapPath[TypeKind](t.raw))
}

// ExprPath is a simple path reference in expression position.
//
// # Grammar
//
//	ExprPath := Path
type ExprPath struct {
	// The path backing this expression.
	Path
}

// AsAny type-erases this type value.
//
// See [TypeAny] for more information.
func (e ExprPath) AsAny() ExprAny {
	return newExprAny(e.Context(), wrapPath[ExprKind](e.raw))
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
	return p.Name().IsZero()
}

// AsExtension returns the Path inside of this path component, if it is an extension
// path component, i.e. (a.b.c).
//
// This is unrelated to the [foo.bar/my.Type] URL-like Any paths that appear in
// some expressions. Those are represented by allowing / as an alternative
// separator to . in paths.
func (p PathComponent) AsExtension() Path {
	if p.Name().IsZero() || p.Name().IsLeaf() {
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
		if first.IsZero() {
			first = token
		}
		last = token
		return true
	})

	return rawPath{first.ID(), last.ID()}.With(p.Context())
}

// AsIdent returns the single identifier that makes up this path component, if
// it is not an extension path component.
//
// May be zero, in the case of e.g. the second component of foo..bar.
func (p PathComponent) AsIdent() token.Token {
	tok := p.name.In(p.Context())
	if tok.Kind() == token.Ident {
		return tok
	}
	return token.Zero
}

// rawPath is the raw contents of a Path without its Context.
//
// This has one of the following configurations.
//
//  1. Two zero tokens. This is the zero path.
//
//  2. Two natural tokens. This means the path is all tokens between them including
//     the end-point
//
//  3. A single synthetic token and a zero token. If this token has children, those are
//     the path components. Otherwise, the token itself is the sole token.
//
// The case Start < 0 && End != 0 is reserved for use by pathLike.
type rawPath struct {
	Start, End token.ID
}

// With wraps this rawPath with a context to present to the user.
func (p rawPath) With(c Context) Path {
	if p.Start.IsZero() {
		return Path{}
	}

	if p.End.IsZero() {
		panic(fmt.Sprintf("protocompile/ast: invalid ast.Path representation %v; this is a bug in protocompile", p))
	}

	return Path{internal.NewWith(c), p}
}
