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
	"io"
	"strings"
)

var closeSymbol = map[tokenType]tokenType{
	openParenToken:   closeParenToken,
	openBraceToken:   closeBraceToken,
	openBracketToken: closeBracketToken,
	openAngleToken:   closeAngleToken,
}

// ScanResult is the result of scanning a Protobuf source file. It contains the
// information extracted from the file.
type ScanResult struct {
	PackageName string
	Imports     []string
}

// ScanForImports scans the given reader, which should contain Protobuf source, and
// returns the set of imports declared in the file. The result also contains the
// value of any package declaration in in the file. It returns an error if there is
// an I/O error reading from r. In the event of such an error, it will still return
// a result that contains as much information as was found before the I/O error
// occurred.
func ScanForImports(r io.Reader) (ScanResult, error) {
	var res ScanResult

	var currentImport []string     // if non-nil, parsing an import statement
	var packageComponents []string // if non-nil, parsing a package statement

	// current stack of open blocks -- those starting with {, [, (, or < for
	// which we haven't yet encountered the closing }, ], ), or >
	var contextStack []tokenType
	declarationStart := true

	lexer := newLexer(r)
	for {
		token, text, err := lexer.Lex()
		if err != nil {
			return res, err
		}
		if token == eofToken {
			return res, nil
		}

		if currentImport != nil {
			switch token {
			case stringToken:
				currentImport = append(currentImport, text.(string))
			default:
				if len(currentImport) > 0 {
					res.Imports = append(res.Imports, strings.Join(currentImport, ""))
				}
				currentImport = nil
			}
		}

		if packageComponents != nil {
			switch token {
			case identifierToken:
				packageComponents = append(packageComponents, text.(string))
			case periodToken:
				packageComponents = append(packageComponents, ".")
			default:
				if len(packageComponents) > 0 {
					res.PackageName = strings.Join(packageComponents, "")
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
				} else if text == "package" {
					packageComponents = []string{}
				}
			}
		}

		declarationStart = token == closeBraceToken || token == semicolonToken
	}
}
