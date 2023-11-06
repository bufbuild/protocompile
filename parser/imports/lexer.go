// Copyright 2020-2023 Buf Technologies, Inc.
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

package imports

import (
	"bufio"
	"bytes"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"
)

// This file contains a streaming lexer. The lexer in the parser package loads the
// entire file into memory. But this lexer is much more memory efficient -- since it
// does not produce an AST or descriptor, there's no need to have the entire file
// loaded in memory.
//
// This was adapted from the streaming lexer in github.com/jhump/protoreflect/desc/protoparse@v1.14.1.
//
// This version is very lenient. It does not report any syntax-related errors. Any
// invalid token or symbol is effectively ignored. Invalid escapes inside string literals
// are left as is instead of resulting in failure. The idea is to do a very fast scan of
// tokens instead of a full parse; since we aren't applying grammar rules anyway, we don't
// really need all the validation. The only benefit of validation might be to short-circuit
// lexing the whole file if it's very obviously NOT a protobuf source. But only extremely
// egregious cases could be really be detected since we're not actually parsing.

type tokenType int

const (
	eofToken = tokenType(iota)

	stringToken = tokenType(iota + 65536)
	numberToken
	identifierToken

	// token type for punctuation/symbols is their ACII value
	openParenToken    = tokenType('(')
	openBraceToken    = tokenType('{')
	openBracketToken  = tokenType('[')
	openAngleToken    = tokenType('<')
	closeParenToken   = tokenType(')')
	closeBraceToken   = tokenType('}')
	closeBracketToken = tokenType(']')
	closeAngleToken   = tokenType('>')
)

type runeReader struct {
	rr     *bufio.Reader
	marked []rune
	unread []rune
	err    error
}

func (rr *runeReader) readRune() (r rune, size int, err error) {
	if rr.err != nil {
		return 0, 0, rr.err
	}
	if len(rr.unread) > 0 {
		r := rr.unread[len(rr.unread)-1]
		rr.unread = rr.unread[:len(rr.unread)-1]
		if rr.marked != nil {
			rr.marked = append(rr.marked, r)
		}
		return r, utf8.RuneLen(r), nil
	}
	r, sz, err := rr.rr.ReadRune()
	if err != nil {
		rr.err = err
	} else if rr.marked != nil {
		rr.marked = append(rr.marked, r)
	}
	return r, sz, err
}

func (rr *runeReader) unreadRune(r rune) {
	if rr.marked != nil {
		if rr.marked[len(rr.marked)-1] != r {
			panic("unread rune is not the same as last marked rune!")
		}
		rr.marked = rr.marked[:len(rr.marked)-1]
	}
	rr.unread = append(rr.unread, r)
}

func (rr *runeReader) startMark(initial rune) {
	rr.marked = []rune{initial}
}

func (rr *runeReader) endMark() string {
	m := string(rr.marked)
	rr.marked = rr.marked[:0]
	return m
}

type protoLex struct {
	input *runeReader
}

var utf8Bom = []byte{0xEF, 0xBB, 0xBF}

func newLexer(in io.Reader) *protoLex {
	br := bufio.NewReader(in)

	// if file has UTF8 byte order marker preface, consume it
	marker, err := br.Peek(3)
	if err == nil && bytes.Equal(marker, utf8Bom) {
		_, _ = br.Discard(3)
	}

	return &protoLex{
		input: &runeReader{rr: br},
	}
}

func (l *protoLex) Lex() (tokenType, any, error) {
	l.input.endMark() // reset, just in case

	for {
		c, _, err := l.input.readRune()
		if err == io.EOF {
			// we're not actually returning a rune, but this will associate
			// accumulated comments as a trailing comment on last symbol
			// (if appropriate)
			return eofToken, nil, nil
		} else if err != nil {
			// we don't call setError because we don't want it wrapped
			// with a source position because it's I/O, not syntax
			return 0, nil, err
		}

		if strings.ContainsRune("\n\r\t\f\v ", c) {
			continue
		}

		l.input.startMark(c)
		if c == '.' {
			// decimal literals could start with a dot
			cn, _, err := l.input.readRune()
			if err != nil {
				return tokenType(c), nil, nil
			}
			if cn >= '0' && cn <= '9' {
				token := l.readNumber(c, cn)
				return numberToken, token, nil
			}
			l.input.unreadRune(cn)
			return tokenType(c), nil, nil
		}

		if c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
			// identifier
			token := []rune{c}
			token = l.readIdentifier(token)
			return identifierToken, string(token), nil
		}

		if c >= '0' && c <= '9' {
			// integer or float literal
			token := l.readNumber(c)
			return numberToken, token, nil
		}

		if c == '\'' || c == '"' {
			// string literal
			str := l.readStringLiteral(c)
			return stringToken, str, nil
		}

		if c == '/' {
			// comment
			cn, _, err := l.input.readRune()
			if err != nil {
				return tokenType(c), nil, nil
			}
			if cn == '/' {
				l.skipToEndOfLineComment()
				continue
			}
			if cn == '*' {
				l.skipToEndOfBlockComment()
				continue
			}
			l.input.unreadRune(cn)
		}

		return tokenType(c), nil, nil
	}
}

