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

// Package stringsx contains extensions to Go's package strings.
package stringsx

import (
	"iter"
	"strings"
	"unicode"
	"unicode/utf8"
	"unsafe"

	"github.com/bufbuild/protocompile/internal/ext/iterx"
	"github.com/bufbuild/protocompile/internal/ext/slicesx"
	"github.com/bufbuild/protocompile/internal/ext/unsafex"
)

// Rune returns the rune at the given byte index.
//
// Returns 0, false if out of bounds. Returns -1, false if rune decoding fails.
func Rune[I slicesx.SliceIndex](s string, idx I) (rune, bool) {
	if !slicesx.BoundsCheck(idx, len(s)) {
		return 0, false
	}
	r, n := utf8.DecodeRuneInString(s[idx:])
	if r == utf8.RuneError && n < 2 {
		// The success conditions for DecodeRune are kind of subtle; this makes
		// sure we get the logic right every time. It is somewhat annoying that
		// Go did not chose to make this easier to inspect.
		return -1, false
	}
	return r, true
}

// Rune returns the previous rune at the given byte index.
//
// Returns 0, false if out of bounds. Returns -1, false if rune decoding fails.
func PrevRune[I slicesx.SliceIndex](s string, idx I) (rune, bool) {
	if !slicesx.BoundsCheck(idx-1, len(s)) {
		return 0, false
	}

	r, n := utf8.DecodeLastRuneInString(s[:idx])
	if r == utf8.RuneError && n < 2 {
		// The success conditions for DecodeRune are kind of subtle; this makes
		// sure we get the logic right every time. It is somewhat annoying that
		// Go did not chose to make this easier to inspect.
		return -1, false
	}
	return r, true
}

// Byte returns the rune at the given index.
func Byte[I slicesx.SliceIndex](s string, idx I) (byte, bool) {
	return slicesx.Get(unsafex.BytesAlias[[]byte](s), idx)
}

// LastLine returns the substring after the last newline (U+000A) rune.
func LastLine(s string) string {
	return s[strings.IndexByte(s, '\n')+1:]
}

// Every verifies that all runes in the string are the one given.
func Every(s string, r rune) bool {
	buf := string(r)
	if len(s)%len(buf) != 0 {
		return false
	}

	for i := 0; i < len(s); i += len(buf) {
		if s[i:i+len(buf)] != buf {
			return false
		}
	}

	return true
}

// EveryFunc verifies that all runes in the string satisfy the given predicate.
func EveryFunc(s string, p func(rune) bool) bool {
	return iterx.Every(iterx.Map2to1(Runes(s), func(_ int, r rune) rune {
		if r == -1 {
			r = unicode.ReplacementChar
		}
		return r
	}), p)
}

// Runes returns an iterator over the runes in a string, and their byte indices.
//
// Each non-UTF-8 byte in the string is yielded with a rune value of `-1`.
func Runes(s string) iter.Seq2[int, rune] {
	return func(yield func(i int, r rune) bool) {
		orig := len(s)
		for {
			r, n := utf8.DecodeRuneInString(s)
			if n == 0 {
				return
			}
			if r == utf8.RuneError && n < 2 {
				r = -1
			}
			if !yield(orig-len(s), r) {
				return
			}
			s = s[n:]
		}
	}
}

// Bytes returns an iterator over the bytes in a string.
func Bytes(s string) iter.Seq[byte] {
	return func(yield func(byte) bool) {
		for i := range len(s) {
			// Avoid performing a bounds check each loop step.
			b := *unsafex.Add(unsafe.StringData(s), i)
			if !yield(b) {
				return
			}
		}
	}
}

// Split polyfills [strings.SplitSeq].
//
// Remove in go 1.24.
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

// Split polyfills [strings.Lines].
//
// Remove in go 1.24.
func Lines(s string) iter.Seq[string] {
	return Split(s, '\n')
}

// CutLast is like [strings.Cut], but searches for the last occurrence of sep.
// If sep is not present in s, returns "", s, false.
func CutLast(s, sep string) (before, after string, found bool) {
	if i := strings.LastIndex(s, sep); i >= 0 {
		return s[:i], s[i+len(sep):], true
	}
	return "", s, false
}

// PartitionKey returns an iterator of the largest substrings of s such that
// key(r) for each rune in each substring is the same value.
//
// The iterator also yields the index at which each substring begins.
//
// Will never yield an empty string.
func PartitionKey[K comparable](s string, key func(rune) K) iter.Seq2[int, string] {
	return func(yield func(int, string) bool) {
		var start int
		var prev K
		for i, r := range s {
			next := key(r)
			if i == 0 {
				prev = next
				continue
			}

			if prev == next {
				continue
			}

			if !yield(start, s[start:i]) {
				return
			}

			start = i
			prev = next
		}

		if start < len(s) {
			yield(start, s[start:])
		}
	}
}
