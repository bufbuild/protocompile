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
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/experimental/token"
)

// TagFor is a looping tag, which evaluates its fragment repeatedly.
//
// # Grammar
//
//	TagFor := `[:` `for` Params `in` Expr `:]` Fragment TagEnd
type TagFor id.Node[TagFor, *File, *rawTagFor]

// TagForArgs sis arguments for [Nodes.NewTagFor].
type TagForArgs struct {
	Brackets token.Token
	Keyword  token.Token
	Params   Params
	In       token.Token
	Iterator ExprAny
	Fragment Fragment
	End      TagEnd
}

type rawTagFor struct {
	brackets    token.ID
	forKw, inKw token.ID

	iter     id.Dyn[ExprAny, ExprKind]
	params   id.ID[Params]
	fragment id.ID[Fragment]
	end      id.ID[TagEnd]
}

// AsAny type-erases this tag value.
//
// See [TagAny] for more information.
func (t TagFor) AsAny() TagAny {
	if t.IsZero() {
		return TagAny{}
	}
	return id.WrapDyn(t.Context(), id.NewDyn(TagKindFor, id.ID[TagAny](t.ID())))
}

// Brackets returns the bracket token that makes up this tag.
func (t TagFor) Brackets() token.Token {
	if t.IsZero() {
		return token.Zero
	}
	return id.Wrap(t.Context().Stream(), t.Raw().brackets)
}

// Keywords returns the keywords for this tag, which are for and in.
func (t TagFor) Keywords() (for_, in token.Token) {
	if t.IsZero() {
		return token.Zero, token.Zero
	}
	return id.Wrap(t.Context().Stream(), t.Raw().forKw),
		id.Wrap(t.Context().Stream(), t.Raw().inKw)
}

// Params returns the iteration parameters for this tag.
func (t TagFor) Params() Params {
	if t.IsZero() {
		return Params{}
	}

	return id.Wrap(t.Context(), t.Raw().params)
}

// Iterator returns the iterator expression for this loop.
//
// May be zero if the user forgot it.
func (t TagFor) Iterator() ExprAny {
	if t.IsZero() {
		return ExprAny{}
	}

	return id.WrapDyn(t.Context(), t.Raw().iter)
}

// Fragment returns the fragment inside of this tag.
func (t TagFor) Fragment() Fragment {
	if t.IsZero() {
		return Fragment{}
	}

	return id.Wrap(t.Context(), t.Raw().fragment)
}

// EndTag returns the end tag for this tag's fragment.
func (t TagFor) End() TagEnd {
	if t.IsZero() {
		return TagEnd{}
	}

	return id.Wrap(t.Context(), t.Raw().end)
}

// Span implements [source.Spanner].
func (t TagFor) Span() source.Span {
	return source.Join(t.Brackets(), t.End())
}
