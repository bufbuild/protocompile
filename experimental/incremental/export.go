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

package incremental

import (
	"fmt"
	"iter"
	"testing"

	"github.com/bufbuild/protocompile/internal/toposort"
)

// Exported symbols for test use only. Placing such symbols in a _test.go
// file avoids them being exported "for real".

// Abort forces an abort on the given task.
func (t *Task) Abort(err error) { t.abort(err) }

func (e *Executor) PrintDeps(t *testing.T) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if len(e.deps) == 0 {
		t.Log("No dependencies recorded")
		return
	}

	t.Log("=== Task Dependency Graph ===")

	// Create a map from task to a human-readable label
	taskLabels := make(map[*task]string)
	taskIndex := 0
	for parent := range e.deps {
		if _, ok := taskLabels[parent]; !ok {
			taskLabels[parent] = fmt.Sprintf("[%d] %v", taskIndex, parent)
			taskIndex++
		}
		for child := range e.deps[parent] {
			if _, ok := taskLabels[child]; !ok {
				taskLabels[child] = fmt.Sprintf("[%d] %v", taskIndex, child)
				taskIndex++
			}
		}
	}

	// Print all edges in the dependency graph
	t.Log("\nDependency edges (parent -> child):")
	for parent, children := range e.deps {
		parentLabel := taskLabels[parent]
		if len(children) == 0 {
			t.Logf("  %s (no deps)", parentLabel)
		}
		for child := range children {
			childLabel := taskLabels[child]
			t.Logf("  %s -> %s", parentLabel, childLabel)
		}
	}

	// Collect all unique tasks
	allTasks := make(map[*task]struct{})
	for parent := range e.deps {
		allTasks[parent] = struct{}{}
		for child := range e.deps[parent] {
			allTasks[child] = struct{}{}
		}
	}

	roots := make([]*task, 0, len(allTasks))
	for task := range allTasks {
		roots = append(roots, task)
	}

	// Try to topologically sort to detect cycles
	t.Log("\nAttempting topological sort to detect cycles...")
	defer func() {
		if r := recover(); r != nil {
			t.Logf("\n*** CYCLE DETECTED: %v ***", r)
		}
	}()

	sorter := toposort.Sorter[*task, *task]{
		Key: func(task *task) *task { return task },
	}

	dag := func(t *task) iter.Seq[*task] {
		return func(yield func(*task) bool) {
			for child := range e.deps[t] {
				if !yield(child) {
					return
				}
			}
		}
	}

	t.Log("\nTopological order (dependencies first):")
	count := 0
	for task := range sorter.Sort(roots, dag) {
		t.Logf("  %d: %s", count, taskLabels[task])
		count++
	}
	t.Log("\nNo cycles detected!")
}
