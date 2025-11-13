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

// TagImport brings a library into scope for a template to use.
//
// # Grammar
//
//	TagImport := `[:` `import` (Ident `:=`)? token.String `:]`
type TagImport id.Node[TagImport, *File, *rawTagImport]

// TagImportArgs sis arguments for [Nodes.NewTagImport].
type TagImportArgs struct {
	Brackets   token.Token
	Keyword    token.Token
	ImportPath ExprAny
}

type rawTagImport struct {
	brackets token.ID
	keyword  token.ID

	importPath id.Dyn[ExprAny, ExprKind]
}

// AsAny type-erases this tag value.
//
// See [TagAny] for more information.
func (t TagImport) AsAny() TagAny {
	if t.IsZero() {
		return TagAny{}
	}
	return id.WrapDyn(t.Context(), id.NewDyn(TagKindImport, id.ID[TagAny](t.ID())))
}

// Brackets returns the bracket token that makes up this tag.
func (t TagImport) Brackets() token.Token {
	if t.IsZero() {
		return token.Zero
	}
	return id.Wrap(t.Context().Stream(), t.Raw().brackets)
}

// Keyword returns the "import" keyword token.
func (t TagImport) Keyword() token.Token {
	if t.IsZero() {
		return token.Zero
	}
	return id.Wrap(t.Context().Stream(), t.Raw().keyword)
}

// ImportPath returns the path for this import.
//
// May be zero, if the user forgot it.
func (d TagImport) ImportPath() ExprAny {
	if d.IsZero() {
		return ExprAny{}
	}

	return id.WrapDyn(d.Context(), d.Raw().importPath)
}

// Span implements [source.Spanner].
func (t TagImport) Span() source.Span {
	return t.Brackets().Span()
}
