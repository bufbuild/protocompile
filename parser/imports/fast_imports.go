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
	"io"
	"strings"
)

var closeSymbol = map[tokenType]tokenType{
	openParenToken:   closeParenToken,
	openBraceToken:   closeBraceToken,
	openBracketToken: closeBracketToken,
	openAngleToken:   closeAngleToken,
}

// ScanForImports scans the given reader, which should contain Protobuf source, and
// returns the set of imports declared in the file. It returns an error if there is
// an I/O error reading from r. In the event of such an error, it will still return
// a slice of imports that contains as many imports as were found before the I/O
// error occurred.
func ScanForImports(r io.Reader) ([]string, error) {
	var imports []string
	var contextStack []tokenType
	var currentImport []string
	lexer := newLexer(r)
	for {
		token, text, err := lexer.Lex()
		if err != nil {
			return imports, err
		}
		if token == eofToken {
			return imports, nil
		}

		if currentImport != nil {
			switch token {
			case stringToken:
				currentImport = append(currentImport, text.(string))
			default:
				if len(currentImport) > 0 {
					imports = append(imports, strings.Join(currentImport, ""))
				}
				currentImport = nil
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
			if text == "import" && len(contextStack) == 0 {
				currentImport = []string{}
			}
		}
	}
}
