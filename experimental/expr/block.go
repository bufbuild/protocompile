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

package expr

import (
	"github.com/bufbuild/protocompile/experimental/id"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/experimental/token"
)

// Block := `{` (Expr (`;` | `\n`))* Expr? `}`.
type Block id.Node[Block, *Context, *rawBlock]

// BlockArgs is arguments for [Nodes.NewBlock].
type BlockArgs struct {
	Braces token.Token
}

type rawBlock struct {
	braces token.ID
	tags   id.DynSeq[Expr, Kind, *Context]
}

// AsAny type-erases this type value.
//
// See [Expr] for more information.
func (e Block) AsAny() Expr {
	if e.IsZero() {
		return Expr{}
	}
	return id.WrapDyn(e.Context(), id.NewDyn(KindBlock, id.ID[Expr](e.ID())))
}

// Braces returns the braces that surround this block.
func (e Block) Braces() token.Token {
	if e.IsZero() {
		return token.Zero
	}
	return id.Wrap(e.Context().Stream(), e.Raw().braces)
}

// Exprs returns an inserter over the expressions in this block.
func (e Block) Exprs() seq.Inserter[Expr] {
	var tags *id.DynSeq[Expr, Kind, *Context]
	if !e.IsZero() {
		tags = &e.Raw().tags
	}
	return tags.Inserter(e.Context())
}

// Span implements [source.Spanner].
func (e Block) Span() source.Span {
	return e.Braces().Span()
}
