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

// TagMacro is a tag which defines a macro: a function whose body is a fragment.
//
// # Grammar
//
//	TagMacro := `[:` `macro` token.Ident `(` Params `)` `:]` Fragment TagEnd
type TagMacro id.Node[TagMacro, *File, *rawTagMacro]

// TagMacroArgs sis arguments for [Nodes.NewTagMacro].
type TagMacroArgs struct {
	Brackets token.Token
	Keyword  token.Token
	Name     token.Token
	Params   Params
	Fragment Fragment
	End      TagEnd
}

type rawTagMacro struct {
	brackets token.ID
	keyword  token.ID

	name     token.ID
	params   id.ID[Params]
	fragment id.ID[Fragment]
	end      id.ID[TagEnd]
}

// AsAny type-erases this tag value.
//
// See [TagAny] for more information.
func (t TagMacro) AsAny() TagAny {
	if t.IsZero() {
		return TagAny{}
	}
	return id.WrapDyn(t.Context(), id.NewDyn(TagKindMacro, id.ID[TagAny](t.ID())))
}

// Brackets returns the bracket token that makes up this tag.
func (t TagMacro) Brackets() token.Token {
	if t.IsZero() {
		return token.Zero
	}
	return id.Wrap(t.Context().Stream(), t.Raw().brackets)
}

// Keyword returns the "macro" keyword for this tag.
func (t TagMacro) Keyword() token.Token {
	if t.IsZero() {
		return token.Zero
	}
	return id.Wrap(t.Context().Stream(), t.Raw().keyword)
}

// Name returns the name for this macro.
//
// May be zero if the user forgot it.
func (t TagMacro) Name() token.Token {
	if t.IsZero() {
		return token.Zero
	}

	return id.Wrap(t.Context().Stream(), t.Raw().name)
}

// Params returns the iteration parameters for this tag.
func (t TagMacro) Params() Params {
	if t.IsZero() {
		return Params{}
	}

	return id.Wrap(t.Context(), t.Raw().params)
}

// Fragment returns the fragment inside of this tag.
func (t TagMacro) Fragment() Fragment {
	if t.IsZero() {
		return Fragment{}
	}

	return id.Wrap(t.Context(), t.Raw().fragment)
}

// EndTag returns the end tag for this tag's fragment.
func (t TagMacro) End() TagEnd {
	if t.IsZero() {
		return TagEnd{}
	}

	return id.Wrap(t.Context(), t.Raw().end)
}

// Span implements [source.Spanner].
func (t TagMacro) Span() source.Span {
	return source.Join(t.Brackets(), t.End())
}
