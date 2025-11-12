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
	"strings"

	"github.com/bufbuild/protocompile/experimental/ast/predeclared"
	"github.com/bufbuild/protocompile/experimental/id"
	"github.com/bufbuild/protocompile/experimental/source"
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

	raw PathID
}

// PathID identifies a [Path] in a [Context].
type PathID struct {
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
	start, end token.ID
}

// In wraps this ID with a context.
func (p PathID) In(f *File) Path {
	if p.start.IsZero() {
		return Path{}
	}

	if p.end.IsZero() {
		panic(fmt.Sprintf("protocompile/ast: invalid ast.Path representation %v; this is a bug in protocompile", p))
	}

	return Path{id.WrapContext(f), p}
}

// ID returns this path's ID.
func (p Path) ID() PathID {
	return p.raw
}

// Absolute returns whether this path starts with a dot.
func (p Path) Absolute() bool {
	first, ok := iterx.First(p.Components)
	return ok && !first.Separator().IsZero()
}

// IsSynthetic returns whether this path was created with [Nodes.NewPath].
func (p Path) IsSynthetic() bool {
	return p.raw.start < 0
}

// ToRelative converts this path into a relative path, by deleting all leading
// separators. In particular, the path "..foo", which contains empty components,
// will be converted into "foo".
//
// If called on zero or a relative path, returns p.
func (p Path) ToRelative() Path {
	for pc := range p.Components {
		if !pc.IsEmpty() {
			p.raw.start = pc.name
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

// IsIdents returns whether p is a sequence of exactly the given identifiers.
func (p Path) IsIdents(idents ...string) bool {
	for i, pc := range iterx.Enumerate(p.Components) {
		if i >= len(idents) || pc.AsIdent().Text() != idents[i] {
			break
		}

		if i == len(idents)-1 {
			return true
		}
	}
	return false
}

// source.Span implements [source.Spanner].
func (p Path) Span() source.Span {
	// No need to check for zero here, if p is zero both start and end will be
	// zero tokens.
	return source.Join(
		id.Wrap(p.Context().Stream(), p.raw.start),
		id.Wrap(p.Context().Stream(), p.raw.end),
	)
}

// Components is an [iter.Seq] that ranges over each component in this path.
// Specifically, it yields the (possibly zero) dot that precedes the component,
// and the identifier token.
func (p Path) Components(yield func(PathComponent) bool) {
	if p.IsZero() {
		return
	}

	var cursor *token.Cursor
	first := id.Wrap(p.Context().Stream(), p.raw.start)
	if p.IsSynthetic() {
		cursor = first.SyntheticChildren(p.raw.synthRange())
	} else {
		cursor = token.NewCursorAt(first)
	}

	var sep token.Token
	var idx uint32
	for tok := range cursor.Rest() {
		if !p.IsSynthetic() && tok.ID() > p.raw.end {
			// We've reached the end of the path.
			break
		}

		if tok.Text() == "." || tok.Text() == "/" {
			if !sep.IsZero() {
				// Uh-oh, empty path component!
				if !yield(PathComponent{p.withContext, p.raw, sep.ID(), 0, idx}) {
					return
				}
				idx++
			}
			sep = tok
			continue
		}

		if !yield(PathComponent{p.withContext, p.raw, sep.ID(), tok.ID(), idx}) {
			return
		}
		idx++
		sep = token.Zero
	}
	if !sep.IsZero() {
		yield(PathComponent{p.withContext, p.raw, sep.ID(), 0, idx})
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
	var found bool
	for pc := range p.Components {
		if n > 0 {
			prev = pc
			n--
			if !pc.Separator().IsZero() {
				i++
			}
			if !pc.Name().IsZero() {
				i++
			}
			continue
		}

		prefix, suffix = p, p
		found = true

		if p.IsSynthetic() {
			a, _ := prefix.raw.synthRange()
			prefix.raw = prefix.raw.withSynthRange(a, a+i)

			a, b := suffix.raw.synthRange()
			a += i
			suffix.raw = suffix.raw.withSynthRange(a, b)

			continue
		}

		if !prev.name.IsZero() {
			prefix.raw.end = prev.name
		} else {
			prefix.raw.end = prev.separator
		}

		if !pc.separator.IsZero() {
			suffix.raw.start = pc.separator
		} else {
			suffix.raw.start = pc.name
		}

		break
	}

	if !found {
		return p, Path{}
	}

	return prefix, suffix
}

// Canonicalized returns a string containing this path's value after
// canonicalization.
//
// Canonicalization converts a path into something that can be used for name
// resolution. This includes removing extra separators and deleting whitespace
// and comments.
func (p Path) Canonicalized() string {
	// Most paths are already in canonical form. Verify this before allocating
	// a fresh string.
	if id := p.AsIdent(); !id.IsZero() {
		return id.Name()
	} else if p.isCanonical() {
		return p.Span().Text()
	}

	var out strings.Builder
	p.canonicalized(&out)
	return out.String()
}

func (p Path) canonicalized(out *strings.Builder) {
	for i, pc := range iterx.Enumerate(p.Components) {
		if pc.Name().IsZero() {
			continue
		}

		if i > 0 || !pc.Separator().IsZero() {
			out.WriteString(pc.Separator().Text())
		}
		if id := pc.Name(); !id.IsZero() {
			out.WriteString(id.Name())
		} else {
			out.WriteByte('(')
			pc.AsExtension().canonicalized(out)
			out.WriteByte(')')
		}
	}
}

func (p Path) isCanonical() bool {
	var prev PathComponent
	for pc := range p.Components {
		sep := pc.Separator()
		name := pc.Name()

		if name.IsZero() {
			return false
		}
		if !sep.IsZero() && sep.Span().End != name.Span().Start {
			return false
		}

		if extn := pc.AsExtension(); !extn.IsZero() {
			if !extn.isCanonical() {
				return false
			}

			// Ensure that the parens tightly wrap extn.
			parens := name.Span()
			extn := extn.Span()
			if parens.Start+1 != extn.Start || parens.End-1 != extn.End {
				return false
			}
		} else if pc.AsIdent().Text() != pc.AsIdent().Name() {
			return false
		}

		if !prev.IsZero() {
			if sep.IsZero() {
				return false
			}
			if prev.Name().Span().End != sep.Span().Start {
				return false
			}
		}

		prev = pc
	}

	return true
}

// trim discards any skippable tokens before and after the start of this path.
func (p Path) trim() Path {
	for p.raw.start < p.raw.end &&
		id.Wrap(p.Context().Stream(), p.raw.start).Kind().IsSkippable() {
		p.raw.start++
	}
	for p.raw.start < p.raw.end &&
		id.Wrap(p.Context().Stream(), p.raw.end).Kind().IsSkippable() {
		p.raw.end--
	}

	if p.raw.start <= p.raw.end {
		return p
	}

	return Path{}
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
	return id.WrapDyn(t.Context(), id.NewDynFromRaw[TypeAny, TypeKind](int32(t.raw.start), int32(t.raw.end)))
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
	return id.WrapDyn(e.Context(), id.NewDynFromRaw[ExprAny, ExprKind](int32(e.raw.start), int32(e.raw.end)))
}

// PathComponent is a piece of a path. This is either an identifier or a nested path
// (for an extension name).
type PathComponent struct {
	withContext
	path            PathID
	separator, name token.ID
	idx             uint32
}

// Path returns the path that this component is part of.
func (p PathComponent) Path() Path {
	return Path{p.withContext, p.path}
}

// IsFirst returns whether this is the first component of its path.
func (p PathComponent) IsFirst() bool {
	if p.Path().IsSynthetic() {
		return p.idx == 0
	}
	return p.separator == p.path.start || p.name == p.path.start
}

// IsLast returns whether this is the last component of its path.
func (p PathComponent) IsLast() bool {
	if p.Path().IsSynthetic() {
		i, j := p.path.synthRange()
		return int(p.idx) == j-i
	}
	return p.separator == p.path.end || p.name == p.path.end
}

// SplitBefore splits the path that this component came from around the
// component boundary before this component.
//
// after's first component will be this component.
//
// Not currently implemented for synthetic paths.
func (p PathComponent) SplitBefore() (before, after Path) {
	if p.IsFirst() {
		return Path{}, p.Path()
	}

	if p.Path().IsSynthetic() {
		panic("protocompile/ast: called PathComponent.SplitBefore with synthetic path")
	}

	prefix, suffix := p.Path(), p.Path()
	if p.separator.IsZero() {
		prefix.raw.end = id.Wrap(p.Context().Stream(), p.name).Prev().ID()
		suffix.raw.start = p.name
	} else {
		prefix.raw.end = id.Wrap(p.Context().Stream(), p.separator).Prev().ID()
		suffix.raw.start = p.separator
	}

	return prefix.trim(), suffix.trim()
}

// SplitAfter splits the path that this component came from around the
// component boundary after this component.
//
// before's last component will be this component.
func (p PathComponent) SplitAfter() (before, after Path) {
	if p.IsLast() {
		return p.Path(), Path{}
	}

	if p.Path().IsSynthetic() {
		panic("protocompile/ast: called PathComponent.SplitAfter with synthetic path")
	}

	prefix, suffix := p.Path(), p.Path()
	if !p.name.IsZero() {
		prefix.raw.end = p.name
		suffix.raw.start = id.Wrap(p.Context().Stream(), p.name).Next().ID()
	} else {
		prefix.raw.end = p.separator
		suffix.raw.start = id.Wrap(p.Context().Stream(), p.separator).Next().ID()
	}

	return prefix.trim(), suffix.trim()
}

// Separator is the token that separates this component from the previous one, if
// any. This may be a dot or a slash.
func (p PathComponent) Separator() token.Token {
	return id.Wrap(p.Context().Stream(), p.separator)
}

// Name is the token that represents this component's name. THis is either an
// identifier or a (...) token containing a path.
func (p PathComponent) Name() token.Token {
	return id.Wrap(p.Context().Stream(), p.name)
}

// Returns whether this is an empty path component. Such components are not allowed
// in the grammar but may occur in invalid inputs nonetheless.
func (p PathComponent) IsEmpty() bool {
	return p.Name().IsZero()
}

// Next returns the next path component after this one, if there is one.
func (p PathComponent) Next() PathComponent {
	_, after := p.SplitAfter()
	next, _ := iterx.First(after.Components)
	return next
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
		return Path{p.withContext, PathID{p.Name().ID(), 0}}
	}

	// Find the first and last non-skippable tokens to be the bounds.
	var first, last token.Token
	for tok := range p.Name().Children().Rest() {
		if first.IsZero() {
			first = tok
		}
		last = tok
	}

	return PathID{first.ID(), last.ID()}.In(p.Context())
}

// AsIdent returns the single identifier that makes up this path component, if
// it is not an extension path component.
//
// May be zero, in the case of e.g. the second component of foo..bar.
func (p PathComponent) AsIdent() token.Token {
	tok := id.Wrap(p.Context().Stream(), p.name)
	if tok.Kind() == token.Ident {
		return tok
	}
	return token.Zero
}

// Span implements [source.Spanner].
func (p PathComponent) Span() source.Span {
	return source.Join(p.Separator(), p.Name())
}

func (p PathID) synthRange() (start, end int) {
	return int(^uint16(p.end)), int(^uint16(p.end >> 16))
}

func (p PathID) withSynthRange(start, end int) PathID {
	p.end = token.ID(^uint16(start)) | (token.ID(^uint16(end)) << 16)
	return p
}
