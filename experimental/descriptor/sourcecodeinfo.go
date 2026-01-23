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

package descriptor

// TODO: docs
//
// path is used to track [descriptorpb.SourceCodeInfo] source paths.
type path struct {
	path  []int32
	depth int
}

// appendElements appends the given path elements to the path for tracking.
func (p *path) appendElements(elements ...int32) {
	p.path = append(p.path, elements...)
}

// resetPath resets the path up to the tracked depth.
func (p *path) resetPath() {
	if len(p.path) > 0 {
		p.path = p.path[:len(p.path)-(len(p.path)-p.depth)]
	}
}

// descend increments the tracked depth.
func (p *path) descend(n int) {
	p.depth += n
}

// ascend decrements the tracked depth.
func (p *path) ascend(n int) {
	p.depth -= n
	if p.depth < 0 {
		// 0 is the floor
		p.depth = 0
	}
}
