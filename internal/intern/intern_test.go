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

func TestHammer(t *testing.T) {
	t.Parallel()

	start := new(sync.WaitGroup)
	end := new(sync.WaitGroup)

	n := new(atomic.Int64)
	it := new(intern.Table)

	// We collect the results of every query to the table, and then ensure
	// each gets a unique answer.
	mu := new(sync.Mutex)
	query := make(map[string][]intern.ID)
	value := make(map[intern.ID][]string)

	for range runtime.GOMAXPROCS(0) {
		start.Add(1)
		end.Add(1)
		go func() {
			defer end.Done()

			data := makeData(int(n.Add(1)))
			m1 := make(map[string][]intern.ID)
			m2 := make(map[intern.ID][]string)

			// This ensures that we have a thundering herd situation: all of
			// these goroutines wake up and hammer the intern table at the
			// same time.
			start.Done()
			start.Wait()

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
		}()
	}

	end.Wait()

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
	b.Run("Collisions", func(b *testing.B) {
		n := new(atomic.Int64)
		it := new(intern.Table)
		b.RunParallel(func(p *testing.PB) {
			data := makeData(int(n.Add(1)))
			for p.Next() {
				for _, s := range data {
					_ = it.Value(it.InternBytes(s))
				}
			}
		})
	})

	b.Run("Unique", func(b *testing.B) {
		n := new(atomic.Int64)
		it := new(intern.Table)
		b.RunParallel(func(p *testing.PB) {
			data := makeData(int(n.Add(1)))
			for p.Next() {
				for i, s := range data {
					s = append(s, '0')
					data[i] = s
					_ = it.Value(it.InternBytes(s))
				}
			}
		})
	})
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

		buf := make([]byte, n)
		for i := range buf {
			buf[i] = byte('a' + r.Intn(26))
		}
		data[i] = buf
	}
	return data
}
