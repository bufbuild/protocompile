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

	testNil[ast.DeclAny](t)
	testNil[ast.DeclBody](t)
	testNil[ast.DeclDef](t)
	testNil[ast.DeclEmpty](t)
	testNil[ast.DeclImport](t)
	testNil[ast.DeclPackage](t)
	testNil[ast.DeclRange](t)
	testNil[ast.DefEnum](t)

	testNil[ast.DefEnumValue](t)
	testNil[ast.DefExtend](t)
	testNil[ast.DefField](t)
	testNil[ast.DefGroup](t)
	testNil[ast.DefMessage](t)
	testNil[ast.DefMethod](t)
	testNil[ast.DefOneof](t)
	testNil[ast.DefOption](t)
	testNil[ast.DefService](t)

	testNil[ast.ExprAny](t)
	testNil[ast.ExprArray](t)
	testNil[ast.ExprDict](t)
	testNil[ast.ExprField](t)
	testNil[ast.ExprLiteral](t)
	testNil[ast.ExprPath](t)
	testNil[ast.ExprPrefixed](t)

	testNil[ast.TypeAny](t)
	testNil[ast.TypeGeneric](t)
	testNil[ast.TypeList](t)
	testNil[ast.TypePath](t)
	testNil[ast.TypePrefixed](t)

	testNil[ast.CompactOptions](t)
	testNil[ast.File](t)
	testNil[ast.Signature](t)
	testNil[ast.Path](t)
	testNil[token.Token](t)
}

// testNil validates that the nil value of some Spanner produces the nil span.
func testNil[N report.Spanner](t *testing.T) {
	t.Helper()
	var Nil N

	t.Run(fmt.Sprintf("%T", Nil), func(t *testing.T) {
		assert.Zero(t, Nil.Span())
	})
}
