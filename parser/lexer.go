package parser

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/jhump/protocompile/ast"
	"github.com/jhump/protocompile/reporter"
)

type runeReader struct {
	data []byte
	pos  int
	err  error
	mark int
}

func (rr *runeReader) readRune() (r rune, size int, err error) {
	if rr.err != nil {
		return 0, 0, rr.err
	}
	if rr.pos == len(rr.data) {
		rr.err = io.EOF
		return 0, 0, rr.err
	}
	r, sz := utf8.DecodeRune(rr.data[rr.pos:])
	if r == utf8.RuneError {
		rr.err = fmt.Errorf("invalid UTF8 at offset %d: %x", rr.pos, rr.data[rr.pos])
		return 0, 0, rr.err
	}
	rr.pos = rr.pos + sz
	return r, sz, nil
}

func (rr *runeReader) offset() int {
	return rr.pos
}

func (rr *runeReader) unreadRune(sz int) {
	newPos := rr.pos - sz
	if newPos < rr.mark {
		panic("unread past mark")
	}
	rr.pos = newPos
}

func (rr *runeReader) setMark() {
	rr.mark = rr.pos
}

func (rr *runeReader) getMark() string {
	return string(rr.data[rr.mark:rr.pos])
}

type protoLex struct {
	input   *runeReader
	info    *ast.FileInfo
	handler *reporter.Handler
	res     *ast.FileNode

	prevSym    ast.TerminalNode
	prevOffset int
	eof        ast.Token

	comments []ast.Token
}

var utf8Bom = []byte{0xEF, 0xBB, 0xBF}

func newLexer(in io.Reader, filename string, handler *reporter.Handler) (*protoLex, error) {
	br := bufio.NewReader(in)

	// if file has UTF8 byte order marker preface, consume it
	marker, err := br.Peek(3)
	if err == nil && bytes.Equal(marker, utf8Bom) {
		_, _ = br.Discard(3)
	}

	contents, err := ioutil.ReadAll(br)
	if err != nil {
		return nil, err
	}
	return &protoLex{
		input:   &runeReader{data: contents},
		info:    ast.NewFileInfo(filename, contents),
		handler: handler,
	}, nil
}

var keywords = map[string]int{
	"syntax":     _SYNTAX,
	"import":     _IMPORT,
	"weak":       _WEAK,
	"public":     _PUBLIC,
	"package":    _PACKAGE,
	"option":     _OPTION,
	"true":       _TRUE,
	"false":      _FALSE,
	"inf":        _INF,
	"nan":        _NAN,
	"repeated":   _REPEATED,
	"optional":   _OPTIONAL,
	"required":   _REQUIRED,
	"double":     _DOUBLE,
	"float":      _FLOAT,
	"int32":      _INT32,
	"int64":      _INT64,
	"uint32":     _UINT32,
	"uint64":     _UINT64,
	"sint32":     _SINT32,
	"sint64":     _SINT64,
	"fixed32":    _FIXED32,
	"fixed64":    _FIXED64,
	"sfixed32":   _SFIXED32,
	"sfixed64":   _SFIXED64,
	"bool":       _BOOL,
	"string":     _STRING,
	"bytes":      _BYTES,
	"group":      _GROUP,
	"oneof":      _ONEOF,
	"map":        _MAP,
	"extensions": _EXTENSIONS,
	"to":         _TO,
	"max":        _MAX,
	"reserved":   _RESERVED,
	"enum":       _ENUM,
	"message":    _MESSAGE,
	"extend":     _EXTEND,
	"service":    _SERVICE,
	"rpc":        _RPC,
	"stream":     _STREAM,
	"returns":    _RETURNS,
}

func (l *protoLex) maybeNewLine(r rune) {
	if r == '\n' {
		l.info.AddLine(l.input.offset())
	}
}

func (l *protoLex) prev() ast.SourcePos {
	return l.info.SourcePos(l.prevOffset)
}

