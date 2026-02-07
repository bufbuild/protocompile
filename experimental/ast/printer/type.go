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
func (p *printer) printType(ty ast.TypeAny, gap gapStyle) {
	if ty.IsZero() {
		return
	}

	switch ty.Kind() {
	case ast.TypeKindPath:
		p.printPath(ty.AsPath().Path, gap)
	case ast.TypeKindPrefixed:
		p.printTypePrefixed(ty.AsPrefixed(), gap)
	case ast.TypeKindGeneric:
		p.printTypeGeneric(ty.AsGeneric(), gap)
	}
}

func (p *printer) printTypePrefixed(ty ast.TypePrefixed, gap gapStyle) {
	if ty.IsZero() {
		return
	}
	p.printToken(ty.PrefixToken(), gap)
	p.printType(ty.Type(), gapSpace)
}

func (p *printer) printTypeGeneric(ty ast.TypeGeneric, gap gapStyle) {
	if ty.IsZero() {
		return
	}

	p.printPath(ty.Path(), gap)
	args := ty.Args()
	brackets := args.Brackets()
	if brackets.IsZero() {
		return
	}

	openTok, closeTok := brackets.StartEnd()
	slots := p.trivia.scopeSlots(brackets.ID())

	p.printToken(openTok, gapNone)
	for i := range args.Len() {
		p.emitSlot(slots, i)
		argGap := gapNone
		if i > 0 {
			p.printToken(args.Comma(i-1), gapNone)
			argGap = gapSpace
		}
		p.printType(args.At(i), argGap)
	}
	p.emitSlot(slots, args.Len())
	p.printToken(closeTok, gapNone)
}
