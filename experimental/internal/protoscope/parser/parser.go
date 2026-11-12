// Copyright 2020-2026 Buf Technologies, Inc.
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
	"github.com/bufbuild/protocompile/experimental/internal/lexer"
	"github.com/bufbuild/protocompile/experimental/internal/protoscope/ast"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/experimental/token/keyword"
	"github.com/bufbuild/protocompile/internal/ext/slicesx"
)

var lex = lexer.Lexer{
	OnKeyword: func(k keyword.Keyword) lexer.OnKeyword {
		switch k {
		case keyword.Hash:
			return lexer.LineComment
		case keyword.LBracket, keyword.LBrace:
			return lexer.BracketKeyword
		case keyword.RParen, keyword.RBracket, keyword.RBrace:
			return lexer.BracketKeyword
		default:
			return lexer.SoftKeyword
		}
	},

	IsAffix: func(affix string, kind token.Kind, suffix bool) bool {
		switch kind {
		case token.Number:
			res := suffix && slicesx.Among(affix, "z", "i32", "i64", "f32", "f64")
			return res
		default:
			return false
		}
	},
	AllowBacktickStrings: true,
}

// Parse lexes and parses a protoscope file.
func Parse(path string, source *source.File, r *report.Report) (file *ast.File, ok bool) {
	prior := len(r.Diagnostics)

	file = ast.New(path, lex.Lex(source, r))
	p := &parser{
		file:   file,
		report: r,
	}
	p.parse(file.Decls(), file.Stream().Cursor())

	ok = true
	for _, d := range r.Diagnostics[prior:] {
		if d.Level() >= report.Error {
			ok = false
			break
		}
	}

	return file, ok
}

type parser struct {
	file   *ast.File
	report *report.Report
}

func (p *parser) parse(inserter seq.Inserter[ast.DeclAny], c *token.Cursor) {
	for !c.Done() {
		m := c.Mark()
		node := p.parseDecl(c)
		if !node.IsZero() {
			seq.Append(inserter, node)
		} else {
			_ = c.Next()
		}
		ensureProgress(c, &m)
	}
}

func (p *parser) parseDecl(c *token.Cursor) ast.DeclAny {
	tok := c.Peek()
	if tok.IsZero() {
		return ast.DeclAny{}
	}

	// Group: !{ ... }
	if tok.Keyword() == keyword.Bang {
		clone := c.Clone()
		_ = clone.Next()
		next := clone.Peek()
		if next.Keyword() == keyword.LBrace || next.Keyword() == keyword.Braces {
			return p.parseGroup(c).AsAny()
		}
	}

	// Heuristic: if it's a number followed by a colon, it's a field.
	if tok.Kind() == token.Number {
		clone := c.Clone()
		_ = clone.Next()
		if clone.Peek().Keyword() == keyword.Colon {
			return p.parseField(c).AsAny()
		}
	}

	// Otherwise, it's a literal or a block.
	if !tok.IsLeaf() {
		return p.parseBlock(c).AsAny()
	}
	return p.parseLiteral(c).AsAny()
}

func (p *parser) parseField(c *token.Cursor) ast.Field {
	tag := c.Next()
	_ = c.Next() // consume colon

	var wireType token.Token
	// Optional wire type
	if c.Peek().Kind() == token.Ident {
		switch c.Peek().Text() {
		case "VARINT", "I64", "LEN", "SGROUP", "EGROUP", "I32":
			wireType = c.Next()
		}
	}

	value := p.parseDecl(c)

	return p.file.Nodes().NewField(ast.FieldArgs{
		Tag:      tag,
		WireType: wireType,
		Value:    value,
	})
}

func (p *parser) parseLiteral(c *token.Cursor) ast.Literal {
	return p.file.Nodes().NewLiteral(c.Next())
}

func (p *parser) parseBlock(c *token.Cursor) ast.Block {
	tok := c.Next()
	b := p.file.Nodes().NewBlock(tok)

	// Recurse into children
	if children := tok.Children(); children != nil {
		p.parse(b.Decls(), children)
	}

	return b
}

func (p *parser) parseGroup(c *token.Cursor) ast.Block {
	bang := c.Next()
	b := p.file.Nodes().NewBlock(bang) // Use bang as the anchor token for the group

	next := c.Next()
	if children := next.Children(); children != nil {
		p.parse(b.Decls(), children)
		return b
	}

	for !c.Done() && c.Peek().Keyword() != keyword.RBrace {
		m := c.Mark()
		node := p.parseDecl(c)
		if !node.IsZero() {
			seq.Append(b.Decls(), node)
		} else {
			_ = c.Next()
		}
		ensureProgress(c, &m)
	}

	if !c.Done() && c.Peek().Keyword() == keyword.RBrace {
		_ = c.Next() // consume }
	}

	return b
}

// ensureProgress is a helper to ensure that the parser is making progress.
// This is used to catch bugs in the parser that would cause it to loop
// forever.
func ensureProgress(c *token.Cursor, m *token.CursorMark) {
	next := c.Mark()
	if *m == next {
		panic("protocompile/parser: parser failed to make progress; this is a bug in protocompile")
	}
	*m = next
}
