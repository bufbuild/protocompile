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

package protoscope

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/bufbuild/protocompile/experimental/internal/protoscope/assembler"
	"github.com/bufbuild/protocompile/experimental/internal/protoscope/ast"
	"github.com/bufbuild/protocompile/experimental/internal/protoscope/disassembler"
	"github.com/bufbuild/protocompile/experimental/internal/protoscope/parser"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/experimental/source/length"
	"github.com/bufbuild/protocompile/experimental/token"
)

// Severity represents diagnostic severity levels.
type Severity int

const (
	SeverityInfo Severity = iota
	SeverityWarning
	SeverityError
)

// Position represents a 1-indexed line and column position.
type Position struct {
	Line, Column int
}

// Range represents a span between two positions.
type Range struct {
	Start, End Position
}

// Diagnostic represents a syntax or validation diagnostic.
type Diagnostic struct {
	Range   Range
	Message string
	Level   Severity
}

// Framing represents the message framing format.
type Framing int

const (
	// FramingNone represents no framing.
	FramingNone Framing = iota
	// FramingGRPC represents gRPC framing format.
	FramingGRPC
	// FramingConnect represents ConnectRPC framing format.
	FramingConnect
	// FramingVarint represents Varint delimited framing format.
	FramingVarint
)

// String returns the string representation of the framing.
func (f Framing) String() string {
	switch f {
	case FramingNone:
		return "none"
	case FramingGRPC:
		return "grpc"
	case FramingConnect:
		return "connect"
	case FramingVarint:
		return "varint"
	default:
		return fmt.Sprintf("unknown(%d)", f)
	}
}

// ParseFraming parses a framing string to its enum value.
func ParseFraming(s string) (Framing, error) {
	switch strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(s, " ", ""), "-", "")) {
	case "none", "raw", "":
		return FramingNone, nil
	case "grpc":
		return FramingGRPC, nil
	case "connect", "connectrpc":
		return FramingConnect, nil
	case "varint", "varintdelimited":
		return FramingVarint, nil
	default:
		return FramingNone, fmt.Errorf("unknown framing: %q", s)
	}
}

// DisassembleOptions matches the internal disassembler options.
type DisassembleOptions struct {
	ExplicitWireTypes      bool
	ExplicitLengthPrefixes bool
	NoGroups               bool
	MaxDepth               int
	Framing                Framing
}

// AssembleOptions contains assembly options.
type AssembleOptions struct {
	Framing Framing
}

// Assemble parses and compiles protoscope text directly to protobuf wire binary.
func Assemble(path string, text []byte) ([]byte, []Diagnostic) {
	return AssembleWithOptions(path, text, AssembleOptions{})
}

// AssembleWithOptions compiles protoscope text directly to protobuf wire binary with options.
func AssembleWithOptions(path string, text []byte, opts AssembleOptions) ([]byte, []Diagnostic) {
	frames := splitFrames(text)
	parentFile := source.NewFile(path, string(text))
	allDiags := make([]Diagnostic, 0, len(frames))
	var payloads [][]byte
	var flags []byte

	if len(frames) > 1 && opts.Framing == FramingNone {
		line := frames[1].lineOffset
		allDiags = append(allDiags, Diagnostic{
			Range: Range{
				Start: Position{Line: line, Column: 1},
				End:   Position{Line: line, Column: 4},
			},
			Message: "multiple frames are not supported for no framing",
			Level:   SeverityError,
		})
		return nil, allDiags
	}

	hasError := false
	for _, frame := range frames {
		// Extract flags comment from this frame if present.
		var frameFlags byte
		frameLines := strings.Split(frame.text, "\n")
		for _, line := range frameLines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}
			if strings.HasPrefix(trimmed, "#") {
				comment := strings.TrimSpace(strings.TrimPrefix(trimmed, "#"))
				if strings.HasPrefix(comment, "flags:") {
					valStr := strings.TrimSpace(strings.TrimPrefix(comment, "flags:"))
					if val, err := strconv.ParseUint(valStr, 10, 8); err == nil {
						frameFlags = byte(val)
					}
				} else if strings.HasPrefix(comment, "flag:") {
					valStr := strings.TrimSpace(strings.TrimPrefix(comment, "flag:"))
					if val, err := strconv.ParseUint(valStr, 10, 8); err == nil {
						frameFlags = byte(val)
					}
				}
			} else {
				break
			}
		}
		flags = append(flags, frameFlags)

		src := source.NewFile(path, frame.text)
		r := &report.Report{}
		file, ok := parser.Parse(path, src, r)

		report.ShiftReportSpans(r, parentFile, frame.byteOffset)

		diags := convertDiagnostics(r)
		allDiags = append(allDiags, diags...)
		if !ok || file == nil {
			hasError = true
			continue
		}

		out := assembler.Assemble(file)
		payloads = append(payloads, out)
	}

	if hasError {
		return nil, allDiags
	}

	// Apply the framing
	var result []byte
	switch opts.Framing {
	case FramingGRPC, FramingConnect:
		for i, payload := range payloads {
			header := make([]byte, 5)
			header[0] = flags[i]
			binary.BigEndian.PutUint32(header[1:5], uint32(len(payload)))
			result = append(result, header...)
			result = append(result, payload...)
		}
	case FramingVarint:
		for _, payload := range payloads {
			var lengthBuf [10]byte
			n := binary.PutUvarint(lengthBuf[:], uint64(len(payload)))
			result = append(result, lengthBuf[:n]...)
			result = append(result, payload...)
		}
	default:
		// raw / default: concatenate all payloads
		for _, payload := range payloads {
			result = append(result, payload...)
		}
	}

	return result, allDiags
}

