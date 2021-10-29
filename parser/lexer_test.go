package parser

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jhump/protocompile/ast"
	"github.com/jhump/protocompile/reporter"
)

func TestLexer(t *testing.T) {
	handler := reporter.NewHandler(nil)
	l := newTestLexer(t, strings.NewReader(`
	// comment

	/*
	 * block comment
	 */ /* inline comment */

	int32  "\032\x16\n\rfoobar\"zap"		'another\tstring\'s\t'
foo

	// another comment
	// more and more...

	service rpc message
	.type
	.f.q.n
	name
	f.q.n

	.01
	.01e12
	.01e+5
	.033e-1

	12345
	-12345
	123.1234
	0.123
	012345
	0x2134abcdef30
	-0543
	-0xff76
	101.0102
	202.0203e1
	304.0304e-10
	3.1234e+12

	{ } + - , ;

	[option=foo]
	syntax = "proto2";

	// some strange cases
	1.543 g12 /* trailing line comment */
	000.000
	0.1234 .5678 .
	12e12

	Random_identifier_with_numbers_0123456789_and_letters...
	// this is a trailing comment
	// that spans multiple lines
	// over two in fact!
	/*
	 * this is a detached comment
	 * with lots of extra words and stuff...
	 */

	// this is an attached leading comment
	foo

	1.23e+20+20
	// a trailing comment for last element

	// comment attached to no tokens (upcoming token is EOF!)
	/* another comment followed by some final whitespace*/

	
	`), handler)

	var prev ast.Node
	var sym protoSymType
	expected := []struct {
		t          int
		line, col  int
		span       int
		v          interface{}
		comments   []string
		trailCount int
	}{
		{t: _INT32, line: 8, col: 9, span: 5, v: "int32", comments: []string{"// comment\n", "/*\n\t * block comment\n\t */", "/* inline comment */"}},
		{t: _STRING_LIT, line: 8, col: 16, span: 25, v: "\032\x16\n\rfoobar\"zap"},
		{t: _STRING_LIT, line: 8, col: 57, span: 22, v: "another\tstring's\t"},
		{t: _NAME, line: 9, col: 1, span: 3, v: "foo"},
		{t: _SERVICE, line: 14, col: 9, span: 7, v: "service", comments: []string{"// another comment\n", "// more and more...\n"}},
		{t: _RPC, line: 14, col: 17, span: 3, v: "rpc"},
		{t: _MESSAGE, line: 14, col: 21, span: 7, v: "message"},
		{t: '.', line: 15, col: 9, span: 1},
		{t: _NAME, line: 15, col: 10, span: 4, v: "type"},
		{t: '.', line: 16, col: 9, span: 1},
		{t: _NAME, line: 16, col: 10, span: 1, v: "f"},
		{t: '.', line: 16, col: 11, span: 1},
		{t: _NAME, line: 16, col: 12, span: 1, v: "q"},
		{t: '.', line: 16, col: 13, span: 1},
		{t: _NAME, line: 16, col: 14, span: 1, v: "n"},
		{t: _NAME, line: 17, col: 9, span: 4, v: "name"},
		{t: _NAME, line: 18, col: 9, span: 1, v: "f"},
		{t: '.', line: 18, col: 10, span: 1},
		{t: _NAME, line: 18, col: 11, span: 1, v: "q"},
		{t: '.', line: 18, col: 12, span: 1},
		{t: _NAME, line: 18, col: 13, span: 1, v: "n"},
		{t: _FLOAT_LIT, line: 20, col: 9, span: 3, v: 0.01},
		{t: _FLOAT_LIT, line: 21, col: 9, span: 6, v: 0.01e12},
		{t: _FLOAT_LIT, line: 22, col: 9, span: 6, v: 0.01e5},
		{t: _FLOAT_LIT, line: 23, col: 9, span: 7, v: 0.033e-1},
		{t: _INT_LIT, line: 25, col: 9, span: 5, v: uint64(12345)},
		{t: '-', line: 26, col: 9, span: 1, v: nil},
		{t: _INT_LIT, line: 26, col: 10, span: 5, v: uint64(12345)},
		{t: _FLOAT_LIT, line: 27, col: 9, span: 8, v: 123.1234},
		{t: _FLOAT_LIT, line: 28, col: 9, span: 5, v: 0.123},
		{t: _INT_LIT, line: 29, col: 9, span: 6, v: uint64(012345)},
		{t: _INT_LIT, line: 30, col: 9, span: 14, v: uint64(0x2134abcdef30)},
		{t: '-', line: 31, col: 9, span: 1, v: nil},
		{t: _INT_LIT, line: 31, col: 10, span: 4, v: uint64(0543)},
		{t: '-', line: 32, col: 9, span: 1, v: nil},
		{t: _INT_LIT, line: 32, col: 10, span: 6, v: uint64(0xff76)},
		{t: _FLOAT_LIT, line: 33, col: 9, span: 8, v: 101.0102},
		{t: _FLOAT_LIT, line: 34, col: 9, span: 10, v: 202.0203e1},
		{t: _FLOAT_LIT, line: 35, col: 9, span: 12, v: 304.0304e-10},
		{t: _FLOAT_LIT, line: 36, col: 9, span: 10, v: 3.1234e+12},
		{t: '{', line: 38, col: 9, span: 1, v: nil},
		{t: '}', line: 38, col: 11, span: 1, v: nil},
		{t: '+', line: 38, col: 13, span: 1, v: nil},
		{t: '-', line: 38, col: 15, span: 1, v: nil},
		{t: ',', line: 38, col: 17, span: 1, v: nil},
		{t: ';', line: 38, col: 19, span: 1, v: nil},
		{t: '[', line: 40, col: 9, span: 1, v: nil},
		{t: _OPTION, line: 40, col: 10, span: 6, v: "option"},
		{t: '=', line: 40, col: 16, span: 1, v: nil},
		{t: _NAME, line: 40, col: 17, span: 3, v: "foo"},
		{t: ']', line: 40, col: 20, span: 1, v: nil},
		{t: _SYNTAX, line: 41, col: 9, span: 6, v: "syntax"},
		{t: '=', line: 41, col: 16, span: 1, v: nil},
		{t: _STRING_LIT, line: 41, col: 18, span: 8, v: "proto2"},
		{t: ';', line: 41, col: 26, span: 1, v: nil},
		{t: _FLOAT_LIT, line: 44, col: 9, span: 5, v: 1.543, comments: []string{"// some strange cases\n"}},
		{t: _NAME, line: 44, col: 15, span: 3, v: "g12"},
		{t: _FLOAT_LIT, line: 45, col: 9, span: 7, v: 0.0, comments: []string{"/* trailing line comment */"}, trailCount: 1},
		{t: _FLOAT_LIT, line: 46, col: 9, span: 6, v: 0.1234},
		{t: _FLOAT_LIT, line: 46, col: 16, span: 5, v: 0.5678},
		{t: '.', line: 46, col: 22, span: 1, v: nil},
		{t: _FLOAT_LIT, line: 47, col: 9, span: 5, v: 12e12},
		{t: _NAME, line: 49, col: 9, span: 53, v: "Random_identifier_with_numbers_0123456789_and_letters"},
		{t: '.', line: 49, col: 62, span: 1, v: nil},
		{t: '.', line: 49, col: 63, span: 1, v: nil},
		{t: '.', line: 49, col: 64, span: 1, v: nil},
		{t: _NAME, line: 59, col: 9, span: 3, v: "foo", comments: []string{"// this is a trailing comment\n", "// that spans multiple lines\n", "// over two in fact!\n", "/*\n\t * this is a detached comment\n\t * with lots of extra words and stuff...\n\t */", "// this is an attached leading comment\n"}, trailCount: 3},
		{t: _FLOAT_LIT, line: 61, col: 9, span: 8, v: 1.23e+20},
		{t: '+', line: 61, col: 17, span: 1, v: nil},
		{t: _INT_LIT, line: 61, col: 18, span: 2, v: uint64(20)},
	}

	for i, exp := range expected {
		tok := l.Lex(&sym)
		if tok == 0 {
			t.Fatalf("lexer reported EOF but should have returned %v", exp)
		}
		var n ast.Node
		var val interface{}
		switch tok {
		case _SYNTAX, _OPTION, _INT32, _SERVICE, _RPC, _MESSAGE, _NAME:
			n = sym.id
			val = sym.id.Val
		case _STRING_LIT:
			n = sym.s
			val = sym.s.Val
		case _INT_LIT:
			n = sym.i
			val = sym.i.Val
		case _FLOAT_LIT:
			n = sym.f
			val = sym.f.Val
		case _ERROR:
			val = sym.err
		default:
			n = sym.b
			val = nil
		}
		if !assert.Equal(t, exp.t, tok, "case %d: wrong token type (expecting %+v, got %+v)", i, exp.v, val) {
			break
		}
		if !assert.Equal(t, exp.v, val, "case %d: wrong token value", i) {
			break
		}
		nodeInfo := l.info.NodeInfo(n)
		var prevNodeInfo ast.NodeInfo
		if prev != nil {
			prevNodeInfo = l.info.NodeInfo(prev)
		}
		assert.Equal(t, exp.line, nodeInfo.Start().Line, "case %d: wrong line number", i)
		assert.Equal(t, exp.col, nodeInfo.Start().Col, "case %d: wrong column number (on line %d)", i, exp.line)
		assert.Equal(t, exp.line, nodeInfo.End().Line, "case %d: wrong end line number", i)
		assert.Equal(t, exp.col+exp.span, nodeInfo.End().Col, "case %d: wrong end column number", i)
		actualTrailCount := 0
		if prev != nil {
			actualTrailCount = prevNodeInfo.TrailingComments().Len()
		}
		assert.Equal(t, exp.trailCount, actualTrailCount, "case %d: wrong number of trailing comments", i)
		assert.Equal(t, len(exp.comments)-exp.trailCount, nodeInfo.LeadingComments().Len(), "case %d: wrong number of comments", i)
		for ci := range exp.comments {
			var c ast.Comment
			if ci < exp.trailCount {
				c = prevNodeInfo.TrailingComments().Index(ci)
			} else {
				c = nodeInfo.LeadingComments().Index(ci - exp.trailCount)
			}
			assert.Equal(t, exp.comments[ci], c.RawText(), "case %d, comment #%d: unexpected text", i, ci+1)
		}
		prev = n
	}
	if tok := l.Lex(&sym); tok != 0 {
		t.Fatalf("lexer reported symbol after what should have been EOF: %d", tok)
	}
	require.NoError(t, handler.Error())
	// Now we check final state of lexer for unattached comments and final whitespace
	// One of the final comments get associated as trailing comment for final token
	prevNodeInfo := l.info.NodeInfo(prev)
	assert.Equal(t, 1, prevNodeInfo.TrailingComments().Len(), "last token: wrong number of trailing comments")
	eofNodeInfo := l.info.TokenInfo(l.eof)
	finalComments := eofNodeInfo.LeadingComments()
	if assert.Equal(t, 2, finalComments.Len(), "wrong number of final remaining comments") {
		assert.Equal(t, "// comment attached to no tokens (upcoming token is EOF!)\n", finalComments.Index(0).RawText(), "incorrect final comment text")
		assert.Equal(t, "/* another comment followed by some final whitespace*/", finalComments.Index(1).RawText(), "incorrect final comment text")
	}
	assert.Equal(t, "\n\n\t\n\t", eofNodeInfo.LeadingWhitespace(), "incorrect final whitespace")
}

