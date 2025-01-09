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

package internal

import "fmt"

// With is an embeddable helper for providing Context and Nil methods.
type With[Context comparable] struct {
	ctx Context
}

// NewWith wraps some value in a With.
//
// The ctx field in With is not exported specifically so that when embedding
// With, it does not become exported in the embedding type.
func NewWith[Context comparable](c Context) With[Context] {
	return With[Context]{c}
}

// Context returns this type's associated context.
//
// Returns zero if this is this type's zero value.
func (w With[Context]) Context() Context {
	return w.ctx
}

// IsZero checks whether this is this type's zero value.
func (w With[Context]) IsZero() bool {
	var zero Context
	return w.ctx == zero
}

// PanicIfZero panics if a context is zero.
//
// This is helpful for immediately panicking on function entry.
func PanicIfZero[Context comparable](with *With[Context]) {
	if with.IsZero() {
		var zero Context
		panic(fmt.Errorf("value has nil context %T", zero))
	}
}
