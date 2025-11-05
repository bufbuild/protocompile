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

package astx

import (
	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/id"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/internal/ext/unsafex"
)

// NewPath creates a new parser-generated path.
//
// This function should not be used outside of the parser, so it is implemented
// using unsafe to avoid needing to export it.
func NewPath(file *ast.File, start, end token.Token) ast.Path {
	// fakePath has the same GC shape as ast.Path; there is a test for this in
	// path_test.go
	return unsafex.Bitcast[ast.Path](fakePath{
		with: id.WrapContext(file),
		raw:  struct{ Start, End token.ID }{start.ID(), end.ID()},
	})
}

type fakePath struct {
	with id.HasContext[*ast.File]
	raw  struct{ Start, End token.ID }
}
