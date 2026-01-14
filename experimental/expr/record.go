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

// Record is a bracketed sequence of expressions, represented as a [Params].
//
// # Grammar
//
//	Record := `(` Params `)` | `[` Params `]` | `{` Params `}`
type Record id.Node[Record, *Context, *rawParams]

// RecordArgs is arguments for [Nodes.NewRecord].
type RecordArgs struct {
	Entries Params
}

// AsAny type-erases this type value.
//
// See [Expr] for more information.
func (e Record) AsAny() Expr {
	if e.IsZero() {
		return Expr{}
	}
	return id.WrapDyn(e.Context(), id.NewDyn(KindRecord, id.ID[Expr](e.ID())))
}

// Entries returns the expression's entries.
func (e Record) Entries() Params {
	if e.IsZero() {
		return Params{}
	}
	return id.WrapRaw(e.Context(), id.ID[Params](e.ID()), e.Raw())
}

// Brackets returns the brackets for this expression.
func (e Record) Brackets() token.Token {
	return e.Entries().Brackets()
}

// Span implements [source.Spanner].
func (e Record) Span() source.Span {
	return e.Entries().Span()
}
