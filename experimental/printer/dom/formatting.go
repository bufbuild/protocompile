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

package dom

import (
	"fmt"
)

// Formatting provides the formatting information on the Dom.
type Formatting struct {
	fmt.Stringer

	formatted bool
	lineLimit int
	indentStr string
}

// String returns a strign representation of the formatting information.
func (f *Formatting) String() string {
	state := "unformatted"
	if f.formatted {
		state = "formatted"
	}
	return fmt.Sprintf("state: %s, line limit %d, indent string %q", state, f.lineLimit, f.indentStr)
}

// Formatted returns if the formatting has been applied.
func (f *Formatting) Formatted() bool {
	return f.formatted
}

// LineLimit returns the line limit of the formatting.
func (f *Formatting) LineLimit() int {
	return f.lineLimit
}

// IndentString returns the indent string of the formatting.
func (f *Formatting) IndentString() string {
	return f.indentStr
}
