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

// TagText is raw text between non-text tags.
type TagText id.Node[TagText, *File, *rawTagText]

// TagTextArgs is arguments for [Nodes.NewTagText].
type TagTextArgs struct {
	Text token.Token
}

type rawTagText struct {
	text token.ID
}

// AsAny type-erases this tag value.
//
// See [TagAny] for more information.
func (t TagText) AsAny() TagAny {
	if t.IsZero() {
		return TagAny{}
	}
	return id.WrapDyn(t.Context(), id.NewDyn(TagKindText, id.ID[TagAny](t.ID())))
}

// Token returns the token which contains the actual text of this tag.
func (t TagText) Token() token.Token {
	if t.IsZero() {
		return token.Zero
	}
	return id.Wrap(t.Context().Stream(), t.Raw().text)
}

// Span implements [source.Spanner].
func (t TagText) Span() source.Span {
	return t.Token().Span()
}
