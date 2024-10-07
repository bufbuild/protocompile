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

package ast

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/bufbuild/protocompile/experimental/report"
)

const MaxFileSize int = math.MaxInt32

// ErrFileTooBig diagnoses a file that is beyond Protocompile's implementation limits.
type ErrFileTooBig struct {
	Path string
}

func (e ErrFileTooBig) Error() string {
	return "files larger than 2GB are not supported"
}

func (e ErrFileTooBig) Diagnose(d *report.Diagnostic) {
	d.With(report.InFile(e.Path))
}

// ErrNotUTF8 diagnoses a file that contains non-UTF-8 bytes.
type ErrNotUTF8 struct {
	Path string
	At   int
	Byte byte
}

func (e ErrNotUTF8) Error() string {
	return "files must be encoded as valid UTF-8"
}

func (e ErrNotUTF8) Diagnose(d *report.Diagnostic) {
	d.With(
		report.InFile(e.Path),
		report.Notef("found a 0x%02x byte at offset %d", e.Byte, e.At),
	)
}

// ErrUnrecognized diagnoses the presence of an unrecognized token.
type ErrUnrecognized struct{ Token Token }

func (e ErrUnrecognized) Error() string {
	return "unrecongnized token"
}

func (e ErrUnrecognized) Diagnose(d *report.Diagnostic) {
	d.With(report.Snippet(e.Token))
}

// ErrUnterminated diagnoses a delimiter for which we found one half of a matched
// delimiter but not the other.
type ErrUnterminated struct {
	Span Span

	// If present, this indicates that we did match with another brace delimiter, but it
	// was of the wrong kind
	Mismatch Span
}

// OpenClose returns the expected open/close delimiters for this matched pair.
func (e ErrUnterminated) OpenClose() (string, string) {
	switch t := e.Span.Text(); t {
	case "(", ")":
		return "(", ")"
	case "[", "]":
		return "[", "]"
	case "{", "}":
		return "{", "}"
	case "<", ">":
		return "<", ">"
	case "/*", "*/":
		return "/*", "*/"
	default:
		start, end := e.Span.Offsets()
		panic(fmt.Sprintf("protocompile/ast: invalid token in ErrUnterminated: %q (byte offset %d:%d)", t, start, end))
	}
}

func (e ErrUnterminated) Error() string {
	return fmt.Sprintf("encountered unterminated `%s` delimiter", e.Span.Text())
}

func (e ErrUnterminated) Diagnose(d *report.Diagnostic) {
	text := e.Span.Text()
	open, close := e.OpenClose()

	if text == open {
		d.With(report.Snippetf(e.Span, "expected to be closed by `%s", close))
		if !e.Mismatch.Nil() {
			d.With(report.Snippetf(e.Mismatch, "closed by this instead"))
		}
	} else {
		d.With(report.Snippetf(e.Span, "expected to be opened by `%s", open))
	}
	if text == "*/" {
		d.With(report.Note("Protobuf does not support nested block comments"))
	}
}

// ErrUnterminatedStringLiteral diagnoses a string literal that continues to EOF.
type ErrUnterminatedStringLiteral struct{ Token Token }

func (e ErrUnterminatedStringLiteral) Error() string {
	return "unterminated string literal"
}

func (e ErrUnterminatedStringLiteral) Diagnose(d *report.Diagnostic) {
	open := e.Token.Text()[:1]
	d.With(report.Snippetf(e.Token, "expected to be terminated by `%s`", open))
	// TODO: check to see if a " or ' escape exists in the string?
}

// ErrInvalidEscape diagnoses an invalid escape sequence.
type ErrInvalidEscape struct{ Span Span }

func (e ErrInvalidEscape) Error() string {
	return "invalid escape sequence"
}

func (e ErrInvalidEscape) Diagnose(d *report.Diagnostic) {
	text := e.Span.Text()

	if len(text) >= 2 {
		switch c := text[1]; c {
		case 'x':
			if len(text) < 3 {
				d.With(report.Snippetf(e.Span, "`\\x` must be followed by at least one hex digit"))
				return
			}
		case 'u', 'U':
			expected := 4
			if c == 'U' {
				expected = 8
			}

			if len(text[2:]) != expected {
				d.With(report.Snippetf(e.Span, "`\\%c` must be followed by exactly %d hex digits", c, expected))
				return
			}

			value, _ := strconv.ParseUint(text[2:], 16, 32)
			if !utf8.ValidRune(rune(value)) {
				d.With(report.Snippetf(e.Span, "must be in the range U+0000 to U+10FFFF, except U+DC00 to U+DFFF"))
				return
			}
		}
	}

	d.With(report.Snippet(e.Span))
}

// ErrNonASCIIIdent diagnoses an identifier that contains non-ASCII runes.
type ErrNonASCIIIdent struct{ Token Token }

func (e ErrNonASCIIIdent) Error() string {
	return "non-ASCII identifiers are not allowed"
}

