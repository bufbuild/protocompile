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

package fdp

import (
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/bufbuild/protocompile/internal"
)

// path is an extension of [protoreflect.SourcePath] to provide an API for path tracking.
type path protoreflect.SourcePath

// clone returns a copy of the currently tracked source path.
func (p *path) clone() protoreflect.SourcePath {
	return internal.ClonePath(protoreflect.SourcePath(*p))
}

// with adds the given elements to the tracked path and returns a reset function. The reset
// trims the length of the given elements off the tracked path. It is the caller's
// responsibility to ensure that reset is called on a valid path length.
func (p *path) with(elements ...int32) func() {
	*p = append(*p, elements...)
	return func() {
		if len(*p) > 0 {
			*p = []int32(*p)[:len(*p)-len(elements)]
		}
	}
}
