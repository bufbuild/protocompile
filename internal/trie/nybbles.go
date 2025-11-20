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

package trie

import (
	"fmt"
	"math"
	"math/bits"
	"slices"
	"strings"
	"unsafe"
)

var allOnes = slices.Repeat([]uint64{math.MaxUint64}, 16)

// nybbles is a nybble radix trie, with different choices of index size
// for potential memory compaction.
type nybbles[N uint8 | uint16 | uint32 | uint64] struct {
	// A prefix trie for byte strings.
	//
	// To walk to the node corresponding to a given string, we follow the
	// indices as follows:
	//
	// n := 0
	// for b := range bytes(s) {
	//   n = lo[hi[n][b.hi]][b.lo]
	// }
	//
	// The resulting value of n is the index into values which
	// contains the corresponding value. If any index in hi/lo turns up
	// MaxUint, iteration stops, indicating the tree ends there.
	hi, lo   [][16]N
	hasValue []uint
}

// search walks the trie along the path given by the
// given key, yielding prefixes and indices for each node visited.
func (t *nybbles[N]) search(key string, yield func(string, int) bool) {
	if t.has(0) && !yield("", 0) {
		return
	}

	var n int
	for i := range len(key) {
		b := key[i]
		lo, hi := b&0xf, b>>4

		if len(t.hi) <= n {
			break
		}
		m := int(t.hi[n][hi])

		if len(t.lo) <= m {
			break
		}
		n = int(t.lo[m][lo])

		if t.has(n) && !yield(key[:i+1], n) {
			return
		}
	}
}

// insert adds a new key to the trie; returns the index to insert the
// corresponding value at.
//
// Returns -1 if the trie becomes full and needs to have its index grown.
func (t *nybbles[N]) insert(key string) int {
	if t.hi == nil {
		t.appendAllOnes(&t.hi)
	}

	n := 0
	for i := range len(key) {
		b := key[i]
		lo, hi := b&0xf, b>>4

		m1 := &t.hi[n][hi]
		if len(t.lo) <= int(*m1) {
			*m1 = N(len(t.lo))
			t.appendAllOnes(&t.lo)
		}

		m2 := &t.lo[uint32(*m1)][lo]
		if len(t.hi) <= int(*m2) {
			if len(t.hi) == int(^N(0)) {
				return -1
			}

			*m2 = N(len(t.hi))
			t.appendAllOnes(&t.hi)
		}
		n = int(*m2)
	}

	t.set(n)
	return n
}

func (t *nybbles[N]) appendAllOnes(s *[][16]N) {
	ptr := (*[16]N)(unsafe.Pointer(unsafe.SliceData(allOnes)))
	*s = append(*s, *ptr)
}

func (t *nybbles[N]) has(n int) bool {
	i := n / bits.UintSize
	j := n % bits.UintSize

	return i < len(t.hasValue) && t.hasValue[i]&(uint(1)<<j) != 0
}

func (t *nybbles[N]) set(n int) {
	i := n / bits.UintSize
	j := n % bits.UintSize

	if len(t.hasValue) <= i {
		t.hasValue = append(t.hasValue, make([]uint, i+1-len(t.hasValue))...)
	}
	t.hasValue[i] |= uint(1) << j
}

func (t *nybbles[N]) dump(buf *strings.Builder) {
	var z N
	fmt.Fprintf(buf, "type: *nybbles[%T]\n", z)

	for i, v := range t.hi {
		fmt.Fprintf(buf, "hi[%#x]:", i)
		for _, i := range v {
			if ^i == 0 {
				buf.WriteString(" --")
			} else {
				fmt.Fprintf(buf, " %02x", i)
			}
		}
		fmt.Fprintln(buf)
	}
	for i, v := range t.lo {
		fmt.Fprintf(buf, "lo[%#x]:", i)
		for _, i := range v {
			if ^i == 0 {
				buf.WriteString(" --")
			} else {
				fmt.Fprintf(buf, " %02x", i)
			}
		}
		fmt.Fprintln(buf)
	}
}

func grow[To, From uint8 | uint16 | uint32 | uint64](in *nybbles[From]) *nybbles[To] {
	conv := func(in [][16]From) [][16]To {
		out := make([][16]To, len(in))
		for i, x := range in {
			var y [16]To
			for i := range 16 {
				y[i] = To(x[i])
				if ^x[i] == 0 {
					y[i] = ^To(0)
				}
			}
			out[i] = y
		}
		return out
	}

	return &nybbles[To]{
		hi:       conv(in.hi),
		lo:       conv(in.lo),
		hasValue: in.hasValue,
	}
}
