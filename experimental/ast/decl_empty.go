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

// DeclEmpty is an empty declaration, a lone ;.
//
// # Grammar
//
//	DeclEmpty := `;`
type DeclEmpty id.Node[DeclEmpty, *File, *rawDeclEmpty]

type rawDeclEmpty struct {
	semi token.ID
}

// AsAny type-erases this declaration value.
//
// See [DeclAny] for more information.
func (d DeclEmpty) AsAny() DeclAny {
	if d.IsZero() {
		return DeclAny{}
	}
	return id.WrapDyn(d.Context(), id.NewDyn(DeclKindEmpty, id.ID[DeclAny](d.ID())))
}

// Semicolon returns this field's ending semicolon.
//
// May be [token.Zero], if not present.
func (d DeclEmpty) Semicolon() token.Token {
	if d.IsZero() {
		return token.Zero
	}

	return id.Wrap(d.Context().Stream(), d.Raw().semi)
}

// Span implements [source.Spanner].
func (d DeclEmpty) Span() source.Span {
	if d.IsZero() {
		return source.Span{}
	}

	return d.Semicolon().Span()
}
