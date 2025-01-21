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

package printer

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/token"
)

const (
	defaultLineLimit = 120
	defaultIndent    = "  " // 2 spaces
)

// Print prints the file to the given writer.
//
// TODO: this is a placeholder, we need to implement this.
func Print(out io.Writer, file ast.File) {
	//p := &printer{}
	//p.File(file)
}

var (
	debugCount int
	debugOn    = true
)

func debugf(format string, args ...interface{}) func() {
	if !debugOn {
		return func() {}
	}
	indent := strings.Repeat(" ", debugCount)
	debugCount += 1
	fmt.Printf(indent+"~~~ "+format+" ~~~\n", args...)
	return func() {
		fmt.Println(indent + "~~~ END ~~~")
		debugCount -= 1
	}
}
func debugLogf(format string, args ...interface{}) {
	if !debugOn {
		return
	}
	indent := strings.Repeat(" ", debugCount)
	fmt.Printf(indent+format+"\n", args...)
}

type printer struct {
	bytes.Buffer

	// Record the state of the last token printed. Between the last token
	// and the current token contains the whitespace and comments.
	lastToken token.Token

	tokens []token.Token

	// TODO: for synthetic tokens record the depth??
	depth int
}

func (p *printer) printFile(file ast.File) {
	// TODO: applyFormatting = true; we need to make this configurable
	applyFormatting := true

	for _, block := range fileToBlocks(file, applyFormatting) {
		block.calculateSplits(defaultLineLimit)
		for _, chunk := range block.chunks {
			p.printChunk(chunk, applyFormatting)
		}
	}
}

// TODO: make indentSize configurable
func (p *printer) printChunk(c chunk, applyFormatting bool) {
	for i := uint32(0); i < c.nestingLevel; i++ {
		p.WriteString(defaultIndent)
	}
	p.WriteString(c.text)
	// In the case where formatting is not applied, we do not want to add whitespace,
	// we want to use/preserve the user-defined whitespace instead.
	if applyFormatting {
		switch c.splitKind {
		case splitKindHard:
			p.WriteString("\n")
		case splitKindDouble:
			p.WriteString("\n\n")
		case splitKindSoft, splitKindNever:
			if c.spaceWhenUnsplit {
				p.WriteString(" ")
			}
		}
	}
}

func (p *printer) File(file ast.File) {
	stream := file.Context().Stream()
	stream.Naturals()(func(tok token.Token) bool {
		p.tokens = append(p.tokens, tok)
		return true
	})

	decls := file.Decls()
	for i := 0; i < decls.Len(); i++ {
		// Print the node.
		decl := decls.At(i)
		defer debugf("NODE %d %s", i, decl.Kind())()
		p.printDeclAny(decl)
	}
	// Print trailing comments and whitespace.
	//cursor := stream.CursorAfter(p.lastToken)
	p.printSkippable(token.ID(len(p.tokens)))
}

func (p *printer) printDeclAny(declAny ast.DeclAny) {
	defer debugf("DECLANY %s", declAny.Kind())()
	switch declAny.Kind() {
	case ast.DeclKindInvalid:
		panic("invalid decl")
	case ast.DeclKindEmpty:
		p.printToken(declAny.AsEmpty().Semicolon(), p.indent)
	case ast.DeclKindSyntax:
		p.printDeclSyntax(declAny.AsSyntax())
	case ast.DeclKindPackage:
		p.printDeclPackage(declAny.AsPackage())
	case ast.DeclKindImport:
		p.printDeclImport(declAny.AsImport())
	case ast.DeclKindDef:
		p.printDeclDef(declAny.AsDef())
	case ast.DeclKindBody:
		p.printDeclBody(declAny.AsBody())
	case ast.DeclKindRange:
		p.printDeclRange(declAny.AsRange())
	default:
		panic(fmt.Sprintf("handle: %s\n", declAny.Kind()))
	}
}

