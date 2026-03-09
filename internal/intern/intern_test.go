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

package intern_test

import (
	"fmt"
	"math/rand"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bufbuild/protocompile/internal/ext/slicesx"
	"github.com/bufbuild/protocompile/internal/ext/synctestx"
	"github.com/bufbuild/protocompile/internal/inlinetest"
	"github.com/bufbuild/protocompile/internal/intern"
)

func TestIntern(t *testing.T) {
	t.Parallel()

	data := []string{
		"",
		"a",
		"abc",
		"?",
		"xy.z",
		"a_b_c",
		".....",
		"foo.",
		"foo.a",
		"very long",
		" ",
		"verylong",
	}

	var table intern.Table
	for i := range 3 {
		for _, s := range data {
			t.Run(fmt.Sprintf("%s/%d", s, i), func(t *testing.T) {
				t.Parallel()

				id := table.Intern(s)
				assert.Equal(t, s, table.Value(id), "id: %v", id)
				assert.Equal(t, shouldInline(s), id < 0)
			})
		}
	}
}

func shouldInline(s string) bool {
	if s == "" || len(s) > 5 || strings.HasSuffix(s, ".") {
		return false
	}

	for _, r := range s {
		switch {
		case r >= '0' && r <= '9',
			r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r == '_', r == '.':

		default:
			return false
		}
	}

	return true
}

func TestInline(t *testing.T) {
	t.Parallel()
	inlinetest.AssertInlined(t, "Table.Value")
}

func TestHammer(t *testing.T) {
	t.Parallel()

	n := new(atomic.Int64)
	it := new(intern.Table)

	// We collect the results of every query to the table, and then ensure
	// each gets a unique answer.
	mu := new(sync.Mutex)
	query := make(map[string][]intern.ID)
	value := make(map[intern.ID][]string)

	synctestx.Hammer(0, func() {
		data := makeData(int(n.Add(1)))
		m1 := make(map[string][]intern.ID)
		m2 := make(map[intern.ID][]string)

		for _, s := range data {
			s := string(s)
			id := it.Intern(s)
			m1[s] = append(m1[s], id)

			v := it.Value(id)
			m2[id] = append(m2[id], v)

			assert.Equal(t, s, v)
		}

		mu.Lock()
		defer mu.Unlock()
		for k, v := range m1 {
			query[k] = append(query[k], v...)
		}
		for k, v := range m2 {
			value[k] = append(value[k], v...)
		}
	})

	for k, v := range query {
		slices.Sort(v)
		v = slicesx.Dedup(v)
		assert.Len(t, v, 1, "query[%v]: %v", k, v)
	}

	for k, v := range value {
		slices.Sort(v)
		v = slicesx.Dedup(v)
		assert.Len(t, v, 1, "value[%v]: %v", k, v)
	}
}

func BenchmarkIntern(b *testing.B) {
	// Helper to ensure that it.Value is actually inlined, which is relevant
	// for benchmarks. Calls within the body of a benchmark are never inlined.
	//
	// Returns the length of the string to ensure that this function is not
	// DCE'd.
	value := func(it *intern.Table, id intern.ID) int {
		return len(it.Value(id))
	}

	run := func(name string, unique float64) {
		b.Run(name, func(b *testing.B) {
			// Pre-allocate data samples for each goroutine.
			data := make([][][]byte, runtime.GOMAXPROCS(0))
			for i := range data {
				data[i] = makeData(i)
			}

			n := new(atomic.Int64)
			it := new(intern.Table)
			b.RunParallel(func(p *testing.PB) {
				n := n.Add(1) - 1
				data := data[n]
				r := rand.New(rand.NewSource(n))

				for p.Next() {
					for i, s := range data {
						if r.Float64() < unique {
							s = append(s, '0')
							data[i] = s
						}

						_ = value(it, it.InternBytes(s))
					}
				}
			})
		})
	}

	run("0pct", 0.0)
	run("10pct", 0.1)
	run("50pct", 0.5)
	run("100pct", 1.0)
}

// makeData generates deterministic pseudo-random data of poor quality, meaning
// that strings are likely to repeat in different orders across different
// seeds.
func makeData(seed int) [][]byte {
	data := make([][]byte, 10000)
	n := seed
	r := rand.New(rand.NewSource(int64(seed)))
	for i := range data {
		n += 5
		n %= 99

		buf := make([]byte, n, 10000)
		for i := range buf {
			buf[i] = byte('a' + r.Intn(26))
		}
		data[i] = buf
	}
	return data
}
