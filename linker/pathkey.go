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

package linker

import (
	"reflect"
	"unsafe"

	"google.golang.org/protobuf/reflect/protoreflect"
)

var pathElementType = reflect.TypeOf(protoreflect.SourcePath{}).Elem()

func pathKey(p protoreflect.SourcePath) any {
	if p == nil {
		// Reflection code below doesn't work with nil slices
		return [0]int32{}
	}
	data := unsafe.Pointer(unsafe.SliceData(p))
	array := reflect.NewAt(reflect.ArrayOf(len(p), pathElementType), data)
	return array.Elem().Interface()
}
