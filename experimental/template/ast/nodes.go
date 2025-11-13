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

package ast

import (
	"fmt"

	"github.com/bufbuild/protocompile/experimental/id"
	"github.com/bufbuild/protocompile/experimental/token"
)

// Nodes provides storage for the various AST node types, and can be used
// to construct new ones.
type Nodes File

// File returns the [File] that this Nodes adds nodes to.
func (n *Nodes) File() *File {
	return (*File)(n)
}

// NewFragment creates a new Fragment node.
func (n *Nodes) NewFragment(args FragmentArgs) Fragment {
	return id.Wrap(n.File(), id.ID[Fragment](n.fragments.NewCompressed(rawFragment{
		indent: args.IndentDelta,
	})))
}

// NewTagEnd creates a new TagEnd node.
func (n *Nodes) NewTagEnd(args TagEndArgs) TagEnd {
	n.panicIfNotOurs(args.Brackets, args.Keyword)

	return id.Wrap(n.File(), id.ID[TagEnd](n.tags.ends.NewCompressed(rawTagEnd{
		brackets: args.Brackets.ID(),
		keyword:  args.Keyword.ID(),
	})))
}

// NewTagExpr creates a new TagExpr node.
func (n *Nodes) NewTagExpr(args TagExprArgs) TagExpr {
	n.panicIfNotOurs(args.Brackets, args.Expr)

	return id.Wrap(n.File(), id.ID[TagExpr](n.tags.exprs.NewCompressed(rawTagExpr{
		brackets: args.Brackets.ID(),
		expr:     args.Expr.ID(),
	})))
}

// NewTagEmit creates a new TagEmit node.
func (n *Nodes) NewTagEmit(args TagEmitArgs) TagEmit {
	n.panicIfNotOurs(args.Brackets, args.Keyword, args.FilePath, args.Fragment, args.End)

	return id.Wrap(n.File(), id.ID[TagEmit](n.tags.emits.NewCompressed(rawTagEmit{
		brackets: args.Brackets.ID(),
		keyword:  args.Keyword.ID(),
		filePath: args.FilePath.ID(),
		fragment: args.Fragment.ID(),
		end:      args.End.ID(),
	})))
}

// NewTagImport creates a new TagImport node.
func (n *Nodes) NewTagImport(args TagImportArgs) TagImport {
	n.panicIfNotOurs(args.Brackets, args.Keyword, args.ImportPath)

	return id.Wrap(n.File(), id.ID[TagImport](n.tags.imports.NewCompressed(rawTagImport{
		brackets:   args.Brackets.ID(),
		keyword:    args.Keyword.ID(),
		importPath: args.ImportPath.ID(),
	})))
}

// panicIfNotOurs checks that a contextual value is owned by this context, and panics if not.
//
// Does not panic if that is zero or has a zero context. Panics if n is zero.
func (n *Nodes) panicIfNotOurs(that ...any) {
	for _, that := range that {
		if that == nil {
			continue
		}

		var path string
		switch that := that.(type) {
		case interface{ Context() *token.Stream }:
			ctx := that.Context()
			if ctx == nil || ctx == n.File().Stream() {
				continue
			}
			path = ctx.Path()

		case interface{ Context() *File }:
			ctx := that.Context()
			if ctx == nil || ctx == n.File() {
				continue
			}
			path = ctx.Stream().Path()

		default:
			continue
		}

		panic(fmt.Sprintf(
			"template/ast: attempt to mix different contexts: %q vs %q",
			n.stream.Path(),
			path,
		))
	}
}