func (e ErrNonASCIIIdent) Diagnose(d *report.Diagnostic) {
	d.With(report.Snippet(e.Token))
}

// ErrIntegerOverflow diagnoses an integer literal that does not fit into the required range.
type ErrIntegerOverflow struct {
	Token Token
	// TODO: Extend this to other bit-sizes.
}

func (e ErrIntegerOverflow) Error() string {
	return "integer literal out of range"
}

func (e ErrIntegerOverflow) Diagnose(d *report.Diagnostic) {
	text := strings.ToLower(e.Token.Text())
	switch {
	case strings.HasPrefix(text, "0x"):
		d.With(
			report.Snippetf(e.Token, "must be in the range `0x0` to `0x%x`", uint64(math.MaxUint64)),
			report.Note("hexadecimal literals must always fit in a uint64"),
		)
	case strings.HasPrefix(text, "0b"):
		d.With(
			report.Snippetf(e.Token, "must be in the range `0b0` to `0b%b`", uint64(math.MaxUint64)),
			report.Note("binary literals must always fit in a uint64"),
		)
	default:
		// NB: Decimal literals cannot overflow, at least in the lexer.
		d.With(
			report.Snippetf(e.Token, "must be in the range `0` to `0%o`", uint64(math.MaxUint64)),
			report.Note("octal literals must always fit in a uint64"),
		)
	}
}

// ErrInvalidNumber diagnoses a numeric literal with invalid syntax.
type ErrInvalidNumber struct {
	Token Token
}

func (e ErrInvalidNumber) Error() string {
	if strings.ContainsRune(e.Token.Text(), '.') {
		return "invalid floating-point literal"
	}

	return "invalid integer literal"
}

func (e ErrInvalidNumber) Diagnose(d *report.Diagnostic) {
	text := strings.ToLower(e.Token.Text())
	d.With(report.Snippet(e.Token))
	if strings.ContainsRune(e.Token.Text(), '.') && strings.HasPrefix(text, "0x") {
		d.With(report.Note("Protobuf does not support binary floats"))
	}
}

// ErrExpectedIdent diagnoses a node that needs to be a single identifier.
type ErrExpectedIdent struct {
	Name, Prior, Named Spanner
}

func (e ErrExpectedIdent) Error() string {
	return fmt.Sprintf("the name of this %s must be a single identifier", describe(e.Named))
}

func (e ErrExpectedIdent) Diagnose(d *report.Diagnostic) {
	d.With(report.Snippet(e.Name))

	if e.Prior != nil {
		d.With(report.Snippetf(e.Prior, "this must be followed by an identifier"))
	}
}

// ErrNoSyntax diagnoses a missing syntax declaration in a file.
type ErrNoSyntax struct {
	Path string
}

func (e ErrNoSyntax) Error() string {
	return "missing syntax declaration"
}

func (e ErrNoSyntax) Diagnose(d *report.Diagnostic) {
	d.With(
		report.InFile(e.Path),
		report.Note("omitting the `syntax` keyword implies \"proto2\" by default"),
		report.Help("explicitly add `syntax = \"proto2\";` at the top of the file"),
	)
}

// ErrUnknownSyntax diagnoses a [DeclSyntax] with an unknown value.
type ErrUnknownSyntax struct {
	Node  DeclSyntax
	Value Token
}

func (e ErrUnknownSyntax) Error() string {
	value, _ := e.Value.AsString()
	if len(value) > 16 {
		value = value[:16] + "..."
	}

	return fmt.Sprintf("%q is not a valid %s", value, e.Node.Keyword().Text())
}

func (e ErrUnknownSyntax) Diagnose(d *report.Diagnostic) {
	d.With(report.Snippet(e.Value))

	var values []string
	switch {
	case e.Node.IsSyntax():
		values = knownSyntaxes
	case e.Node.IsEdition():
		values = knownEditions
	}

	if len(values) > 0 {
		var list strings.Builder
		for i, value := range values {
			if i != 0 {
				list.WriteString(", ")
			}
			list.WriteRune('"')
			list.WriteString(value)
			list.WriteRune('"')
		}

		d.With(report.Notef("protocompile only recognizes %v", list.String()))
	}
}

// ErrNoPackage diagnoses a missing package declaration in a file.
type ErrNoPackage struct {
	Path string
}

func (e ErrNoPackage) Error() string {
	return "missing package declaration"
}

func (e ErrNoPackage) Diagnose(d *report.Diagnostic) {
	d.With(
		report.InFile(e.Path),
		report.Note("omitting the `package` keyword implies an empty package"),
		report.Help("using the empty package is discouraged"),
		report.Help("explicitly add `package ...;` at the top of the file, after the syntax declaration"),
	)
}

// ErrMoreThanOne indicates that some portion of a production appeared more than once,
// even though it should occur at most once.
type ErrMoreThanOne struct {
	First, Second Spanner
	what          string
}