// Disassemble converts protobuf wire binary back to protoscope text.
func Disassemble(data []byte, opts DisassembleOptions) (string, error) {
	var buf strings.Builder
	disOpts := disassembler.Options{
		ExplicitWireTypes:      opts.ExplicitWireTypes,
		ExplicitLengthPrefixes: opts.ExplicitLengthPrefixes,
		NoGroups:               opts.NoGroups,
		MaxDepth:               opts.MaxDepth,
	}

	switch opts.Framing {
	case FramingGRPC, FramingConnect:
		off := 0
		first := true
		for off < len(data) {
			if off+5 > len(data) {
				return "", fmt.Errorf("unexpected EOF reading header at offset %d", off)
			}
			flags := data[off]
			length := binary.BigEndian.Uint32(data[off+1 : off+5])
			off += 5

			if uint64(length) > uint64(len(data)-off) {
				return "", fmt.Errorf("length %d out of bounds at offset %d", length, off)
			}

			payload := data[off : off+int(length)]
			off += int(length)

			if !first {
				buf.WriteString("---\n")
			}
			first = false

			if flags != 0 {
				fmt.Fprintf(&buf, "# flags: %d\n", flags)
			}

			err := disassembler.DisassembleWithOptions(payload, &buf, disOpts)
			if err != nil {
				return "", err
			}
		}
		return buf.String(), nil

	case FramingVarint:
		off := 0
		first := true
		for off < len(data) {
			l, n := binary.Uvarint(data[off:])
			if n <= 0 {
				return "", fmt.Errorf("invalid varint length prefix at offset %d", off)
			}
			off += n

			if l > uint64(len(data)-off) {
				return "", fmt.Errorf("length %d out of bounds at offset %d", l, off)
			}

			payload := data[off : off+int(l)]
			off += int(l)

			if !first {
				buf.WriteString("---\n")
			}
			first = false

			err := disassembler.DisassembleWithOptions(payload, &buf, disOpts)
			if err != nil {
				return "", err
			}
		}
		return buf.String(), nil

	default:
		err := disassembler.DisassembleWithOptions(data, &buf, disOpts)
		if err != nil {
			return "", err
		}
		return buf.String(), nil
	}
}

// Diagnostics parses the text and returns any syntactic or structural diagnostics.
func Diagnostics(path string, text []byte) []Diagnostic {
	frames := splitFrames(text)
	parentFile := source.NewFile(path, string(text))
	allDiags := make([]Diagnostic, 0, len(frames))
	for _, frame := range frames {
		src := source.NewFile(path, frame.text)
		r := &report.Report{}
		_, _ = parser.Parse(path, src, r)
		report.ShiftReportSpans(r, parentFile, frame.byteOffset)
		allDiags = append(allDiags, convertDiagnostics(r)...)
	}
	return allDiags
}

