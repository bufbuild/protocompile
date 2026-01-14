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

// Expr is any expression type in this package.
//
// Values of this type can be obtained by calling an AsAny method on a expression
// type, such as [Token.AsAny]. It can be type-asserted back to any of
// the concrete expression types using its own As* methods.
//
// This type is used in lieu of a putative Expr interface type to avoid heap
// allocations in functions that would return one of many different expression
// types.
//
// Note that the expression and type grammars for the language are the same, so
// Expr appears in places you might expect to see a "Type" node.
type Expr id.DynNode[Expr, Kind, *Context]

// AsError converts an Expr into an [Error], if that is its concrete type.
//
// Otherwise, returns zero.
func (e Expr) AsError() Error {
	if e.Kind() != KindError {
		return Error{}
	}

	return id.Wrap(e.Context(), id.ID[Error](e.ID().Value()))
}

// AsBlock converts an Expr into a [Block], if that is its concrete type.
//
// Otherwise, returns zero.
func (e Expr) AsBlock() Block {
	if e.Kind() != KindBlock {
		return Block{}
	}

	return id.Wrap(e.Context(), id.ID[Block](e.ID().Value()))
}

// AsFor converts an Expr into a [Call], if that is its concrete type.
//
// Otherwise, returns zero.
func (e Expr) AsCall() Call {
	if e.Kind() != KindCall {
		return Call{}
	}

	return id.Wrap(e.Context(), id.ID[Call](e.ID().Value()))
}

// AsControl converts an Expr into a [Control], if that is its concrete type.
//
// Otherwise, returns zero.
func (e Expr) AsControl() Control {
	if e.Kind() != KindControl {
		return Control{}
	}

	return id.Wrap(e.Context(), id.ID[Control](e.ID().Value()))
}

// AsFor converts an Expr into a [For], if that is its concrete type.
//
// Otherwise, returns zero.
func (e Expr) AsFor() For {
	if e.Kind() != KindFor {
		return For{}
	}

	return id.Wrap(e.Context(), id.ID[For](e.ID().Value()))
}

// AsFunc converts an Expr into a [Func], if that is its concrete type.
//
// Otherwise, returns zero.
func (e Expr) AsFunc() Func {
	if e.Kind() != KindFunc {
		return Func{}
	}

	return id.Wrap(e.Context(), id.ID[Func](e.ID().Value()))
}

// AsIf converts an Expr into a [If], if that is its concrete type.
//
// Otherwise, returns zero.
func (e Expr) AsIf() If {
	if e.Kind() != KindIf {
		return If{}
	}

	return id.Wrap(e.Context(), id.ID[If](e.ID().Value()))
}

// AsOp converts an Expr into a [Op], if that is its concrete type.
//
// Otherwise, returns zero.
func (e Expr) AsOp() Op {
	if e.Kind() != KindOp {
		return Op{}
	}

	return id.Wrap(e.Context(), id.ID[Op](e.ID().Value()))
}

// AsRecord converts an Expr into a [Record], if that is its concrete type.
//
// Otherwise, returns zero.
func (e Expr) AsRecord() Record {
	if e.Kind() != KindRecord {
		return Record{}
	}

	return id.Wrap(e.Context(), id.ID[Record](e.ID().Value()))
}

// AsSwitch converts an Expr into a [Switch], if that is its concrete type.
//
// Otherwise, returns zero.
func (e Expr) AsSwitch() Switch {
	if e.Kind() != KindSwitch {
		return Switch{}
	}

	return id.Wrap(e.Context(), id.ID[Switch](e.ID().Value()))
}

// AsToken converts a Any into a ExprLiteral, if that is the type
// it contains.
//
// Otherwise, returns zero.
func (e Expr) AsToken() Token {
	if e.Kind() != KindToken {
		return Token{}
	}
	return Token{
		ExprContext: e.Context(),
		Token:       id.Wrap(e.Context().Stream(), id.ID[token.Token](e.ID().Value())),
	}
}

// Span implements [source.Spanner].
func (e Expr) Span() source.Span {
	return source.Join(
		e.AsBlock(),
		e.AsCall(),
		e.AsControl(),
		e.AsFor(),
		e.AsFunc(),
		e.AsIf(),
		e.AsOp(),
		e.AsRecord(),
		e.AsSwitch(),
		e.AsToken(),
	)
}

func (Kind) DecodeDynID(lo, _ int32) Kind {
	return Kind(lo)
}

func (k Kind) EncodeDynID(value int32) (int32, int32, bool) {
	return int32(k), value, true
}
