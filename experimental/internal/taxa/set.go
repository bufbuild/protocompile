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

package taxa

import (
	"fmt"
	"strings"

	"github.com/bufbuild/protocompile/internal/iter"
)

// Set is a set of [Subject] values, implicitly ordered by the [Subject] values'
// intrinsic order.
//
// A zero Set is empty and ready to use.
type Set struct {
	bits [(Total + 63) / 64]uint64
}

// NewSet returns a new [Set] with the given values set.
//
// Panics if any value is not one of the constants in this package.
func NewSet(whats ...Subject) Set {
	return Set{}.With(whats...)
}

// Len returns the number of values in the set.
func (s Set) Len() int {
	var n int
	s.All()(func(_ Subject) bool {
		n++
		return true
	})
	return n
}

// Has checks whether w is present in this set.
func (s Set) Has(w Subject) bool {
	if w >= Subject(Total) {
		return false
	}

	has := s.bits[int(w)/64] & (uint64(1) << (int(w) % 64))
	return has != 0
}

// With returns a new Set with the given values inserted.
//
// Panics if any value is not one of the constants in this package.
func (s Set) With(whats ...Subject) Set {
	for _, w := range whats {
		if w >= Subject(Total) {
			panic(fmt.Sprintf("internal/what: inserted invalid value %d", w))
		}

		s.bits[int(w)/64] |= uint64(1) << (int(w) % 64)
	}
	return s
}

// All returns an iterator over the elements in the set.
func (s Set) All() iter.Seq[Subject] {
	return func(yield func(Subject) bool) {
		for i, word := range s.bits {
			next := i * 64
			for word != 0 {
				if word&1 == 1 && !yield(Subject(next)) {
					return
				}

				word >>= 1
				next++
			}
		}
	}
}

// Join returns a comma-delimited string containing the names of the elements of
// this set, using the given conjunction as the final separator, and taking
// care to include an Oxford comma only when necessary.
//
// For example, NewSet(Message, Enum, Service).Join("and") will produce the
// string "message, enum, and service".
//
// If the set is empty, returns the empty string.
func (s Set) Join(conj string) string {
	var elems []Subject
	s.All()(func(s Subject) bool {
		elems = append(elems, s)
		return true
	})

	var out strings.Builder
	switch len(elems) {
	case 0:
	case 1:
		fmt.Fprintf(&out, "%v", elems[0])
	case 2:
		fmt.Fprintf(&out, "%v %s %v", elems[0], conj, elems[1])
	default:
		for _, v := range elems[:len(elems)-1] {
			fmt.Fprintf(&out, "%v, ", v)
		}
		fmt.Fprintf(&out, "%s %v", conj, elems[len(elems)-1])
	}

	return out.String()
}