// DocumentSymbol represents a simplified symbol hierarchy (e.g. fields, groups, blocks).
type DocumentSymbol struct {
	Name     string
	Detail   string
	Kind     string // e.g., "field", "group", "block", "literal"
	Range    Range
	Children []DocumentSymbol
}

// DocumentSymbols returns a hierarchy of symbols within the protoscope file.
func DocumentSymbols(path string, text []byte) ([]DocumentSymbol, []Diagnostic) {
	frames := splitFrames(text)
	parentFile := source.NewFile(path, string(text))
	allSymbols := make([]DocumentSymbol, 0, len(frames))
	allDiags := make([]Diagnostic, 0, len(frames))

	for _, frame := range frames {
		src := source.NewFile(path, frame.text)
		r := &report.Report{}
		file, ok := parser.Parse(path, src, r)
		report.ShiftReportSpans(r, parentFile, frame.byteOffset)
		diags := convertDiagnostics(r)
		allDiags = append(allDiags, diags...)

		if ok && file != nil {
			var symbols []DocumentSymbol
			for decl := range seq.Values(file.Decls()) {
				symbols = append(symbols, collectSymbols(decl)...)
			}
			if frame.lineOffset > 0 {
				for i := range symbols {
					shiftSymbolRange(&symbols[i], frame.lineOffset)
				}
			}
			allSymbols = append(allSymbols, symbols...)
		}
	}
	return allSymbols, allDiags
}

type InspectKind int

const (
	InspectKindField InspectKind = iota + 1
	InspectKindLiteral
	InspectKindBlock
)

type FieldInspectInfo struct {
	Tag      string
	WireType string
}

type LiteralInspectInfo struct {
	RawText       string
	Type          string // "Number" or "String"
	Suffix        string
	VarintBytes   string
	DecodedText   string
	IntValue      uint64
	FloatValue    float64
	Zigzag        uint64
	HexLength     int
	ByteLength    int
	CharLength    int
	HasInt        bool
	HasFloat      bool
	IsHexHexQuote bool // true if backtick quote `...`
}

type BlockInspectInfo struct {
	Name string // "{" or "!{"
}

// InspectInfo holds structured information to display on inspect (hover).
type InspectInfo struct {
	Range   Range
	Kind    InspectKind
	Field   *FieldInspectInfo
	Literal *LiteralInspectInfo
	Block   *BlockInspectInfo
}

// Inspect returns structured documentation for the token/node at the given line/column.
func Inspect(path string, text []byte, line, col int) (*InspectInfo, error) {
	frames := splitFrames(text)

	var targetFrame *frameInfo
	for i := len(frames) - 1; i >= 0; i-- {
		if line > frames[i].lineOffset {
			targetFrame = &frames[i]
			break
		}
	}
	if targetFrame == nil {
		return nil, nil
	}

	localLine := line - targetFrame.lineOffset
	src := source.NewFile(path, targetFrame.text)
	r := &report.Report{}
	file, _ := parser.Parse(path, src, r)
	if file == nil {
		return nil, nil
	}

	loc := src.InverseLocation(localLine, col, length.UTF16)
	offset := loc.Offset

	node := findNode(file, offset)
	if node.IsZero() {
		return nil, nil
	}

	inspectRange := convertSpan(node.Span())
	inspectRange.Start.Line += targetFrame.lineOffset
	inspectRange.End.Line += targetFrame.lineOffset

	inspect := &InspectInfo{
		Range: inspectRange,
	}

	switch node.Kind() {
	case ast.DeclKindField:
		f := node.AsField()
		inspect.Kind = InspectKindField
		inspect.Field = &FieldInspectInfo{
			Tag: f.Tag().Text(),
		}
		if wt := f.WireType(); !wt.IsZero() && wt.Text() != "" {
			inspect.Field.WireType = wt.Text()
		}

	case ast.DeclKindLiteral:
		l := node.AsLiteral()
		tok := l.Token()
		inspect.Kind = InspectKindLiteral
		inspect.Literal = &LiteralInspectInfo{
			RawText: tok.Text(),
		}

		if tok.Kind() == token.Number {
			inspect.Literal.Type = "Number"
			num := tok.AsNumber()
			inspect.Literal.Suffix = num.Suffix().Text()
			if v, exact := num.Int(); exact {
				inspect.Literal.HasInt = true
				inspect.Literal.IntValue = v
				inspect.Literal.VarintBytes = varintBytes(v)

				// Interpret as signed 64-bit to show zigzag encoding if applicable
				sval := int64(v)
				inspect.Literal.Zigzag = uint64((sval << 1) ^ (sval >> 63))
			} else if fval, exactf := num.Float(); exactf {
				inspect.Literal.HasFloat = true
				inspect.Literal.FloatValue = fval
			}
		} else if tok.Kind() == token.String {
			inspect.Literal.Type = "String"
			sToken := tok.AsString()
			open, _ := sToken.Quotes()
			if open.Text() == "`" {
				inspect.Literal.IsHexHexQuote = true
				// Hex string literal
				decoded, err := hexDecode(tok.Text())
				if err == nil {
					inspect.Literal.HexLength = len(decoded)
					if isPrintable(decoded) {
						inspect.Literal.DecodedText = string(decoded)
					}
				}
			} else {
				// Standard string literal
				strVal := sToken.Text()
				inspect.Literal.ByteLength = len(strVal)
				inspect.Literal.CharLength = utf8.RuneCountInString(strVal)
			}
		}

	case ast.DeclKindBlock:
		b := node.AsBlock()
		inspect.Kind = InspectKindBlock
		inspect.Block = &BlockInspectInfo{
			Name: b.Token().Text(),
		}
	}

	return inspect, nil
}

