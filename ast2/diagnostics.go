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

package ast2

import "errors"

var (
	// Well-known diagnostics, so that the Err field of a [report2.Diagnostic] can be checked
	// for specific values. Not all diagnostics will have a well-known value, and some
	// diagnostics are represented by types (which all start with Err*) rather than specific values.
	ErrUnrecognized              = errors.New("unrecongnized token")
	ErrUnopenedDelimiter         = errors.New("unopened brace delimiter")
	ErrUnclosedDelimiter         = errors.New("unclosed brace delimiter")
	ErrUnterminatedBlockComment  = errors.New("unterminated block comment")
	ErrUnterminatedStringLiteral = errors.New("unterminated string literal")

	ErrInvalidEscape = errors.New("invalid escape sequence")
	ErrNonASCIIIdent = errors.New("non-ASCII identifiers are not permitted")

	ErrIntegerOverflow     = errors.New("invalid integer literal")
	ErrInvalidDecLiteral   = errors.New("invalid decimal integer literal")
	ErrInvalidBinLiteral   = errors.New("invalid binary integer literal")
	ErrInvalidOctLiteral   = errors.New("invalid octal integer literal")
	ErrInvalidHexLiteral   = errors.New("invalid hexadecimal integer literal")
	ErrInvalidFloatLiteral = errors.New("invalid floating-point number literal")
)
