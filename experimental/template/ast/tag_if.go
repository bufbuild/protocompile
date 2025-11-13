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

// TagIf is a conditional tag, which evaluates to either itself or a following
// TagIf depending on the result of the condition.
//
// A TagIf node represents only one branch of a conditional: the else branch
// can be found under [TagIf.Else].
//
// # Grammar
//
//	TagIf   := `[:` `if` Expr `:]` Fragment TagElse
//	TagElse := TagEnd
//	  | `[:` `else` `if` Expr `:]` Fragment TagElse
//	  | `[:` `else` `:]` Fragment TagEnd
type TagIf id.Node[TagIf, *File, *rawTagIf]

// TagIfArgs sis arguments for [Nodes.NewTagIf].
type TagIfArgs struct {
	Brackets token.Token
	Keyword  token.Token
	FilePath ExprAny
	Fragment Fragment
	End      TagEnd
}

type rawTagIf struct {
	brackets     token.ID
	ifKw, elseKw token.ID

	cond     id.Dyn[ExprAny, ExprKind]
	fragment id.ID[Fragment]
	elseTag  id.Dyn[TagAny, TagKind]
}

// AsAny type-erases this tag value.
//
// See [TagAny] for more information.
func (t TagIf) AsAny() TagAny {
	if t.IsZero() {
		return TagAny{}
	}
	return id.WrapDyn(t.Context(), id.NewDyn(TagKindIf, id.ID[TagAny](t.ID())))
}

// Brackets returns the bracket token that makes up this tag.
func (t TagIf) Brackets() token.Token {
	if t.IsZero() {
		return token.Zero
	}
	return id.Wrap(t.Context().Stream(), t.Raw().brackets)
}

// Keywords returns the keywords for this tag, which may be one of else, if,
// or both (for else if tags).
func (t TagIf) Keywords() (else_, if_ token.Token) {
	if t.IsZero() {
		return token.Zero, token.Zero
	}
	return id.Wrap(t.Context().Stream(), t.Raw().elseKw),
		id.Wrap(t.Context().Stream(), t.Raw().ifKw)
}

// Condition returns the condition expression for this branch.
//
// May be zero if this is an [:else:] tag.
func (t TagIf) Condition() ExprAny {
	if t.IsZero() {
		return ExprAny{}
	}

	return id.WrapDyn(t.Context(), t.Raw().cond)
}

// Fragment returns the fragment inside of this tag.
func (t TagIf) Fragment() Fragment {
	if t.IsZero() {
		return Fragment{}
	}

	return id.Wrap(t.Context(), t.Raw().fragment)
}

// Else returns the end tag for this tag's fragment; this may be either another
// TagIf, or a [TagEnd] if this is the last branch of the if.
func (t TagIf) Else() TagAny {
	if t.IsZero() {
		return TagAny{}
	}

	return id.WrapDyn(t.Context(), t.Raw().elseTag)
}

// Span implements [source.Spanner].
func (t TagIf) Span() source.Span {
	return source.Join(t.Brackets(), t.Else())
}
