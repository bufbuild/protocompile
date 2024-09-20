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

// package width exports functions which measure the number of terminal window
// cells that a particular Unicode string can be expected to use up. The
// definition of "width", as well as the implementation, is taken from the
// Rust unicode-width library.
//
// See https://github.com/unicode-rs/unicode-width for details on how this is
// defined.
//
// This functionality should not be confused with the go.golang.org/x/text/width
// package, which is about conversion between full- and half-width variants of
// runes as present in East Asian computing.
package width

// Width makes a best-effort guess at the width of s when displayed on a terminal.
// Tabstops (\t) are treated as specially: they are assumed to justify text to the
// next column that is a multiple of
//
// This function treats characters in the Ambiguous category according
// to Unicode Standard Annex #11 (see http://www.unicode.org/reports/tr11/)
// as 1 column wide. This is consistent with the recommendations for
// non-CJK contexts, or when the context cannot be reliably determined.
func Width(s string, tabstop int) (width int) {
	var state widthInfo
	for _, r := range s {
		n, next := widthInStr(r, state)
		if r == '\t' {
			width += tabstop - width%tabstop
		} else {
			width += int(int8(n))
		}

		state = next
	}
	return
}

// Ruler tracks the state of an ongoing measurement.
//
// Unsurprisingly, measuring a Unicode string is stateful. Being able to stop
// in the middle of a measurement operation, adjust the current width, and
// continue is enabled by this type.
//
// A zero Ruler is ready to use.
type Ruler struct {
	state widthInfo
	width int
}

// Measure pushes a rune onto the running tally and returns its width.
func (r *Ruler) Measure(ch rune) int {
	n, next := widthInStr(ch, r.state)
	r.width += int(int8(n))
	r.state = next
	return r.width
}

// Width returns the width this ruler has measured so far.
func (r *Ruler) Width() int {
	return r.width
}
