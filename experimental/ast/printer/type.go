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

package printer

import "github.com/bufbuild/protocompile/experimental/ast"

// printType prints a type with the specified leading gap.
func (p *printer) printType(ty ast.TypeAny, gap gapStyle, ctx printCtx) {
	if ty.IsZero() {
		return
	}

	switch ty.Kind() {
	case ast.TypeKindPath:
		p.printPath(ty.AsPath().Path, gap, ctx)
	case ast.TypeKindPrefixed:
		p.printTypePrefixed(ty.AsPrefixed(), gap, ctx)
	case ast.TypeKindGeneric:
		p.printTypeGeneric(ty.AsGeneric(), gap, ctx)
	}
}

func (p *printer) printTypePrefixed(ty ast.TypePrefixed, gap gapStyle, ctx printCtx) {
	if ty.IsZero() {
		return
	}
	p.printToken(ty.PrefixToken(), gap, ctx)
	p.printType(ty.Type(), gapSpace, ctx)
}

func (p *printer) printTypeGeneric(ty ast.TypeGeneric, gap gapStyle, ctx printCtx) {
	if ty.IsZero() {
		return
	}

	p.printPath(ty.Path(), gap, ctx)
	args := ty.Args()
	brackets := args.Brackets()
	if brackets.IsZero() {
		return
	}

	openTok, closeTok := brackets.StartEnd()
	trivia := p.trivia.scopeTrivia(brackets.ID())

	p.printToken(openTok, gapPreserve, ctx)
	for i := range args.Len() {
		p.emitTriviaSlot(trivia, i)
		argGap := gapPreserve
		if i > 0 {
			p.printToken(args.Comma(i-1), p.semiGap(), ctx)
			argGap = gapSpace
		}
		p.printType(args.At(i), argGap, ctx)
	}
	p.emitRemainingTrivia(trivia, args.Len())
	p.printToken(closeTok, gapPreserve, ctx)
}