func (p *printer) printDeclSyntax(declSyntax ast.DeclSyntax) {
	if declSyntax.IsZero() {
		return
	}
	p.printToken(declSyntax.Keyword(), p.indent)
	p.printToken(declSyntax.Equals(), space)
	p.printExprAny(declSyntax.Value(), space)
	p.printToken(declSyntax.Semicolon(), empty)
}

func (p *printer) printDeclPackage(declPackage ast.DeclPackage) {
	if declPackage.IsZero() {
		return
	}
	p.printToken(declPackage.Keyword(), p.indent)
	p.printPath(declPackage.Path(), space)
	p.printToken(declPackage.Semicolon(), empty)
}

func (p *printer) printDeclImport(declImport ast.DeclImport) {
	if declImport.IsZero() {
		return
	}
	p.printToken(declImport.Keyword(), p.indent)
	p.printToken(declImport.Modifier(), space)
	p.printExprAny(declImport.ImportPath(), space)
	p.printToken(declImport.Semicolon(), empty)
}

func (p *printer) printDeclDef(declDef ast.DeclDef) {
	if declDef.IsZero() {
		return
	}
	switch declDef.Classify() {
	case ast.DefKindMessage:
		msg := declDef.AsMessage()
		p.printToken(msg.Keyword, p.indent)
		p.printToken(msg.Name, space)
		p.printDeclBody(msg.Body)
	//case ast.DefKindEnum:
	//case ast.DefKindService:
	//case ast.DefKindExtend:
	//case ast.DefKindOption:
	case ast.DefKindField:
		field := declDef.AsField()
		p.printTypeAny(field.Type, p.indent)
		p.printToken(field.Name, space)
		p.printToken(field.Equals, space)
		p.printExprAny(field.Tag, space)
		p.printOptions(field.Options, space)
		p.printToken(field.Semicolon, empty)
	//case ast.DefKindEnumValue:
	//case ast.DefKindMethod:
	//case ast.DefKindOneof:
	default:
		panic(fmt.Sprintf("TODO: %v\n", declDef.Classify()))
	}
	// // TODO: other declDefs
	// spacer := p.indent
	//
	//	if typeAny := declDef.Type(); !typeAny.IsZero() {
	//		p.printTypeAny(typeAny, spacer)
	//		spacer = space
	//	} else if ident := declDef.F
	//
	// //p.printToken(declDef.Keyword(), spacer)
	// p.printPath(declDef.Name(), space)
	// if declBody := declDef.
	// p.printDeclBody(declDef.Body())
	// p.printToken(declDef.Semicolon(), empty)
}

func (p *printer) printDeclBody(declBody ast.DeclBody) {
	if declBody.IsZero() {
		return
	}
	braces := declBody.Braces()
	bracesStart, bracesEnd := braces.StartEnd()
	p.printToken(bracesStart, space)
	p.depth += 1
	decls := declBody.Decls()
	for i := 0; i < decls.Len(); i++ {
		// Print the node.
		decl := decls.At(i)
		p.printDeclAny(decl)
	}
	p.depth -= 1
	p.printToken(bracesEnd, p.indent)
}

func (p *printer) printDeclRange(declRange ast.DeclRange) {
	if declRange.IsZero() {
		return
	}
	panic("TODO")
}

func (p *printer) printPath(path ast.Path, spacer spacer) {
	if path.IsZero() {
		return
	}
	p.printToken(path.AsIdent(), spacer)
	//path.Components(func(pathComponent ast.PathComponent) bool {
	//	fmt.Printf("PATH COMPONENT: %s %v\n", pathComponent.AsIdent().Kind(), pathComponent.AsIdent().Text())
	//	fmt.Printf("|-> PATH SEP: %T %v\n", pathComponent.Separator(), pathComponent.Separator().Text())
	//	if pathComponent.IsZero() {
	//		return false
	//	}
	//	if !pathComponent.IsEmpty() {
	//		p.printToken(pathComponent.AsIdent(), spacer)
	//		spacer = empty
	//	}
	//	p.printToken(pathComponent.Separator(), spacer)
	//	spacer = empty
	//	return true
	//})
}

