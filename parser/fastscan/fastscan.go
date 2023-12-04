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

package fastscan

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/bufbuild/protocompile/ast"
)

var closeSymbol = map[tokenType]tokenType{
	openParenToken:   closeParenToken,
	openBraceToken:   closeBraceToken,
	openBracketToken: closeBracketToken,
	openAngleToken:   closeAngleToken,
}

// Result is the result of scanning a Protobuf source file. It contains the
// information extracted from the file.
type Result struct {
	PackageName string
	Imports     []Import
}

// Import represents an import in a Protobuf source file.
type Import struct {
	// Path of the imported file.
	Path string
	// Indicate if public or weak keyword was used in import statement.
	IsPublic, IsWeak bool
}

// SyntaxError is returned from Scan when one or more syntax errors are observed.
// Scan does not fully parse the source, so there are many kinds of syntax errors
// that will not be recognized. A full parser should be used to reliably detect
// errors in the source. But if the scanner happens to see things that are clearly
// wrong while scanning for the package and imports, it will return them.
type SyntaxError struct {
	errs []singleSyntaxError
}

// Error implements the error interface, returning an error message with the
// details of the syntax error issues.
func (e *SyntaxError) Error() string {
	var buf bytes.Buffer
	for i := range e.errs {
		if i > 0 {
			buf.WriteByte('\n')
		}
		buf.WriteString(e.errs[i].Error())
	}
	return buf.String()
}

// Len returns the number of different locations where a syntax error was
// identified.
func (e *SyntaxError) Len() int {
	return len(e.errs)
}

// Get returns details for a syntax error at a single location.
func (e *SyntaxError) Get(i int) error {
	return &e.errs[i]
}

// GetPosition returns the location in the source file where a syntax error
// was identified.
func (e *SyntaxError) GetPosition(i int) ast.SourcePos {
	return ast.SourcePos{
		Filename: e.errs[i].filename,
		Line:     e.errs[i].line,
		Col:      e.errs[i].col,
	}
}

// Unwrap returns an error for each location where a syntax error was
// identified.
func (e *SyntaxError) Unwrap() []error {
	slice := make([]error, len(e.errs))
	for i := range e.errs {
		slice[i] = &e.errs[i]
	}
	return slice
}

func newSyntaxError(errs []singleSyntaxError) error {
	if len(errs) == 0 {
		return nil
	}
	return &SyntaxError{errs: errs}
}

type singleSyntaxError struct {
	msg       string
	filename  string
	line, col int
}

func (s *singleSyntaxError) Error() string {
	file := s.filename
	if file == "" {
		file = "<input>"
	}
	return fmt.Sprintf("%s:%d:%d: %s", file, s.line, s.col, s.msg)
}

