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

// TagEmit specifies a new file to emit as part of template evaluation.
//
// # Grammar
//
//	TagEmit := `[:` `emit` Expr `:]`
type TagEmit id.Node[TagEmit, *File, *rawTagEmit]

// TagEmitArgs sis arguments for [Nodes.NewTagEmit].
type TagEmitArgs struct {
	Brackets token.Token
	Keyword  token.Token
	FilePath ExprAny
	Fragment Fragment
	End      TagEnd
}

type rawTagEmit struct {
	brackets token.ID
	keyword  token.ID

	filePath id.Dyn[ExprAny, ExprKind]
	fragment id.ID[Fragment]
	end      id.ID[TagEnd]
}

// AsAny type-erases this tag value.
//
// See [TagAny] for more information.
func (t TagEmit) AsAny() TagAny {
	if t.IsZero() {
		return TagAny{}
	}
	return id.WrapDyn(t.Context(), id.NewDyn(TagKindEmit, id.ID[TagAny](t.ID())))
}

// Brackets returns the bracket token that makes up this tag.
func (t TagEmit) Brackets() token.Token {
	if t.IsZero() {
		return token.Zero
	}
	return id.Wrap(t.Context().Stream(), t.Raw().brackets)
}

// Keyword returns the "import" keyword token.
func (t TagEmit) Keyword() token.Token {
	if t.IsZero() {
		return token.Zero
	}
	return id.Wrap(t.Context().Stream(), t.Raw().keyword)
}

// FilePath returns the path to emit the file at.
//
// May be zero, if the user forgot it.
func (t TagEmit) FilePath() ExprAny {
	if t.IsZero() {
		return ExprAny{}
	}

	return id.WrapDyn(t.Context(), t.Raw().filePath)
}

// Fragment returns the fragment inside of this tag.
func (t TagEmit) Fragment() Fragment {
	if t.IsZero() {
		return Fragment{}
	}

	return id.Wrap(t.Context(), t.Raw().fragment)
}

// EndTag returns the end tag for this tag's fragment.
func (t TagEmit) End() TagEnd {
	if t.IsZero() {
		return TagEnd{}
	}

	return id.Wrap(t.Context(), t.Raw().end)
}

// Span implements [source.Spanner].
func (t TagEmit) Span() source.Span {
	return source.Join(t.Brackets(), t.End())
}