func collectSymbols(decl ast.DeclAny) []DocumentSymbol {
	if decl.IsZero() {
		return nil
	}
	switch decl.Kind() {
	case ast.DeclKindField:
		f := decl.AsField()
		tagText := f.Tag().Text()

		var children []DocumentSymbol
		val := f.Value()
		if !val.IsZero() {
			children = collectSymbols(val)
		}

		detail := ""
		if wt := f.WireType(); !wt.IsZero() && wt.Text() != "" {
			detail = ":" + wt.Text()
		}

		return []DocumentSymbol{{
			Name:     tagText + ":",
			Detail:   detail,
			Kind:     "field",
			Range:    convertSpan(f.Span()),
			Children: children,
		}}

	case ast.DeclKindLiteral:
		l := decl.AsLiteral()
		return []DocumentSymbol{{
			Name:  l.Token().Text(),
			Kind:  "literal",
			Range: convertSpan(l.Span()),
		}}

	case ast.DeclKindBlock:
		b := decl.AsBlock()
		var children []DocumentSymbol
		for child := range seq.Values(b.Decls()) {
			children = append(children, collectSymbols(child)...)
		}

		name := b.Token().Text()
		detail := ""
		switch name {
		case "!{":
			name = "Group"
			detail = "!{}"
		case "{":
			name = "Length-Prefixed"
			detail = "{}"
		}

		return []DocumentSymbol{{
			Name:     name,
			Detail:   detail,
			Kind:     "block",
			Range:    convertSpan(b.Span()),
			Children: children,
		}}
	}
	return nil
}

func convertSpan(span source.Span) Range {
	if span.IsZero() {
		return Range{}
	}
	startLoc := span.StartLoc()
	endLoc := span.EndLoc()
	return Range{
		Start: Position{Line: startLoc.Line, Column: startLoc.Column},
		End:   Position{Line: endLoc.Line, Column: endLoc.Column},
	}
}

func varintBytes(v uint64) string {
	var buf []string
	for v >= 0x80 {
		buf = append(buf, fmt.Sprintf("%02X", byte(v|0x80)))
		v >>= 7
	}
	buf = append(buf, fmt.Sprintf("%02X", byte(v)))
	return strings.Join(buf, " ")
}

func isPrintable(data []byte) bool {
	if !utf8.Valid(data) {
		return false
	}
	for len(data) > 0 {
		r, size := utf8.DecodeRune(data)
		if r == utf8.RuneError {
			return false
		}
		if !unicode.IsPrint(r) && !unicode.IsSpace(r) {
			return false
		}
		data = data[size:]
	}
	return true
}

func hexDecode(text string) ([]byte, error) {
	// Strip backticks
	text = strings.Trim(text, "`")
	// Remove whitespace
	var cleaned strings.Builder
	for _, r := range text {
		if !unicode.IsSpace(r) {
			cleaned.WriteRune(r)
		}
	}
	return hex.DecodeString(cleaned.String())
}