func (l *protoLex) Lex(lval *protoSymType) int {
	if l.handler.ReporterError() != nil {
		// if error reporter already returned non-nil error,
		// we can skip the rest of the input
		return 0
	}

	l.comments = nil

	for {
		l.input.setMark()

		l.prevOffset = l.input.offset()
		c, _, err := l.input.readRune()
		if err == io.EOF {
			// we're not actually returning a rune, but this will associate
			// accumulated comments as a trailing comment on last symbol
			// (if appropriate)
			l.setRune(lval, 0)
			l.eof = lval.b.Token()
			return 0
		} else if err != nil {
			l.setError(lval, err)
			return _ERROR
		}

		if strings.ContainsRune("\n\r\t\f\v ", c) {
			// skip whitespace
			l.maybeNewLine(c)
			continue
		}

		if c == '.' {
			// decimal literals could start with a dot
			cn, szn, err := l.input.readRune()
			if err != nil {
				l.setRune(lval, c)
				return int(c)
			}
			if cn >= '0' && cn <= '9' {
				l.readNumber()
				token := l.input.getMark()
				f, err := strconv.ParseFloat(token, 64)
				if err != nil {
					l.setError(lval, numError(err, "float", token))
					return _ERROR
				}
				l.setFloat(lval, f)
				return _FLOAT_LIT
			}
			l.input.unreadRune(szn)
			l.setRune(lval, c)
			return int(c)
		}

		if c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
			// identifier
			l.readIdentifier()
			token := l.input.getMark()
			str := string(token)
			if t, ok := keywords[str]; ok {
				l.setIdent(lval, str)
				return t
			}
			l.setIdent(lval, str)
			return _NAME
		}

		if c >= '0' && c <= '9' {
			// integer or float literal
			l.readNumber()
			token := l.input.getMark()
			if strings.HasPrefix(token, "0x") || strings.HasPrefix(token, "0X") {
				// hexadecimal
				ui, err := strconv.ParseUint(token[2:], 16, 64)
				if err != nil {
					l.setError(lval, numError(err, "hexadecimal integer", token[2:]))
					return _ERROR
				}
				l.setInt(lval, ui)
				return _INT_LIT
			}
			if strings.Contains(token, ".") || strings.Contains(token, "e") || strings.Contains(token, "E") {
				// floating point!
				f, err := strconv.ParseFloat(token, 64)
				if err != nil {
					l.setError(lval, numError(err, "float", token))
					return _ERROR
				}
				l.setFloat(lval, f)
				return _FLOAT_LIT
			}
			// integer! (decimal or octal)
			ui, err := strconv.ParseUint(token, 0, 64)
			if err != nil {
				kind := "integer"
				if numErr, ok := err.(*strconv.NumError); ok && numErr.Err == strconv.ErrRange {
					// if it's too big to be an int, parse it as a float
					var f float64
					kind = "float"
					f, err = strconv.ParseFloat(token, 64)
					if err == nil {
						l.setFloat(lval, f)
						return _FLOAT_LIT
					}
				}
				l.setError(lval, numError(err, kind, token))
				return _ERROR
			}
			l.setInt(lval, ui)
			return _INT_LIT
		}

		if c == '\'' || c == '"' {
			// string literal
			str, err := l.readStringLiteral(c)
			if err != nil {
				l.setError(lval, err)
				return _ERROR
			}
			l.setString(lval, str)
			return _STRING_LIT
		}

		if c == '/' {
			// comment
			cn, szn, err := l.input.readRune()
			if err != nil {
				l.setRune(lval, '/')
				return int(c)
			}
			if cn == '/' {
				l.skipToEndOfLineComment()
				l.comments = append(l.comments, l.newToken())
				continue
			}
			if cn == '*' {
				if ok := l.skipToEndOfBlockComment(); !ok {
					l.setError(lval, errors.New("block comment never terminates, unexpected EOF"))
					return _ERROR
				}
				l.comments = append(l.comments, l.newToken())
				continue
			}
			l.input.unreadRune(szn)
		}

		if c > 127 {
			l.setError(lval, errors.New("invalid character"))
			return _ERROR
		}
		l.setRune(lval, c)
		return int(c)
	}
}

func (l *protoLex) newToken() ast.Token {
	offset := l.input.mark
	length := l.input.pos - l.input.mark
	return l.info.AddToken(offset, length)
}