// Scan scans the given reader, which should contain Protobuf source, and
// returns the set of imports declared in the file. The result also contains the
// value of any package declaration in the file. It returns an error if there is
// an I/O error reading from r or if syntax errors are recognized while scanning.
// In the event of such an error, it will still return a result that contains as
// much information as was found (either before the I/O error occurred, or all
// that could be parsed despite syntax errors). The results are not necessarily
// valid, in that the parsed package name might not be a legal package name in
// protobuf or the imports may not refer to valid paths. Full validation of the
// source should be done using a full parser.
func Scan(filename string, r io.Reader) (Result, error) {
	var res Result

	var currentImport []string     // if non-nil, parsing an import statement
	var isPublic, isWeak bool      // if public or weak keyword observed in current import statement
	var packageComponents []string // if non-nil, parsing a package statement
	var syntaxErrs []singleSyntaxError

	// current stack of open blocks -- those starting with {, [, (, or < for
	// which we haven't yet encountered the closing }, ], ), or >
	var contextStack []tokenType
	declarationStart := true

	var prevLine, prevCol int
	lexer := newLexer(r)
	for {
		token, text, err := lexer.Lex()
		if err != nil {
			return res, err
		}
		if token == eofToken {
			return res, newSyntaxError(syntaxErrs)
		}

		if currentImport != nil {
			switch token {
			case stringToken:
				currentImport = append(currentImport, text.(string))
			case identifierToken:
				ident := text.(string) //nolint:errcheck
				if len(currentImport) == 0 && (ident == "public" || ident == "weak") {
					isPublic = ident == "public"
					isWeak = ident == "weak"
					break
				}
				fallthrough
			default:
				if len(currentImport) > 0 {
					if token != semicolonToken {
						syntaxErrs = append(syntaxErrs, singleSyntaxError{
							msg:  fmt.Sprintf("unexpected %s; expecting semicolon", token.describe()),
							line: lexer.prevTokenLine + 1,
							col:  lexer.prevTokenCol + 1,
						})
					}
					res.Imports = append(res.Imports, Import{
						Path:     strings.Join(currentImport, ""),
						IsPublic: isPublic,
						IsWeak:   isWeak,
					})
				} else {
					syntaxErrs = append(syntaxErrs, singleSyntaxError{
						msg:      fmt.Sprintf("unexpected %s; expecting import path string", token.describe()),
						filename: filename,
						line:     lexer.prevTokenLine + 1,
						col:      lexer.prevTokenCol + 1,
					})
				}
				currentImport = nil
			}
		}

		if packageComponents != nil {
			switch token {
			case identifierToken:
				if len(packageComponents) > 0 && packageComponents[len(packageComponents)-1] != "." {
					syntaxErrs = append(syntaxErrs, singleSyntaxError{
						msg:  "package name should have a period between name components",
						line: lexer.prevTokenLine + 1,
						col:  lexer.prevTokenCol + 1,
					})
				}
				packageComponents = append(packageComponents, text.(string))
			case periodToken:
				if len(packageComponents) == 0 {
					syntaxErrs = append(syntaxErrs, singleSyntaxError{
						msg:  "package name should not begin with a period",
						line: lexer.prevTokenLine + 1,
						col:  lexer.prevTokenCol + 1,
					})
				} else if packageComponents[len(packageComponents)-1] == "." {
					syntaxErrs = append(syntaxErrs, singleSyntaxError{
						msg:  "package name should not have two periods in a row",
						line: lexer.prevTokenLine + 1,
						col:  lexer.prevTokenCol + 1,
					})
				}
				packageComponents = append(packageComponents, ".")
			default:
				if len(packageComponents) > 0 {
					if token != semicolonToken {
						syntaxErrs = append(syntaxErrs, singleSyntaxError{
							msg:  fmt.Sprintf("unexpected %s; expecting semicolon", token.describe()),
							line: lexer.prevTokenLine + 1,
							col:  lexer.prevTokenCol + 1,
						})
					}
					if packageComponents[len(packageComponents)-1] == "." {
						syntaxErrs = append(syntaxErrs, singleSyntaxError{
							msg:  "package name should not end with a period",
							line: prevLine + 1,
							col:  prevCol + 1,
						})
					}
					res.PackageName = strings.Join(packageComponents, "")
				} else {
					syntaxErrs = append(syntaxErrs, singleSyntaxError{
						msg:  fmt.Sprintf("unexpected %s; expecting package name", token.describe()),
						line: lexer.prevTokenLine + 1,
						col:  lexer.prevTokenCol + 1,
					})
				}
				packageComponents = nil
			}
		}

		switch token {
		case openParenToken, openBraceToken, openBracketToken, openAngleToken:
			contextStack = append(contextStack, closeSymbol[token])
		case closeParenToken, closeBraceToken, closeBracketToken, closeAngleToken:
			if len(contextStack) > 0 && contextStack[len(contextStack)-1] == token {
				contextStack = contextStack[:len(contextStack)-1]
			}
		case identifierToken:
			if declarationStart && len(contextStack) == 0 {
				if text == "import" {
					currentImport = []string{}
					isPublic, isWeak = false, false
				} else if text == "package" {
					packageComponents = []string{}
				}
			}
		}

		declarationStart = token == closeBraceToken || token == semicolonToken
		prevLine, prevCol = lexer.prevTokenLine, lexer.prevTokenCol
	}
}