func TestLexerErrors(t *testing.T) {
	testCases := []struct {
		str    string
		errMsg string
	}{
		{str: `0xffffffffffffffffffff`, errMsg: "value out of range"},
		{str: `"foobar`, errMsg: "unexpected EOF"},
		{str: `"foobar\J"`, errMsg: "invalid escape sequence"},
		{str: `"foobar\xgfoo"`, errMsg: "invalid hex escape"},
		{str: `"foobar\u09gafoo"`, errMsg: "invalid unicode escape"},
		{str: `"foobar\U0010005zfoo"`, errMsg: "invalid unicode escape"},
		{str: `"foobar\U00110000foo"`, errMsg: "unicode escape is out of range"},
		{str: "'foobar\nbaz'", errMsg: "encountered end-of-line"},
		{str: "'foobar\000baz'", errMsg: "null character ('\\0') not allowed"},
		{str: `1.543g12`, errMsg: "invalid syntax"},
		{str: `0.1234.5678.`, errMsg: "invalid syntax"},
		{str: `0x987.345aaf`, errMsg: "invalid syntax"},
		{str: `0.987.345`, errMsg: "invalid syntax"},
		{str: `0.987e34e-20`, errMsg: "invalid syntax"},
		{str: `0.987e-345e20`, errMsg: "invalid syntax"},
		{str: `.987to123`, errMsg: "invalid syntax"},
		{str: `/* foobar`, errMsg: "unexpected EOF"},
	}
	for i, tc := range testCases {
		handler := reporter.NewHandler(nil)
		l := newTestLexer(t, strings.NewReader(tc.str), handler)
		var sym protoSymType
		tok := l.Lex(&sym)
		if assert.Equal(t, _ERROR, tok) {
			assert.True(t, sym.err != nil)
			assert.True(t, strings.Contains(sym.err.Error(), tc.errMsg), "case %d: expected message to contain %q but does not: %q", i, tc.errMsg, sym.err.Error())
			t.Logf("case %d: %v", i, sym.err)
		}
	}
}

func newTestLexer(t *testing.T, in io.Reader, h *reporter.Handler) *protoLex {
	lexer, err := newLexer(in, "test.proto", h)
	require.NoError(t, err)
	return lexer
}
