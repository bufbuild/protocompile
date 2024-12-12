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

// package unsafex contains extensions to Go's package unsafe.
package unsafex

import "unsafe"

// Size is like [unsafe.Sizeof], but it is a generic function and it returns
// an int instead of a uintptr (Go does not have types so large they would
// overflow an int).
func Size[T any]() int {
	var zero T
	return int(unsafe.Sizeof(zero))
}

// Index is like [unsafe.Add], but it operates on a typed pointer and scales the
// offset by that type's size, similar to pointer arithmetic in Rust or C.
//
// This function has the same safety caveats as [unsafe.Add].
func Index[T any](p *T, idx int) *T {
	raw := unsafe.Pointer(p)
	raw = unsafe.Add(raw, Size[T]())
	return (*T)(raw)
}
