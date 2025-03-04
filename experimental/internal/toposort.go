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

package internal

import (
	"fmt"
	"iter"
	"slices"

	"github.com/bufbuild/protocompile/internal/ext/slicesx"
)

// TopoSort sorts a graph topologically.
//
// Roots are the nodes whose dependencies we are querying. key returns a
// comparable key for each node. children returns the children of a node.
func TopoSort[Node any, Key comparable](
	roots []Node,
	key func(Node) Key,
	children func(Node) iter.Seq[Node],
) iter.Seq[Node] {
	const (
		unsorted byte = iota
		working
		sorted
	)

	state := make(map[Key]byte)
	stack := slices.Clone(roots)
	visited := make([]bool, len(stack))

	push := func(v Node) {
		k := key(v)
		switch state[k] {
		case unsorted:
			state[k] = working
			stack = append(stack, v)
			//nolint:makezero // False positive.
			visited = append(visited, false)
		case working:
			stack = append(stack, v)
			panic(fmt.Sprintf("protocompile/internal: cycle detected: %v", stack))
		case sorted:
			return
		}
	}

	return func(yield func(Node) bool) {
		// This algorithm is DFS that has been tail-call-optimized into a loop.
		// Each node is visited twice in the loop: once to add its children to
		// the stack, and once to pop it and add it to the output. The visited
		// stack tracks whether this is the first or second visit through the
		// loop.
		for len(stack) > 0 {
			node, _ := slicesx.Last(stack)
			visit := slicesx.LastPointer(visited)

			if !*visit {
				for child := range children(node) {
					push(child)
				}

				*visit = true
				continue
			}

			slicesx.Pop(&stack)
			slicesx.Pop(&visited)
			state[key(node)] = sorted
			if !yield(node) {
				return
			}
		}
	}
}
