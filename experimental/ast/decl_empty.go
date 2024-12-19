// Copyright 2020-2024 Buf Technologies, Inc.
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
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/internal/arena"
)

// DeclEmpty is an empty declaration, a lone ;.
type DeclEmpty struct{ declImpl[rawDeclEmpty] }

type rawDeclEmpty struct {
	semi token.ID
}

// Semicolon returns this field's ending semicolon.
//
// May be [token.Zero], if not present.
func (d DeclEmpty) Semicolon() token.Token {
	if d.IsZero() {
		return token.Zero
	}

	return d.raw.semi.In(d.Context())
}

// Span implements [report.Spanner].
func (d DeclEmpty) Span() report.Span {
	if d.IsZero() {
		return report.Span{}
	}

	return d.Semicolon().Span()
}

func wrapDeclEmpty(c Context, ptr arena.Pointer[rawDeclEmpty]) DeclEmpty {
	return DeclEmpty{wrapDecl(c, ptr)}
}
