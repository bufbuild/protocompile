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

package ir_test

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bufbuild/protocompile/experimental/id"
	"github.com/bufbuild/protocompile/experimental/ir"
)

func TestZero(t *testing.T) {
	t.Parallel()

	testZeroAny[*ir.File](t)
	testZeroAny[ir.Import](t) // Import embeds *ir.File
	testZeroAny[*ir.Imports](t)

	testZeroNode[ir.FeatureSet](t)
	testZero[ir.Feature](t)
	testZero[ir.FeatureInfo](t)

	testZeroNode[ir.Member](t)
	testZeroNode[ir.Extend](t)
	testZeroNode[ir.Oneof](t)
	testZeroNode[ir.ReservedRange](t)
	testZero[ir.ReservedName](t)

	testZero[ir.Service](t)
	testZero[ir.Method](t)

	testZeroNode[ir.Symbol](t)

	testZeroNode[ir.Type](t)

	testZeroNode[ir.Value](t)
	testZeroNode[ir.MessageValue](t)
	testZero[ir.Element](t)
}

// zeroable is a helper interface to enforce that types implement the IsZero method.
type zeroable interface {
	IsZero() bool
}

// node is a helper interface to enforce [id.Node] types.
type node[T any] interface {
	zeroable
	ID() id.ID[T]
}

// testZeroNode is a helper that validates the zero value of IR nodes and enforces the
// [nodes] interface.
func testZeroNode[T node[T]](t *testing.T) {
	t.Helper()
	testZero[T](t)
}

// testZero is a helper that validates the zero value of IR structures and enforces the
// [zeroable] interface.
func testZero[T zeroable](t *testing.T) {
	t.Helper()

	testZeroAny[T](t)
	testZeroAny[ir.Ref[T]](t)
}

// testZeroAny is a helper that validates the zero value of T:
//
//  1. Accessors do not panic.
//  2. The method, IsZero() bool, returns true when called with the zero value.
//  3. The method, Context() [id.Constraint], if present, returns the zero value of *ir.File,
//     which is always comparable.
//  4. Other accessors return zero values.
func testZeroAny[T any](t *testing.T) {
	t.Helper()

	var z T
	assert.Zero(t, z)

	v := reflect.ValueOf(z)
	ty := reflect.TypeOf(z)

	t.Run(fmt.Sprintf("%T", z), func(t *testing.T) {
		for i := range ty.NumMethod() {
			m := ty.Method(i)
			// This roughly represent the "accessors" (NumIn includes the receiver).
			if m.Func.Type().NumIn() != 1 || m.Func.Type().NumOut() == 0 {
				continue
			}
			returns := m.Func.Call([]reflect.Value{v})
			switch m.Name {
			case "IsZero":
				assert.Len(t, returns, 1)
				assert.True(t, returns[0].Bool())
			case "ValueNodeIndex":
				// This is a special case for [ir.Element], since 0 is a valid index, so for the
				// zero value, it returns -1.
				assert.Len(t, returns, 1)
				assert.Equal(t, int64(-1), returns[0].Int())
			case "Context":
				assert.Len(t, returns, 1)
				assert.True(t, returns[0].Type().Comparable())
				assert.True(t, returns[0].Type().AssignableTo(reflect.TypeOf(&ir.File{})))
			default:
				for i, r := range returns {
					if r.Type().Kind() == reflect.Func {
						continue
					}
					// r is an indexable type, so we test that length is 0.
					if m := r.MethodByName("Len"); m.IsValid() {
						assert.Equal(t, 0, m.Type().NumIn())
						assert.Equal(t, 1, m.Type().NumOut())
						r = m.Call(nil)[0]
					}
					assert.Zero(t, r.Interface(), "non-zero return #%d %#v of %T.%s", i, r, z, m.Name)
				}
			}
		}
	})
}