func (l *protoLex) setPrevAndAddComments(n ast.TerminalNode, isDot bool) {
	info := l.info.NodeInfo(n)
	nStart := info.Start().Line
	if _, ok := n.(*ast.RuneNode); ok {
		// This is really gross, but there are many cases where we don't want
		// to attribute comments to punctuation (like commas, equals, semicolons)
		// and would instead prefer to attribute comments to a more meaningful
		// element in the AST.
		//
		// So if it's a simple node OTHER THAN PERIOD (since that is not just
		// punctuation but typically part of a qualified identifier), don't
		// attribute comments to it. We do that with this TOTAL HACK: adjusting
		// the start line makes leading comments appear detached so logic below
		// will naturally associate trailing comment to previous symbol
		if !isDot {
			nStart += 2
		}
	}
	var prevEnd int
	if l.prevSym != nil {
		prevEnd = l.info.NodeInfo(l.prevSym).End().Line
	}
	comments := l.comments
	l.comments = nil
	var prevTrailingComments []ast.Token
	if l.prevSym != nil && len(comments) > 0 && prevEnd < nStart {
		// we may need to re-attribute the first comment to
		// instead be previous node's trailing comment
		c := comments[0]
		commentInfo := l.info.TokenInfo(c)
		commentStart := commentInfo.Start().Line
		if commentStart == prevEnd {
			// comment is on same line as previous symbol
			comments = comments[1:]
			prevTrailingComments = []ast.Token{c}
		} else if commentStart == prevEnd+1 {
			// comment is right after previous symbol; see if it is detached
			// and if so re-attribute
			prevSingleLineStyle := strings.HasPrefix(commentInfo.RawText(), "//")
			prevLine := commentInfo.End().Line
			groupEnd := 0
			for i := 1; i < len(comments); i++ {
				c := comments[i]
				commentInfo := l.info.TokenInfo(c)
				newGroup := false
				if !prevSingleLineStyle || commentInfo.Start().Line > prevLine+1 {
					// we've found a gap between comments, which means the
					// previous comments were detached
					newGroup = true
				} else {
					singleLineStyle := strings.HasPrefix(commentInfo.RawText(), "//")
					if !singleLineStyle {
						// we've found a switch from // comments to /*
						// consider that a new group which means the
						// previous comments were detached
						newGroup = true
					}
					prevLine = commentInfo.End().Line
					prevSingleLineStyle = singleLineStyle
				}
				if newGroup {
					groupEnd = i
					break
				}
			}

			if groupEnd == 0 {
				// just one group of comments; we'll mark it as a trailing
				// comment if it immediately follows previous symbol and is
				// detached from current symbol
				c1 := comments[0]
				c2 := comments[len(comments)-1]
				c1info := l.info.TokenInfo(c1)
				c2info := l.info.TokenInfo(c2)
				if c1info.Start().Line <= prevEnd+1 && c2info.End().Line < nStart-1 {
					groupEnd = len(comments)
				}
			}

			prevTrailingComments = comments[:groupEnd]
			comments = comments[groupEnd:]
		}
	}

	// now we can associate comments
	for _, c := range prevTrailingComments {
		l.info.AddComment(c, l.prevSym.Token())
	}
	for _, c := range comments {
		l.info.AddComment(c, n.Token())
	}

	l.prevSym = n
}

func (l *protoLex) setString(lval *protoSymType, val string) {
	lval.s = ast.NewStringLiteralNode(val, l.newToken())
	l.setPrevAndAddComments(lval.s, false)
}

func (l *protoLex) setIdent(lval *protoSymType, val string) {
	lval.id = ast.NewIdentNode(val, l.newToken())
	l.setPrevAndAddComments(lval.id, false)
}

func (l *protoLex) setInt(lval *protoSymType, val uint64) {
	lval.i = ast.NewUintLiteralNode(val, l.newToken())
	l.setPrevAndAddComments(lval.i, false)
}

func (l *protoLex) setFloat(lval *protoSymType, val float64) {
	lval.f = ast.NewFloatLiteralNode(val, l.newToken())
	l.setPrevAndAddComments(lval.f, false)
}

func (l *protoLex) setRune(lval *protoSymType, val rune) {
	lval.b = ast.NewRuneNode(val, l.newToken())
	l.setPrevAndAddComments(lval.b, val == '.')
}

func (l *protoLex) setError(lval *protoSymType, err error) {
	lval.err = l.addSourceError(err)
}

func (l *protoLex) readNumber() {
	allowExpSign := false
	for {
		c, sz, err := l.input.readRune()
		if err != nil {
			break
		}
		if (c == '-' || c == '+') && !allowExpSign {
			l.input.unreadRune(sz)
			break
		}
		allowExpSign = false
		if c != '.' && c != '_' && (c < '0' || c > '9') &&
			(c < 'a' || c > 'z') && (c < 'A' || c > 'Z') &&
			c != '-' && c != '+' {
			// no more chars in the number token
			l.input.unreadRune(sz)
			break
		}
		if c == 'e' || c == 'E' {
			// scientific notation char can be followed by
			// an exponent sign
			allowExpSign = true
		}
	}
}

func numError(err error, kind, s string) error {
	ne, ok := err.(*strconv.NumError)
	if !ok {
		return err
	}
	if ne.Err == strconv.ErrRange {
		return fmt.Errorf("value out of range for %s: %s", kind, s)
	}
	// syntax error
	return fmt.Errorf("invalid syntax in %s value: %s", kind, s)
}

func (l *protoLex) readIdentifier() {
	for {
		c, sz, err := l.input.readRune()
		if err != nil {
			break
		}
		if c != '_' && (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
			l.input.unreadRune(sz)
			break
		}
	}
}

