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

package printer

import "github.com/bufbuild/protocompile/experimental/dom"

// Named tag helpers for common whitespace patterns. These centralize
// the dom primitives so call sites express intent rather than
// mechanics.

var (
	tagSpace     = dom.Text(" ")
	tagNewline   = dom.Text("\n")
	tagSoftbreak = dom.TextIf(dom.Broken, "\n")
)

// softline pushes a space-if-flat, newline-if-broken pair into the
// given sink.
func softline(push dom.Sink) {
	push(dom.TextIf(dom.Flat, " "), dom.TextIf(dom.Broken, "\n"))
}

// blankline pushes two newlines (one blank line) into the given sink.
func blankline(push dom.Sink) {
	push(dom.Text("\n"), dom.Text("\n"))
}
