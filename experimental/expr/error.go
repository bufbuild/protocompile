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
)

// Error is an expression produced due to a parse failure, where a non-zero
// expression was still needed.
//
// Note that Error is not an [error].
type Error id.Node[Error, *Context, *rawError]

type rawError struct {
	span source.Span
}

// AsAny type-erases this type value.
//
// See [Expr] for more information.
func (e Error) AsAny() Expr {
	if e.IsZero() {
		return Expr{}
	}
	return id.WrapDyn(e.Context(), id.NewDyn(KindError, id.ID[Expr](e.ID())))
}

// Span implements [source.Spanner].
func (e Error) Span() source.Span {
	if e.IsZero() {
		return source.Span{}
	}
	return e.Raw().span
}
