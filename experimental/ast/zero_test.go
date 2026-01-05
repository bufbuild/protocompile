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

package ast_test

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/experimental/token"
)

func TestZero(t *testing.T) {
	t.Parallel()

	testZero[ast.DeclAny](t)
	testZero[ast.DeclBody](t)
	testZero[ast.DeclDef](t)
	testZero[ast.DeclEmpty](t)
	testZero[ast.DeclImport](t)
	testZero[ast.DeclPackage](t)
	testZero[ast.DeclRange](t)
	testZero[ast.DefEnum](t)

	testZero[ast.DefEnumValue](t)
	testZero[ast.DefExtend](t)
	testZero[ast.DefField](t)
	testZero[ast.DefGroup](t)
	testZero[ast.DefMessage](t)
	testZero[ast.DefMethod](t)
	testZero[ast.DefOneof](t)
	testZero[ast.DefOption](t)
	testZero[ast.DefService](t)

	testZero[ast.ExprAny](t)
	testZero[ast.ExprError](t)
	testZero[ast.ExprArray](t)
	testZero[ast.ExprDict](t)
	testZero[ast.ExprField](t)
	testZero[ast.ExprLiteral](t)
	testZero[ast.ExprPath](t)
	testZero[ast.ExprPrefixed](t)

	testZero[ast.TypeAny](t)
	testZero[ast.TypeError](t)
	testZero[ast.TypeGeneric](t)
	testZero[ast.TypeList](t)
	testZero[ast.TypePath](t)
	testZero[ast.TypePrefixed](t)

	testZero[ast.CompactOptions](t)
	testZero[ast.Signature](t)
	testZero[ast.Path](t)
	testZero[token.Token](t)
}

// testZero validates that the zero value of some Spanner produces the
// zero span.
func testZero[Node source.Spanner](t *testing.T) {
	t.Helper()
	var z Node

	t.Run(fmt.Sprintf("%T", z), func(t *testing.T) {
		assert.Zero(t, z.Span())

		// Ensure that every nilary method (used as a rough query for "accessors")
		// on the zero value:
		// 1. Does not panic.
		// 2. Returns zero values.
		v := reflect.ValueOf(z)
		ty := v.Type()
		for i := range ty.NumMethod() {
			m := ty.Method(i)
			if m.Func.Type().NumIn() != 1 || m.Func.Type().NumOut() == 0 {
				continue // NumIn includes the receiver.
			}
			switch m.Name {
			case "IsZero", "String", "Next", "Prev":
				continue
			}
			for i, r := range m.Func.Call([]reflect.Value{v}) {
				if r.Type().Kind() == reflect.Func {
					continue
				}

				if m := r.MethodByName("Len"); m.IsValid() &&
					m.Type().NumIn() == 0 && m.Type().NumOut() == 1 {
					r = m.Call(nil)[0]
				}

				assert.Zero(t, r.Interface(), "non-zero return #%d %#v of %T.%s", i, r, z, m.Name)
			}
		}
	})
}
