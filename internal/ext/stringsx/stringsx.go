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
	"strings"
	"unicode/utf8"
	"unsafe"

	"github.com/bufbuild/protocompile/internal/ext/iterx"
	"github.com/bufbuild/protocompile/internal/ext/slicesx"
	"github.com/bufbuild/protocompile/internal/ext/unsafex"
	"github.com/bufbuild/protocompile/internal/iter"
)

// Rune returns the rune at the given index.
//
// Returns 0, false if out of bounds. Returns U+FFFD, false if rune decoding fails.
func Rune[I slicesx.SliceIndex](s string, idx I) (rune, bool) {
	if !slicesx.BoundsCheck(idx, len(s)) {
		return 0, false
	}
	r, _ := utf8.DecodeRuneInString(s[idx:])
	return r, r != utf8.RuneError
}

// Rune returns the previous rune at the given index.
//
// Returns 0, false if out of bounds. Returns U+FFFD, false if rune decoding fails.
func PrevRune[I slicesx.SliceIndex](s string, idx I) (rune, bool) {
	if !slicesx.BoundsCheck(idx-1, len(s)) {
		return 0, false
	}

	r, _ := utf8.DecodeLastRuneInString(s[:idx])
	return r, r != utf8.RuneError
}

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
		for i := range len(s) {
			// Avoid performing a bounds check each loop step.
			b := *unsafex.Add(unsafe.StringData(s), i)
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
