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

package reflectx_test

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bufbuild/protocompile/internal/ext/reflectx"
)

func TestUnwrap(t *testing.T) {
	t.Parallel()
	tests := []struct {
		have, want reflect.Type
	}{
		{have: reflect.TypeFor[int](), want: reflect.TypeFor[int]()},
		{have: reflect.TypeFor[[1]int](), want: reflect.TypeFor[int]()},
		{have: reflect.TypeFor[[2]int](), want: reflect.TypeFor[[2]int]()},

		{have: reflect.TypeFor[struct {
			V int
		}](), want: reflect.TypeFor[int]()},

		{have: reflect.TypeFor[struct {
			_ [0]uint64
			V byte
		}](), want: reflect.TypeFor[byte]()},

		{have: reflect.TypeFor[struct {
			_ [1]uint64
			V byte
		}](), want: reflect.TypeFor[struct {
			_ [1]uint64
			V byte
		}]()},

		{have: reflect.TypeFor[struct {
			_ [0]uint32
			V [1]struct {
				_ [0]uint64
				V int
			}
		}](), want: reflect.TypeFor[int]()},

		{have: reflect.TypeFor[struct {
			V uint32
			_ [0]uint32
		}](), want: reflect.TypeFor[struct {
			V uint32
			_ [0]uint32
		}]()},
	}

	for _, tt := range tests {
		t.Run(tt.have.Name(), func(t *testing.T) {
			t.Parallel()

			v := reflect.New(tt.have).Elem()
			v = reflectx.UnwrapStruct(v)
			assert.Equal(t, tt.want, v.Type())
		})
	}
}