func (e ErrMoreThanOne) Error() string {
	return "encountered more than one " + e.what
}

func (e ErrMoreThanOne) Diagnose(d *report.Diagnostic) {
	d.With(
		report.Snippet(e.Second),
		report.Snippetf(e.First, "first one is here"),
	)
}

// ErrInvalidChild diagnoses a declaration that should not occur inside of another.
type ErrInvalidChild struct {
	Parent, Decl Spanner
}

func (e ErrInvalidChild) Error() string {
	return fmt.Sprintf("unexpected %s inside %s", describe(e.Decl), describe(e.Parent))
}

func (e ErrInvalidChild) Diagnose(d *report.Diagnostic) {
	d.With(
		report.Snippet(e.Decl),
		report.Snippetf(e.Parent, "inside this %s", describe(e.Parent)),
	)

	switch e.Decl.(type) {
	case DeclSyntax, DeclPackage, DeclImport:
		d.With(report.Help("perhaps you meant to place this in the top-level scope?"))
	}
}

// ** PRIVATE ** //

// errUnexpected is a low-level parser error for when we hit a token we don't
// know how to handle.
type errUnexpected struct {
	node  Spanner
	where string
	want  []string
	got   string
}

func (e errUnexpected) Error() string {
	if e.got == "" {
		e.got = describe(e.node)
	}
	if e.where != "" {
		return fmt.Sprintf("unexpected %s %s", e.got, e.where)
	}
	return fmt.Sprintf("unexpected %s", e.got)
}

func (e errUnexpected) Diagnose(d *report.Diagnostic) {
	var buf strings.Builder
	switch len(e.want) {
	case 0:
	case 1:
		fmt.Fprintf(&buf, "expected %s", e.want[0])
	case 2:
		fmt.Fprintf(&buf, "expected %s or %s", e.want[0], e.want[1])
	default:
		buf.WriteString("expected ")
		for _, want := range e.want[:len(e.want)-1] {
			fmt.Fprintf(&buf, "%s, ", want)
		}
		fmt.Fprintf(&buf, "or %s", e.want[len(e.want)-1])
	}

	d.With(report.Snippetf(e.node, "%s", buf.String()))
}

// describe attempts to generate a user-friendly name for `node`.
func describe(node Spanner) string {
	switch node := node.(type) {
	case File:
		return "top-level scope"
	case Path, ExprPath, TypePath:
		return "path"
	case DeclSyntax:
		if node.IsEdition() {
			return "edition declaration"
		}
		return "syntax declaration"
	case DeclPackage:
		return "package declaration"
	case DeclImport:
		switch {
		case node.IsWeak():
			return "weak import"
		case node.IsPublic():
			return "public import"
		default:
			return "import"
		}
	case DeclRange:
		switch {
		case node.IsExtensions():
			return "extension range"
		case node.IsReserved():
			return "reserved range"
		default:
			return "range declaration"
		}
	case DeclDef:
		switch def := node.Classify().(type) {
		case DefMessage:
			return "message definition"
		case DefEnum:
			return "enum definition"
		case DefService:
			return "service definition"
		case DefExtend:
			return "extension declaration"
		case DefOption:
			var first PathComponent
			def.Path.Components(func(pc PathComponent) bool {
				first = pc
				return false
			})
			if first.IsExtension() {
				return "custom option setting"
			} else {
				return "option setting"
			}
		case DefField, DefGroup:
			return "message field"
		case DefEnumValue:
			return "enum value"
		case DefMethod:
			return "service method"
		case DefOneof:
			return "oneof definition"
		default:
			return "invalid definition"
		}
	case DeclEmpty:
		return "empty declaration"
	case DeclScope:
		return "definition body"
	case Decl:
		return "declaration"
	case ExprLiteral:
		return describe(node.Token)
	case ExprPrefixed:
		if lit, ok := node.Expr().(ExprLiteral); ok && lit.Token.Kind() == TokenNumber &&
			node.Prefix() == ExprPrefixMinus {
			return describe(lit.Token)
		}
		return "expression"
	case Expr:
		return "expression"
	case Type:
		return "type"
	case CompactOptions:
		return "compact options"
	case Token:
		switch node.Kind() {
		case TokenSpace:
			return "whitespace"
		case TokenComment:
			return "comment"
		case TokenIdent:
			if name := node.Text(); keywords[name] {
				return fmt.Sprintf("`%s`", name)
			}
			return "identifier"
		case TokenString:
			return "string literal"
		case TokenNumber:
			if strings.ContainsRune(node.Text(), '.') {
				return "floating-point literal"
			}

			return "integer literal"
		case TokenPunct:
			if !node.IsLeaf() {
				start, end := node.StartEnd()
				return fmt.Sprintf("`%s...%s`", start.Text(), end.Text())
			}

			fallthrough
		default:
			return fmt.Sprintf("`%s`", node.Text())
		}
	}

	return ""
}