func (l *protoLex) readStringLiteral(quote rune) (string, error) {
	var buf bytes.Buffer
	for {
		c, _, err := l.input.readRune()
		if err != nil {
			if err == io.EOF {
				err = io.ErrUnexpectedEOF
			}
			return "", err
		}
		if c == '\n' {
			return "", errors.New("encountered end-of-line before end of string literal")
		}
		if c == quote {
			break
		}
		if c == 0 {
			return "", errors.New("null character ('\\0') not allowed in string literal")
		}
		if c == '\\' {
			// escape sequence
			c, _, err = l.input.readRune()
			if err != nil {
				return "", err
			}
			if c == 'x' || c == 'X' {
				// hex escape
				c, _, err := l.input.readRune()
				if err != nil {
					return "", err
				}
				c2, sz2, err := l.input.readRune()
				if err != nil {
					return "", err
				}
				var hex string
				if (c2 < '0' || c2 > '9') && (c2 < 'a' || c2 > 'f') && (c2 < 'A' || c2 > 'F') {
					l.input.unreadRune(sz2)
					hex = string(c)
				} else {
					hex = string([]rune{c, c2})
				}
				i, err := strconv.ParseInt(hex, 16, 32)
				if err != nil {
					return "", fmt.Errorf("invalid hex escape: \\x%q", hex)
				}
				buf.WriteByte(byte(i))

			} else if c >= '0' && c <= '7' {
				// octal escape
				c2, sz2, err := l.input.readRune()
				if err != nil {
					return "", err
				}
				var octal string
				if c2 < '0' || c2 > '7' {
					l.input.unreadRune(sz2)
					octal = string(c)
				} else {
					c3, sz3, err := l.input.readRune()
					if err != nil {
						return "", err
					}
					if c3 < '0' || c3 > '7' {
						l.input.unreadRune(sz3)
						octal = string([]rune{c, c2})
					} else {
						octal = string([]rune{c, c2, c3})
					}
				}
				i, err := strconv.ParseInt(octal, 8, 32)
				if err != nil {
					return "", fmt.Errorf("invalid octal escape: \\%q", octal)
				}
				if i > 0xff {
					return "", fmt.Errorf("octal escape is out range, must be between 0 and 377: \\%q", octal)
				}
				buf.WriteByte(byte(i))

			} else if c == 'u' {
				// short unicode escape
				u := make([]rune, 4)
				for i := range u {
					c, _, err := l.input.readRune()
					if err != nil {
						return "", err
					}
					u[i] = c
				}
				i, err := strconv.ParseInt(string(u), 16, 32)
				if err != nil {
					return "", fmt.Errorf("invalid unicode escape: \\u%q", string(u))
				}
				buf.WriteRune(rune(i))

			} else if c == 'U' {
				// long unicode escape
				u := make([]rune, 8)
				for i := range u {
					c, _, err := l.input.readRune()
					if err != nil {
						return "", err
					}
					u[i] = c
				}
				i, err := strconv.ParseInt(string(u), 16, 32)
				if err != nil {
					return "", fmt.Errorf("invalid unicode escape: \\U%q", string(u))
				}
				if i > 0x10ffff || i < 0 {
					return "", fmt.Errorf("unicode escape is out of range, must be between 0 and 0x10ffff: \\U%q", string(u))
				}
				buf.WriteRune(rune(i))

			} else if c == 'a' {
				buf.WriteByte('\a')
			} else if c == 'b' {
				buf.WriteByte('\b')
			} else if c == 'f' {
				buf.WriteByte('\f')
			} else if c == 'n' {
				buf.WriteByte('\n')
			} else if c == 'r' {
				buf.WriteByte('\r')
			} else if c == 't' {
				buf.WriteByte('\t')
			} else if c == 'v' {
				buf.WriteByte('\v')
			} else if c == '\\' {
				buf.WriteByte('\\')
			} else if c == '\'' {
				buf.WriteByte('\'')
			} else if c == '"' {
				buf.WriteByte('"')
			} else if c == '?' {
				buf.WriteByte('?')
			} else {
				return "", fmt.Errorf("invalid escape sequence: %q", "\\"+string(c))
			}
		} else {
			buf.WriteRune(c)
		}
	}
	return buf.String(), nil
}

func (l *protoLex) skipToEndOfLineComment() {
	for {
		c, _, err := l.input.readRune()
		if err != nil {
			return
		}
		if c == '\n' {
			l.info.AddLine(l.input.offset())
			return
		}
	}
}

func (l *protoLex) skipToEndOfBlockComment() bool {
	for {
		c, _, err := l.input.readRune()
		if err != nil {
			return false
		}
		l.maybeNewLine(c)
		if c == '*' {
			c, sz, err := l.input.readRune()
			if err != nil {
				return false
			}
			if c == '/' {
				return true
			}
			l.input.unreadRune(sz)
		}
	}
}

func (l *protoLex) addSourceError(err error) reporter.ErrorWithPos {
	ewp, ok := err.(reporter.ErrorWithPos)
	if !ok {
		ewp = reporter.Error(l.prev(), err)
	}
	_ = l.handler.HandleError(ewp)
	return ewp
}

func (l *protoLex) Error(s string) {
	_ = l.addSourceError(errors.New(s))
}
