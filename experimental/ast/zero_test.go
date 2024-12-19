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

package ast_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/token"
)

func TestNilSpans(t *testing.T) {
	t.Parallel()

	testzero[ast.DeclAny](t)
	testzero[ast.DeclBody](t)
	testzero[ast.DeclDef](t)
	testzero[ast.DeclEmpty](t)
	testzero[ast.DeclImport](t)
	testzero[ast.DeclPackage](t)
	testzero[ast.DeclRange](t)
	testzero[ast.DefEnum](t)

	testzero[ast.DefEnumValue](t)
	testzero[ast.DefExtend](t)
	testzero[ast.DefField](t)
	testzero[ast.DefGroup](t)
	testzero[ast.DefMessage](t)
	testzero[ast.DefMethod](t)
	testzero[ast.DefOneof](t)
	testzero[ast.DefOption](t)
	testzero[ast.DefService](t)

	testzero[ast.ExprAny](t)
	testzero[ast.ExprArray](t)
	testzero[ast.ExprDict](t)
	testzero[ast.ExprField](t)
	testzero[ast.ExprLiteral](t)
	testzero[ast.ExprPath](t)
	testzero[ast.ExprPrefixed](t)

	testzero[ast.TypeAny](t)
	testzero[ast.TypeGeneric](t)
	testzero[ast.TypeList](t)
	testzero[ast.TypePath](t)
	testzero[ast.TypePrefixed](t)

	testzero[ast.CompactOptions](t)
	testzero[ast.File](t)
	testzero[ast.Signature](t)
	testzero[ast.Path](t)
	testzero[token.Token](t)
}

// testzero validates that the zero value of some Spanner produces the zero span.
func testzero[Node report.Spanner](t *testing.T) {
	t.Helper()
	var z Node

	t.Run(fmt.Sprintf("%T", z), func(t *testing.T) {
		assert.Zero(t, z.Span())
	})
}