func findNode(file *ast.File, offset int) ast.DeclAny {
	var best ast.DeclAny
	var search func(decl ast.DeclAny)
	search = func(decl ast.DeclAny) {
		if decl.IsZero() {
			return
		}
		span := decl.Span()
		if span.IsZero() {
			return
		}
		if offset >= span.Start && offset <= span.End {
			best = decl
			if decl.Kind() == ast.DeclKindBlock {
				for child := range seq.Values(decl.AsBlock().Decls()) {
					search(child)
				}
			} else if decl.Kind() == ast.DeclKindField {
				search(decl.AsField().Value())
			}
		}
	}
	for decl := range seq.Values(file.Decls()) {
		search(decl)
	}
	return best
}

func convertDiagnostics(r *report.Report) []Diagnostic {
	diagnostics := make([]Diagnostic, 0, len(r.Diagnostics))
	for _, diag := range r.Diagnostics {
		severity := SeverityError
		switch diag.Level() {
		case report.Warning:
			severity = SeverityWarning
		case report.Remark:
			severity = SeverityInfo
		}

		span := diag.Primary()
		var rangeVal Range
		if !span.IsZero() {
			startLoc := span.StartLoc()
			endLoc := span.EndLoc()
			rangeVal = Range{
				Start: Position{Line: startLoc.Line, Column: startLoc.Column},
				End:   Position{Line: endLoc.Line, Column: endLoc.Column},
			}
		}

		diagnostics = append(diagnostics, Diagnostic{
			Range:   rangeVal,
			Message: diag.Message(),
			Level:   severity,
		})
	}
	return diagnostics
}

// Representation represents a possible translation/formatting of a protobuf value.
type Representation struct {
	Type        string  // E.g., "message", "string", "bytes", "varint", "zigzag", "bool", "fixed32", "float32", "fixed64", "float64", "packed_varint", "packed_fixed32", "packed_fixed64"
	Text        string  // The protoscope textual value representation
	Description string  // Human-readable description
	Likelihood  float64 // Likelihood score, between 0.0 and 1.0 (higher is more likely)
}

func mapRepresentations(internalReps []disassembler.Representation) []Representation {
	if internalReps == nil {
		return nil
	}
	reps := make([]Representation, len(internalReps))
	for i, r := range internalReps {
		reps[i] = Representation{
			Type:        r.Type,
			Text:        r.Text,
			Description: r.Description,
			Likelihood:  r.Likelihood,
		}
	}
	return reps
}

// Possibilities analyzes the raw payload bytes for a given wire type and returns
// all valid alternative representations sorted by likelihood.
func Possibilities(wireType int, payload []byte) []Representation {
	return mapRepresentations(disassembler.Possibilities(wireType, payload))
}

type frameInfo struct {
	text       string
	byteOffset int
	lineOffset int
}

func splitFrames(text []byte) []frameInfo {
	var frames []frameInfo
	s := string(text)
	lines := strings.Split(s, "\n")
	var currentFrame strings.Builder
	frameLineOffset := 0
	frameByteOffset := 0
	currentByteOffset := 0

	for i, line := range lines {
		lineLen := len(line)
		if i < len(lines)-1 {
			lineLen++ // add 1 for '\n'
		}

		trimmed := strings.TrimSpace(line)
		if trimmed == "---" {
			frames = append(frames, frameInfo{
				text:       currentFrame.String(),
				byteOffset: frameByteOffset,
				lineOffset: frameLineOffset,
			})
			currentFrame.Reset()
			frameLineOffset = i + 1
			frameByteOffset = currentByteOffset + lineLen
		} else {
			currentFrame.WriteString(line)
			if i < len(lines)-1 {
				currentFrame.WriteByte('\n')
			}
		}
		currentByteOffset += lineLen
	}
	frames = append(frames, frameInfo{
		text:       currentFrame.String(),
		byteOffset: frameByteOffset,
		lineOffset: frameLineOffset,
	})
	return frames
}

func shiftSymbolRange(s *DocumentSymbol, lineOffset int) {
	s.Range.Start.Line += lineOffset
	s.Range.End.Line += lineOffset
	for i := range s.Children {
		shiftSymbolRange(&s.Children[i], lineOffset)
	}
}
