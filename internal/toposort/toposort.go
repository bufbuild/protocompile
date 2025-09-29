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

// Package toposort provides a generic topological sort implementation.
package toposort

import (
	"fmt"
	"iter"

	"github.com/bufbuild/protocompile/internal/ext/slicesx"
)

const (
	unsorted byte = iota
	walking
	sorted
)

// Sort sorts a DAG topologically.
//
// Roots are the nodes whose dependencies we are querying. key returns a
// comparable key for each node. dag contains the data of the DAG being sorted,
// and returns the children of a node.
func Sort[Node any, Key comparable](
	roots []Node,
	key func(Node) Key,
	dag func(Node) iter.Seq[Node],
) iter.Seq[Node] {
	s := Sorter[Node, Key]{Key: key}
	return s.Sort(roots, dag)
}

// Sorter is reusable scratch space for a particular stencil of [Sort], which
// needs to allocate memory for book-keeping. This struct allows amortizing that
// cost.
type Sorter[Node any, Key comparable] struct {
	// A function to extract a unique key from each node, for marking.
	Key func(Node) Key

	state     map[Key]byte
	stack     []Node
	iterating bool
}

// Sort is like [Sort], but re-uses allocated resources stored in s.
func (s *Sorter[Node, Key]) Sort(
	roots []Node,
	dag func(Node) iter.Seq[Node],
) iter.Seq[Node] {
	if s.state == nil {
		s.state = make(map[Key]byte)
	}

	return func(yield func(Node) bool) {
		if s.iterating {
			panic("internal/toposort: Sort() called reëntrantly")
		}
		s.iterating = true
		defer func() {
			clear(s.state)
			clear(s.stack)
			s.stack = s.stack[:0]
			s.iterating = false
		}()

		for _, root := range roots {
			s.push(root)
			// This algorithm is DFS that has been tail-call-optimized into a loop.
			// Each node is visited twice in the loop: once to add its children to
			// the stack, and once to pop it and add it to the output. The state
			// tracks whether this node has been visisted and if its the first
			// or second visit through the loop.
			for len(s.stack) > 0 {
				node, _ := slicesx.Last(s.stack)
				k := s.Key(node)
				state := s.state[k]

				if state == unsorted {
					s.state[k] = walking
					for child := range dag(node) {
						s.push(child)
					}
					continue
				}

				s.stack = s.stack[:len(s.stack)-1]
				if state != sorted {
					if !yield(node) {
						return
					}
					s.state[k] = sorted
				}
			}
		}
	}
}

func (s *Sorter[Node, Key]) push(v Node) {
	k := s.Key(v)
	switch s.state[k] {
	case unsorted:
		s.stack = append(s.stack, v)

	case walking:
		prev := slicesx.LastIndexFunc(s.stack, func(n Node) bool {
			return s.Key(n) == k
		})
		suffix := s.stack[prev:]
		panic(fmt.Sprintf("protocompile/internal: cycle detected: %v -> %v", slicesx.Join(suffix, "->"), v))

	case sorted:
		return
	}
}
