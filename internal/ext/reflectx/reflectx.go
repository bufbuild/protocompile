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

package reflectx

import "reflect"

// UnwrapStruct removes as many layers of "one field wrappers" on v as possible.
//
// This means:
// 1. The one field in a struct with non-zero size.
// 2. The one element of a [1]T.
func UnwrapStruct(v reflect.Value) reflect.Value {
loop:
	switch v.Kind() {
	case reflect.Struct:
		ty := v.Type()

		var nonzero reflect.StructField
		for i := range ty.NumField() {
			f := ty.Field(i)
			if f.Offset > 0 {
				// This catches the following problematic struct:
				//
				// struct { A int; B [0]int }
				//
				// Zero-sized fields after the last non-zero-sized field
				// result in padding.
				break loop
			}
			if f.Type.Size() > 0 {
				if nonzero.Type != nil {
					break loop
				}
				nonzero = f
			}
		}

		v = v.FieldByIndex(nonzero.Index)
		goto loop

	case reflect.Array:
		if v.Len() == 1 {
			v = v.Index(0)
			goto loop
		}
	}

	return v
}
