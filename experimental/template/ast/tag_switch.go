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
	"github.com/bufbuild/protocompile/experimental/id"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/experimental/token"
)

// TagSwitch is a conditional tag, which evaluates to one of the cases contained
// within.
//
// # Grammar
//
//	TagSwitch := `[:` `switch` Expr? `:]` TagCase* TagEnd
type TagSwitch id.Node[TagSwitch, *File, *rawTagSwitch]

// TagSwitchArgs is arguments for [Nodes.NewTagSwitch].
type TagSwitchArgs struct {
	Brackets token.Token
	Keyword  token.Token
	Expr     ExprAny
	End      TagEnd
}

type rawTagSwitch struct {
	brackets token.ID
	keyword  token.ID

	expr  id.Dyn[ExprAny, ExprKind]
	cases []id.ID[TagCase]
	end   id.ID[TagEnd]
}

// TagCase is one of the cases within a [TagSwitch].
//
// # Grammar
//
//	TagCase   := `[:` (`case` Params) | `else` `:]` Fragment TagEnd
type TagCase id.Node[TagCase, *File, *rawTagCase]

// TagCaseArgs is arguments for [Nodes.NewTagCase].
type TagCaseArgs struct {
	Brackets token.Token
	Keyword  token.Token
	Params   Params
	Fragment Fragment
	End      TagEnd
}

type rawTagCase struct {
	brackets token.ID
	keyword  token.ID
	params   id.ID[Params]
	fragment id.ID[Fragment]
	end      id.ID[TagEnd]
}

// AsAny type-erases this tag value.
//
// See [TagAny] for more information.
func (t TagSwitch) AsAny() TagAny {
	if t.IsZero() {
		return TagAny{}
	}
	return id.WrapDyn(t.Context(), id.NewDyn(TagKindSwitch, id.ID[TagAny](t.ID())))
}

// Brackets returns the bracket token that makes up this tag.
func (t TagSwitch) Brackets() token.Token {
	if t.IsZero() {
		return token.Zero
	}
	return id.Wrap(t.Context().Stream(), t.Raw().brackets)
}

// Keywords returns the "switch" keyword for this tag.
func (t TagSwitch) Keyword() token.Token {
	if t.IsZero() {
		return token.Zero
	}
	return id.Wrap(t.Context().Stream(), t.Raw().keyword)
}

// Expr returns the expression being switched on.
//
// May be zero if one was not specified (this is valid syntax).
func (t TagSwitch) Expr() ExprAny {
	if t.IsZero() {
		return ExprAny{}
	}

	return id.WrapDyn(t.Context(), t.Raw().expr)
}

// Cases returns the case tags within this tag.
func (t TagSwitch) Cases() seq.Indexer[TagCase] {
	var cases *[]id.ID[TagCase]
	if t.IsZero() {
		cases = &t.Raw().cases
	}
	return seq.NewSliceInserter(
		cases,
		func(_ int, c id.ID[TagCase]) TagCase {
			return id.Wrap(t.Context(), c)
		},
		func(_ int, e TagCase) id.ID[TagCase] {
			e.Context().Nodes().panicIfNotOurs(e)
			return e.ID()
		},
	)
}

// EndTag returns the end tag for this tag's fragment.
func (t TagSwitch) End() TagEnd {
	if t.IsZero() {
		return TagEnd{}
	}

	return id.Wrap(t.Context(), t.Raw().end)
}

// Span implements [source.Spanner].
func (t TagSwitch) Span() source.Span {
	return source.Join(t.Brackets(), t.End())
}

// AsAny type-erases this tag value.
//
// See [TagAny] for more information.
func (t TagCase) AsAny() TagAny {
	if t.IsZero() {
		return TagAny{}
	}
	return id.WrapDyn(t.Context(), id.NewDyn(TagKindCase, id.ID[TagAny](t.ID())))
}

// Brackets returns the bracket token that makes up this tag.
func (t TagCase) Brackets() token.Token {
	if t.IsZero() {
		return token.Zero
	}
	return id.Wrap(t.Context().Stream(), t.Raw().brackets)
}

// Keywords returns the "switch" keyword for this tag.
func (t TagCase) Keyword() token.Token {
	if t.IsZero() {
		return token.Zero
	}
	return id.Wrap(t.Context().Stream(), t.Raw().keyword)
}

// Params returns the case parameters for this tag.
func (t TagCase) Params() Params {
	if t.IsZero() {
		return Params{}
	}

	return id.Wrap(t.Context(), t.Raw().params)
}

// Fragment returns the fragment inside of this tag.
func (t TagCase) Fragment() Fragment {
	if t.IsZero() {
		return Fragment{}
	}

	return id.Wrap(t.Context(), t.Raw().fragment)
}

// EndTag returns the end tag for this tag's fragment.
func (t TagCase) End() TagEnd {
	if t.IsZero() {
		return TagEnd{}
	}

	return id.Wrap(t.Context(), t.Raw().end)
}

// Span implements [source.Spanner].
func (t TagCase) Span() source.Span {
	return source.Join(t.Brackets(), t.End())
}
