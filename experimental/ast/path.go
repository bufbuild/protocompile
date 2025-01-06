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

	"github.com/bufbuild/protocompile/experimental/ast/predeclared"
	"github.com/bufbuild/protocompile/experimental/internal"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/internal/ext/iterx"
	"github.com/bufbuild/protocompile/internal/ext/slicesx"
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

// IsSynthetic returns whether this path was created with [Nodes.NewPath].
func (p Path) IsSynthetic() bool {
	return p.raw.Start < 0
}

// ToRelative converts this path into a relative path, by deleting all leading
// separators. In particular, the path "..foo", which contains empty components,
// will be converted into "foo".
//
// If called on zero or a relative path, returns p.
func (p Path) ToRelative() Path {
	p.Components(func(pc PathComponent) bool {
		if pc.IsEmpty() {
			// Skip path components until we see one that is non-empty.
			return true
		}

		p.raw.Start = pc.name
		return false
	})
	return p
}

// AsIdent returns the single identifier that comprises this path, or
// the zero token.
func (p Path) AsIdent() token.Token {
	var buf [2]PathComponent
	prefix := slicesx.AppendSeq(buf[:0], iterx.Limit(2, p.Components))
	if len(prefix) != 1 || !prefix[0].Separator().IsZero() {
		return token.Zero
	}
	return prefix[0].AsIdent()
}

// AsPredeclared returns the [predeclared.Name] that this path represents.
//
// If this path does not represent a builtin, returns [BuiltinUnknown].
func (p Path) AsPredeclared() predeclared.Name {
	return predeclared.Lookup(p.AsIdent().Text())
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

	var cursor *token.Cursor
	first := p.raw.Start.In(p.Context())
	if p.IsSynthetic() {
		i := int(^int16(p.raw.End))
		j := int(^int16(p.raw.End >> 16))
		cursor = first.SyntheticChildren(i, j)
		fmt.Sprintln(cursor)
	} else {
		cursor = token.NewCursor(first, p.raw.End.In(p.Context()))
	}

	var sep token.Token
	var broken bool
	cursor.Rest()(func(tok token.Token) bool {
		fmt.Println(tok)
		if tok.Text() == "." || tok.Text() == "/" {
			if !sep.IsZero() {
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
		sep = token.Zero
		return true
	})
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

	var i int
	var prev PathComponent
	p.Components(func(pc PathComponent) bool {
		if n > 0 {
			prev = pc
			n--
			if !pc.Separator().IsZero() {
				i++
			}
			if !pc.Name().IsZero() {
				i++
			}
			return true
		}

		prefix, suffix = p, p

		if p.IsSynthetic() {
			a, _ := prefix.raw.synthRange()
			prefix.raw.setSynthRange(a, a+i)

			a, b := suffix.raw.synthRange()
			a += i
			suffix.raw.setSynthRange(a, b)

			return true
		}

		if !pc.name.IsZero() {
			prefix.raw.End = prev.name
		} else {
			prefix.raw.End = prev.separator
		}

		if !pc.separator.IsZero() {
			suffix.raw.Start = pc.separator
		} else {
			suffix.raw.Start = pc.name
		}

		return false
	})

	if prefix.raw.Start == prefix.raw.End {
		prefix = Path{}
	}
	if suffix.raw.Start == suffix.raw.End {
		suffix = Path{}
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
	return p.name.In(p.Context())
}

// rawPath is the raw contents of a Path without its Context.
//
// This has one of the following configurations.
//
//  1. Two zero tokens. This is the zero path.
//
//  2. Two natural tokens. This means the path is all tokens between them,
//     including the end-point.
//
//  3. Two synthetic tokens. The former is a an actual token, whose children
//     are the path tokens. The latter is a packed pair of uint16s representing
//     the subslice of Start.children that the path uses. This is necessary to
//     implement Split() for synthetic paths.
//
// The case Start < 0 && End > 0 is reserved for use by pathLike. The case
// Start < 0 && End == 0 is currently unused.
type rawPath struct {
	Start, End token.ID
}

func (p rawPath) synthRange() (start, end int) {
	return int(^uint16(p.End)), int(^uint16(p.End >> 16))
}

func (p *rawPath) setSynthRange(start, end int) {
	p.End = token.ID(^uint16(start)) | (token.ID(^uint16(end)) << 16)
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