func (l *protoLex) readNumber(sofar ...rune) string {
	token := sofar
	allowExpSign := false
	for {
		c, _, err := l.input.readRune()
		if err != nil {
			break
		}
		if (c == '-' || c == '+') && !allowExpSign {
			l.input.unreadRune(c)
			break
		}
		allowExpSign = false
		if c != '.' && c != '_' && (c < '0' || c > '9') &&
			(c < 'a' || c > 'z') && (c < 'A' || c > 'Z') &&
			c != '-' && c != '+' {
			// no more chars in the number token
			l.input.unreadRune(c)
			break
		}
		if c == 'e' || c == 'E' {
			// scientific notation char can be followed by
			// an exponent sign
			allowExpSign = true
		}
		token = append(token, c)
	}
	return string(token)
}

func (l *protoLex) readIdentifier(sofar []rune) []rune {
	token := sofar
	for {
		c, _, err := l.input.readRune()
		if err != nil {
			break
		}
		if c != '_' && (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
			l.input.unreadRune(c)
			break
		}
		token = append(token, c)
	}
	return token
}

func (l *protoLex) readStringLiteral(quote rune) string {
	var buf bytes.Buffer
	for {
		c, _, err := l.input.readRune()
		if err != nil {
			break
		}
		if c == quote {
			break
		}
		if c == '\\' {
			// escape sequence
			c, _, err = l.input.readRune()
			if err != nil {
				buf.WriteByte('\\')
				break
			}
			if c == 'x' || c == 'X' {
				// hex escape
				c1, _, err := l.input.readRune()
				if err != nil {
					buf.WriteByte('\\')
					buf.WriteRune(c)
					break
				}
				c2, _, err := l.input.readRune()
				if err != nil {
					buf.WriteByte('\\')
					buf.WriteRune(c)
					buf.WriteRune(c1)
					break
				}
				var hex string
				if (c2 < '0' || c2 > '9') && (c2 < 'a' || c2 > 'f') && (c2 < 'A' || c2 > 'F') {
					l.input.unreadRune(c2)
					hex = string(c1)
				} else {
					hex = string([]rune{c1, c2})
				}
				i, err := strconv.ParseInt(hex, 16, 32)
				if err != nil {
					// just include raw, invalid hex escape
					buf.WriteByte('\\')
					buf.WriteRune(c)
					buf.WriteString(hex)
				} else {
					buf.WriteByte(byte(i))
				}

			} else if c >= '0' && c <= '7' {
				// octal escape
				c2, _, err := l.input.readRune()
				if err != nil {
					buf.WriteByte('\\')
					buf.WriteRune(c)
					break
				}
				var octal string
				if c2 < '0' || c2 > '7' {
					l.input.unreadRune(c2)
					octal = string(c)
				} else {
					c3, _, err := l.input.readRune()
					if err != nil {
						buf.WriteByte('\\')
						buf.WriteRune(c)
						buf.WriteRune(c2)
						break
					}
					if c3 < '0' || c3 > '7' {
						l.input.unreadRune(c3)
						octal = string([]rune{c, c2})
					} else {
						octal = string([]rune{c, c2, c3})
					}
				}
				i, err := strconv.ParseInt(octal, 8, 32)
				if err != nil || i > 0xff {
					// just include raw, invalid octal escape
					buf.WriteByte('\\')
					buf.WriteString(octal)
				} else {
					buf.WriteByte(byte(i))
				}

			} else if c == 'u' {
				// short unicode escape
				u := make([]rune, 4)
				for i := range u {
					c, _, err := l.input.readRune()
					if err != nil {
						buf.WriteString("\\u")
						for j := 0; j > i; j++ {
							buf.WriteRune(u[j])
						}
						break
					}
					u[i] = c
				}
				i, err := strconv.ParseInt(string(u), 16, 32)
				if err != nil {
					// just include raw, invalid unicode escape
					buf.WriteString("\\u")
					for _, r := range u {
						buf.WriteRune(r)
					}
				} else {
					buf.WriteRune(rune(i))
				}

			} else if c == 'U' {
				// long unicode escape
				u := make([]rune, 8)
				for i := range u {
					c, _, err := l.input.readRune()
					if err != nil {
						buf.WriteString("\\U")
						for j := 0; j > i; j++ {
							buf.WriteRune(u[j])
						}
						break
					}
					u[i] = c
				}
				i, err := strconv.ParseInt(string(u), 16, 32)
				if err != nil || i > 0x10ffff || i < 0 {
					// just include raw, invalid unicode escape
					buf.WriteString("\\U")
					for _, r := range u {
						buf.WriteRune(r)
					}
				} else {
					buf.WriteRune(rune(i))
				}

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
				// just include raw, invalid escape
				buf.WriteByte('\\')
				buf.WriteRune(c)
			}
		} else {
			buf.WriteRune(c)
		}
	}
	return buf.String()
}

func (l *protoLex) skipToEndOfLineComment() {
	for {
		c, _, err := l.input.readRune()
		if err != nil {
			return
		}
		if c == '\n' {
			return
		}
	}
}

func (l *protoLex) skipToEndOfBlockComment() {
	for {
		c, _, err := l.input.readRune()
		if err != nil {
			return
		}
		if c == '*' {
			c, _, err := l.input.readRune()
			if err != nil {
				return
			}
			if c == '/' {
				return
			}
			l.input.unreadRune(c)
		}
	}
}
