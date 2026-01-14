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
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/experimental/token"
)

// If is a chain of if conditions.
//
// Each If represents one of the blocks in the chain, which may be either an
// if, an else if, or an else. Only ifs and else ifs have conditions.
//
// # Grammar
//
//	If := `if` Expr Block (`else` (If | Block))?
type If id.Node[If, *Context, *rawIf]

// IfArgs is arguments for [Nodes.NewIf].
type IfArgs struct {
	Else, If token.Token
	Cond     Expr
	Block    Block
}

type rawIf struct {
	elseT, ifT token.ID
	cond       id.Dyn[Expr, Kind]
	block      id.ID[Block]
	else_      id.ID[If] //nolint:revive
}

// AsAny type-erases this type value.
//
// See [Expr] for more information.
func (e If) AsAny() Expr {
	if e.IsZero() {
		return Expr{}
	}
	return id.WrapDyn(e.Context(), id.NewDyn(KindIf, id.ID[Expr](e.ID())))
}

// IfToken returns this expression's if token, if it has one.
func (e If) IfToken() token.Token {
	if e.IsZero() {
		return token.Zero
	}
	return id.Wrap(e.Context().Stream(), e.Raw().ifT)
}

// ElseToken returns this expression's else token, if it has one.
func (e If) ElseToken() token.Token {
	if e.IsZero() {
		return token.Zero
	}
	return id.Wrap(e.Context().Stream(), e.Raw().elseT)
}

// Condition returns the condition expression for this if.
//
// This will be zero if it is a final else block.
func (e If) Condition() Expr {
	if e.IsZero() {
		return Expr{}
	}
	return id.WrapDyn(e.Context(), e.Raw().cond)
}

// Block returns this if's block.
func (e If) Block() Block {
	if e.IsZero() {
		return Block{}
	}
	return id.Wrap(e.Context(), e.Raw().block)
}

// Else returns this if's else block, if it has one.
func (e If) Else() If {
	if e.IsZero() {
		return If{}
	}
	return id.Wrap(e.Context(), e.Raw().else_)
}

// SetElse sets this if's else block.
func (e If) SetElse(then If) {
	if !e.IsZero() {
		e.Context().Nodes().panicIfNotOurs(then)
		e.Raw().else_ = e.ID()
	}
}

// Span implements [source.Spanner].
func (e If) Span() source.Span {
	return source.Join(e.ElseToken(), e.IfToken(), e.Condition(), e.Block(), e.Else())
}
