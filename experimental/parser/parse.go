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

package parser

import (
	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/internal/taxa"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/token"
)

// Parse lexes and parses the Protobuf file tracked by ctx.
//
// Diagnostics generated by this process are written to errs. Returns whether
// parsing succeeded without errors.
//
// Parse will freeze the stream in ctx when it is done.
func Parse(ctx ast.Context, errs *report.Report) (file ast.File, ok bool) {
	prior := len(errs.Diagnostics)

	Lex(ctx, errs)
	parse(ctx, errs)

	ok = true
	for _, d := range errs.Diagnostics[prior:] {
		ok = ok && d.Level != report.Error
	}

	return ctx.Nodes().Root(), ok
}

// parse implements the core parser loop.
func parse(ctx ast.Context, errs *report.Report) {
	p := &parser{
		Context: ctx,
		Nodes:   ctx.Nodes(),
		Report:  errs,
	}

	defer p.CatchICE(false, nil)

	c := ctx.Stream().Cursor()
	root := ctx.Nodes().Root()

	var mark token.CursorMark
	for !c.Done() {
		next := c.Mark()
		if mark == next {
			panic("protocompile/parser: parser failed to make progress; this is a bug in protocompile")
		}
		mark = next

		node := parseDecl(p, c, taxa.TopLevel)
		if !node.Nil() {
			root.Append(node)
		}
	}
}