func (p *printer) printTypeAny(typeAny ast.TypeAny, spacer spacer) {
	defer debugf("typeAny", typeAny.Kind())()
	switch typeAny.Kind() {
	case ast.TypeKindInvalid:
		panic("invalid type")
	case ast.TypeKindPath:
		p.printTypePath(typeAny.AsPath(), spacer)
	default:
		panic(fmt.Sprintf("TODO: type %s\n", typeAny.Kind()))
	}

}

func (p *printer) printTypePath(typePath ast.TypePath, spacer spacer) {
	p.printPath(typePath.Path, spacer)
}

func (p *printer) printExprAny(exprAny ast.ExprAny, spacer spacer) {
	defer debugf("exprAny", exprAny.Kind())()
	switch exprAny.Kind() {
	case ast.ExprKindInvalid:
		panic("invalid expr")
	//case ast.ExprKindError
	case ast.ExprKindLiteral:
		p.printExprLiteral(exprAny.AsLiteral(), spacer)
	//case ast.ExprKindPrefixed
	//case ast.ExprKindPath
	//case ast.ExprKindRange
	//case ast.ExprKindArray
	//case ast.ExprKindDict
	//case ast.ExprKindField
	default:
		panic(fmt.Sprintf("TODO: %s\n", exprAny.Kind()))
	}
}

func (p *printer) printExprLiteral(exprLiteral ast.ExprLiteral, spacer spacer) {
	p.printToken(exprLiteral.Token, spacer)
}

func (p *printer) printOptions(options ast.CompactOptions, spacer spacer) {
	if options.IsZero() {
		return
	}
	panic("TODO: printOptions")
}

type spacer func() string

func empty() string               { return "" }
func space() string               { return " " }
func (p *printer) indent() string { return "\n" + strings.Repeat(" ", p.depth) }

//	func (p *printer) printSkippable(cursor *token.Cursor) {
//		for {
//			tok := cursor.PopSkippable()
//			if !tok.Kind().IsSkippable() {
//				// TODO: handle this better.
//				warn := fmt.Sprintf("non-skippable token %s %q", tok, tok.Text())
//				panic(warn)
//			}
//			if tok.IsZero() {
//				break
//			}
//			p.WriteString(tok.Text())
//		}
//	}
func (p *printer) printSkippable(end token.ID) {
	tokIdx := int(end)
	idx := tokIdx
	if idx > len(p.tokens) {
		panic("invalid index")
	}
	for idx > 0 {
		idx -= 1
		tok := p.tokens[idx]
		if !tok.Kind().IsSkippable() {
			break
		}
		if tok.IsZero() {
			break
		}
	}
	for idx < tokIdx {
		tok := p.tokens[idx]
		p.WriteString(tok.Text())
		idx += 1
	}
}

func (p *printer) printToken(tok token.Token, spacer spacer) {
	defer debugf("TOKEN %v => %s", tok, tok.Text())()
	if tok.IsZero() {
		debugLogf("SKIPPING ZERO")
		return
	}
	//stream := tok.Context().Stream()

	if tok.IsSynthetic() {
		p.WriteString(spacer())
	} else {
		// Only print skippable characters. Any non-skippable is a bug.
		debugLogf("last token %s %q, isSync? = %t", p.lastToken, p.lastToken.Text(), p.lastToken.IsSynthetic())
		debugLogf("current token %s %q", tok, tok.Text())
		//cursor := stream.CursorBetween(p.lastToken, tok)
		p.printSkippable(tok.ID() - 1)
	}

	p.WriteString(tok.Text())
	// Set the last seen token.
	if !tok.IsSynthetic() {
		p.lastToken = tok
	}
}
