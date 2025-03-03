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

package slicesx

import (
	"iter"
	"slices"
)

// Cut is like [strings.Cut], but for slices.
func Cut[S ~[]E, E comparable](s S, needle E) (before, after S, found bool) {
	idx := slices.Index(s, needle)
	if idx == -1 {
		return s, s[len(s):], false
	}
	return s[:idx], s[idx+1:], true
}

// CutAfter is like [Cut], but includes the needle in before.
func CutAfter[S ~[]E, E comparable](s S, needle E) (before, after S, found bool) {
	idx := slices.Index(s, needle)
	if idx == -1 {
		return s, s[len(s):], false
	}
	return s[:idx+1], s[idx+1:], true
}

// CutFunc is like [Cut], but uses a function to select the cut-point.
func CutFunc[S ~[]E, E any](s S, p func(int, E) bool) (before, after S, found bool) {
	idx := IndexFunc(s, p)
	if idx == -1 {
		return s, s[len(s):], false
	}
	return s[:idx], s[idx+1:], true
}

// CutAfterFunc is like [CutFunc], but includes the needle in before.
func CutAfterFunc[S ~[]E, E any](s S, p func(int, E) bool) (before, after S, found bool) {
	idx := IndexFunc(s, p)
	if idx == -1 {
		return s, s[len(s):], false
	}
	return s[:idx+1], s[idx+1:], true
}

// Split is like [strings.Split], but for slices.
func Split[S ~[]E, E comparable](s S, sep E) iter.Seq[S] {
	return func(yield func(S) bool) {
		for {
			before, after, found := Cut(s, sep)
			if !yield(before) {
				return
			}
			if !found {
				break
			}
			s = after
		}
	}
}

// SplitAfter is like [strings.SplitAfter], but for slices.
func SplitAfter[S ~[]E, E comparable](s S, sep E) iter.Seq[S] {
	return func(yield func(S) bool) {
		for {
			before, after, found := CutAfter(s, sep)
			if !yield(before) {
				return
			}
			if !found {
				break
			}
			s = after
		}
	}
}

// SplitFunc is like [Split], but uses a function to select the cut-point.
func SplitFunc[S ~[]E, E any](s S, sep func(int, E) bool) iter.Seq[S] {
	return func(yield func(S) bool) {
		for {
			before, after, found := CutFunc(s, sep)
			if !yield(before) {
				return
			}
			if !found {
				break
			}
			s = after
		}
	}
}

// SplitAfterFunc is like [SplitAfter], but uses a function to select the cut-point.
func SplitAfterFunc[S ~[]E, E any](s S, sep func(int, E) bool) iter.Seq[S] {
	return func(yield func(S) bool) {
		for {
			before, after, found := CutAfterFunc(s, sep)
			if !yield(before) {
				return
			}
			if !found {
				break
			}
			s = after
		}
	}
}
