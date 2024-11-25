// Copyright 2020-2024 Buf Technologies, Inc.
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

package ast2

import (
	"unsafe"

	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/internal"
	"github.com/bufbuild/protocompile/experimental/token"
)

type fakePath struct {
	with internal.With[ast.Context]
	raw  struct{ Start, End token.ID }
}

// NewPath creates a new parser-generated path.
//
// This function should not be used outside of the parser, so it is implemented
// using unsafe to avoid needing to export it.
func NewPath(ctx ast.Context, start, end token.Token) ast.Path {
	path := fakePath{
		with: internal.NewWith(ctx),
		raw:  struct{ Start, End token.ID }{start.ID(), end.ID()},
	}

	return *(*ast.Path)(unsafe.Pointer(&path))
}
