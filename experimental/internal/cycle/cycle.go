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

// Package cycle contains internal helpers for dealing with dependency cycles.
package cycle

import (
	"fmt"
	"strings"
)

// ErrCycle is an error due to cyclic dependencies.
type Error[T any] struct {
	// The offending cycle. The first and last entries will be equal.
	Cycle []T
}

// Error implements [error].
func (e *Error[T]) Error() string {
	var buf strings.Builder
	buf.WriteString("cycle detected: ")
	for i, q := range e.Cycle {
		if i != 0 {
			buf.WriteString(" -> ")
		}
		fmt.Fprintf(&buf, "%#v", q)
	}
	return buf.String()
}
