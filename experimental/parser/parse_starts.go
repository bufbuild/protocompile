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

package parser

import (
	"github.com/bufbuild/protocompile/experimental/internal/taxa"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/experimental/token/keyword"
	"github.com/bufbuild/protocompile/internal/ext/slicesx"
)

var (
	startsPath = taxa.NewSet(taxa.Ident, taxa.Parens, taxa.Dot)
	startsDecl = startsPath.With(taxa.Braces, taxa.Semi)
)

func canStartDecl(tok token.Token) bool {
	return canStartPath(tok) ||
		slicesx.Among(tok.Keyword(), keyword.Semi, keyword.Braces)
}

// canStartPath returns whether or not tok can start a path.
func canStartPath(tok token.Token) bool {
	return tok.Kind() == token.Ident ||
		slicesx.Among(tok.Keyword(), keyword.Dot, keyword.Slash, keyword.Parens)
}

// canStartExpr returns whether or not tok can start an expression.
func canStartExpr(tok token.Token) bool {
	return canStartPath(tok) ||
		tok.Kind() == token.Number || tok.Kind() == token.String ||
		slicesx.Among(tok.Keyword(), keyword.Minus, keyword.Braces, keyword.Brackets, keyword.Less)
}

func canStartOptions(tok token.Token) bool {
	return tok.Keyword() == keyword.Brackets
}

func canStartBody(tok token.Token) bool {
	return tok.Keyword() == keyword.Braces
}
