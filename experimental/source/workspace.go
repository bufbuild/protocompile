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

package source

// Workspace is a set of Protobuf source paths.
//
// Workspace implementations are assumed by Protocompile to be comparable. It is
// sufficient to always ensure that the implementation uses a pointer receiver.
type Workspace interface {
	// Paths returns an iterator for the paths of the Workspace.
	Paths() []string
}

// NewWorkspace returns a [Workspace] implementation for the given paths that is comparable.
//
// No validations/transformations are performed on the given paths, it is the responsibility
// of the caller to enforce path order and validity.
func NewWorkspace(paths []string) Workspace {
	return &workspace{paths: paths}
}

type workspace struct {
	paths []string
}

// Path implements [Workspace].
func (w *workspace) Paths() []string {
	if w == nil {
		return []string{}
	}
	return w.paths
}
