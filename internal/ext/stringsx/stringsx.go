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

// package stringsx contains extensions to Go's package strings.
package stringsx

import (
	"strings"

	"github.com/bufbuild/protocompile/internal/ext/iterx"
	"github.com/bufbuild/protocompile/internal/ext/unsafex"
	"github.com/bufbuild/protocompile/internal/iter"
)

// EveryFunc verifies that all runes in the string satisfy the given predicate.
func EveryFunc(s string, p func(rune) bool) bool {
	return iterx.All(Runes(s), p)
}

// Runes returns an iterator over the runes in a string.
//
// Each non-UTF-8 byte in the string is yielded as a replacement character (U+FFFD).
func Runes(s string) iter.Seq[rune] {
	return func(yield func(r rune) bool) {
		for _, r := range s {
			if !yield(r) {
				return
			}
		}
	}
}

// Bytes returns an iterator over the bytes in a string.
func Bytes(s string) iter.Seq[byte] {
	return func(yield func(byte) bool) {
		for i := 0; i < len(s); i++ {
			// Avoid performing a bounds check each loop step.
			b := *unsafex.Add(unsafex.StringData(s), i)
			if !yield(b) {
				return
			}
		}
	}
}

// Split is like [strings.Split], but returning an iterator instead of a slice.
func Split[Sep string | rune](s string, sep Sep) iter.Seq[string] {
	r := string(sep)
	return func(yield func(string) bool) {
		for {
			chunk, rest, found := strings.Cut(s, r)
			s = rest
			if !yield(chunk) || !found {
				return
			}
		}
	}
}

// Lines returns an iterator over the lines in the given string.
//
// It is equivalent to Split(s, '\n').
func Lines(s string) iter.Seq[string] {
	return Split(s, '\n')
}
