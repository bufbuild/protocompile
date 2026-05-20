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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAssembleAndDisassemble(t *testing.T) {
	input := `1: 150
2: {
  1: "hello"
}
`
	binary, diags := Assemble("test.protoscope", []byte(input))
	require.Empty(t, diags)
	require.NotEmpty(t, binary)

	// Verify disassembled output matches input
	text, err := Disassemble(binary, DisassembleOptions{})
	require.NoError(t, err)
	assert.Contains(t, text, "1: 150")
	assert.Contains(t, text, `2: {`+"`"+`0a 05 68 65 6c 6c 6f`+"`"+`}`)

	// Test MaxDepth
	nestedInput := `1: {
  2: {
    3: 150
  }
}
`
	nestedBinary, nestedDiags := Assemble("nested.protoscope", []byte(nestedInput))
	require.Empty(t, nestedDiags)
	require.NotEmpty(t, nestedBinary)

	// Disassembling with default options should work
	_, err = Disassemble(nestedBinary, DisassembleOptions{})
	require.NoError(t, err)

	// Disassembling with MaxDepth = 1 should fail
	_, err = Disassemble(nestedBinary, DisassembleOptions{MaxDepth: 1})
	require.Error(t, err)
	assert.Equal(t, "max depth exceeded", err.Error())
}

func TestDiagnostics(t *testing.T) {
	// Syntactically invalid input
	invalidInput := `1: 
2: {
`
	diags := Diagnostics("invalid.protoscope", []byte(invalidInput))
	assert.NotEmpty(t, diags)

	var hasError bool
	for _, diag := range diags {
		if diag.Level == SeverityError {
			hasError = true
		}
		assert.NotEmpty(t, diag.Message)
		assert.Positive(t, diag.Range.Start.Line)
	}
	assert.True(t, hasError)
}

func TestDocumentSymbols(t *testing.T) {
	input := `1: 150
2: {
  3: "hello"
}
`
	symbols, diags := DocumentSymbols("test.protoscope", []byte(input))
	require.Empty(t, diags)
	require.Len(t, symbols, 2)

	// First symbol: Field 1
	assert.Equal(t, "1:", symbols[0].Name)
	assert.Equal(t, "field", symbols[0].Kind)

	// Second symbol: Field 2 containing Block
	assert.Equal(t, "2:", symbols[1].Name)
	assert.Equal(t, "field", symbols[1].Kind)
	require.Len(t, symbols[1].Children, 1)

	// Block symbol
	block := symbols[1].Children[0]
	assert.Equal(t, "Length-Prefixed", block.Name)
	assert.Equal(t, "block", block.Kind)
	require.Len(t, block.Children, 1)

	// Inside Block: Field 3
	field3 := block.Children[0]
	assert.Equal(t, "3:", field3.Name)
	assert.Equal(t, "field", field3.Kind)
}

func TestInspect(t *testing.T) {
	input := `1: 150
2: {
  3: ` + "`" + `01 02 03` + "`" + `
}
`
	// Test hover over "1:" (line 1, column 1)
	h1, err := Inspect("test.protoscope", []byte(input), 1, 1)
	require.NoError(t, err)
	require.NotNil(t, h1)
	assert.Equal(t, InspectKindField, h1.Kind)
	require.NotNil(t, h1.Field)
	assert.Equal(t, "1", h1.Field.Tag)

	// Test hover over "150" (line 1, column 4)
	h2, err := Inspect("test.protoscope", []byte(input), 1, 4)
	require.NoError(t, err)
	require.NotNil(t, h2)
	assert.Equal(t, InspectKindLiteral, h2.Kind)
	require.NotNil(t, h2.Literal)
	assert.Equal(t, "Number", h2.Literal.Type)
	assert.Equal(t, "150", h2.Literal.RawText)
	assert.True(t, h2.Literal.HasInt)
	assert.Equal(t, uint64(150), h2.Literal.IntValue)

	// Test hover over Hex string literal (line 3, column 6)
	h3, err := Inspect("test.protoscope", []byte(input), 3, 6)
	require.NoError(t, err)
	require.NotNil(t, h3)
	assert.Equal(t, InspectKindLiteral, h3.Kind)
	require.NotNil(t, h3.Literal)
	assert.Equal(t, "String", h3.Literal.Type)
	assert.True(t, h3.Literal.IsHexHexQuote)
	assert.Equal(t, 3, h3.Literal.HexLength)

	// Test hover over Hex string literal with UTF-8
	utf8Input := "1: {`e6 97 a5 e6 9c ac e8 aa 9e`}\n"
	h4, err := Inspect("utf8.protoscope", []byte(utf8Input), 1, 5)
	require.NoError(t, err)
	require.NotNil(t, h4)
	assert.Equal(t, InspectKindLiteral, h4.Kind)
	require.NotNil(t, h4.Literal)
	assert.True(t, h4.Literal.IsHexHexQuote)
	assert.Equal(t, "\u65e5\u672c\u8a9e", h4.Literal.DecodedText)

	// Test hover over standard string literal with multi-byte runes
	stdStringInput := "1: \"Hello, UTF-8 text! \u65e5\u672c\u8a9e, \U00010348, \U0001f4bb, \U0001f680\""
	h5, err := Inspect("std.protoscope", []byte(stdStringInput), 1, 5)
	require.NoError(t, err)
	require.NotNil(t, h5)
	assert.Equal(t, InspectKindLiteral, h5.Kind)
	require.NotNil(t, h5.Literal)
	assert.Equal(t, "String", h5.Literal.Type)
	assert.False(t, h5.Literal.IsHexHexQuote)
	assert.Equal(t, 46, h5.Literal.ByteLength)
	assert.Equal(t, 31, h5.Literal.CharLength)
}

func TestPossibilities(t *testing.T) {
	// wireVarint = 0, payload = [0x96, 0x01] (varint for 150)
	reps := Possibilities(0, []byte{0x96, 0x01})
	require.NotEmpty(t, reps)

	var foundVarint bool
	for _, r := range reps {
		if r.Type == "varint" {
			foundVarint = true
			assert.Equal(t, "150", r.Text)
			assert.Equal(t, "Varint", r.Description)
		}
	}
	assert.True(t, foundVarint, "Should have found varint representation")
}

func TestMultiFrameAndVariants(t *testing.T) {
	// 1. No framing with single frame
	rawInput := `1: 150
2: "hello"
`
	binary, diags := AssembleWithOptions("raw.protoscope", []byte(rawInput), AssembleOptions{Framing: FramingNone})
	require.Empty(t, diags)
	// Output should be concatenated binary:
	// 1: 150 -> 08 96 01
	// 2: "hello" -> 12 05 68 65 6c 6c 6f
	expectedRaw := []byte{0x08, 0x96, 0x01, 0x12, 0x05, 0x68, 0x65, 0x6c, 0x6c, 0x6f}
	assert.Equal(t, expectedRaw, binary)

	// Since raw has no headers, disassemble raw treats the whole stream as 1 message.
	disText, err := Disassemble(binary, DisassembleOptions{Framing: FramingNone})
	require.NoError(t, err)
	assert.Contains(t, disText, "1: 150")
	assert.Contains(t, disText, `2: {"hello"}`) // disassembled as tag 2 since it was concatenated

	// 2. Varint delimited framing with multiple frames
	varintInput := `1: 150
---
2: "hello"
`
	binaryV, diagsV := AssembleWithOptions("varint.protoscope", []byte(varintInput), AssembleOptions{Framing: FramingVarint})
	require.Empty(t, diagsV)
	// Frame 1: len 3, Frame 2: len 7
	expectedV := []byte{
		3, 0x08, 0x96, 0x01,
		7, 0x12, 0x05, 0x68, 0x65, 0x6c, 0x6c, 0x6f,
	}
	assert.Equal(t, expectedV, binaryV)

	disTextV, err := Disassemble(binaryV, DisassembleOptions{Framing: FramingVarint})
	require.NoError(t, err)
	assert.Equal(t, "1: 150\n---\n2: {\"hello\"}\n", disTextV)

	// 3. gRPC / ConnectRPC framing with custom flags
	grpcInput := `# flags: 1
1: 150
---
# flag: 2
2: "hello"
`
	binaryG, diagsG := AssembleWithOptions("grpc.protoscope", []byte(grpcInput), AssembleOptions{Framing: FramingGRPC})
	require.Empty(t, diagsG)
	// Frame 1: flags 1, len 3 -> 01, 00 00 00 03, 08 96 01
	// Frame 2: flags 2, len 7 -> 02, 00 00 00 07, 12 05 68 65 6c 6c 6f
	expectedG := []byte{
		1, 0x00, 0x00, 0x00, 0x03, 0x08, 0x96, 0x01,
		2, 0x00, 0x00, 0x00, 0x07, 0x12, 0x05, 0x68, 0x65, 0x6c, 0x6c, 0x6f,
	}
	assert.Equal(t, expectedG, binaryG)

	disTextG, err := Disassemble(binaryG, DisassembleOptions{Framing: FramingGRPC})
	require.NoError(t, err)
	assert.Equal(t, "# flags: 1\n1: 150\n---\n# flags: 2\n2: {\"hello\"}\n", disTextG)

	// 4. Test diagnostics shifting across multiple frames
	invalidInput := `1: 150
---
2: {
# syntax error in second frame
`
	diagsErr := Diagnostics("test.protoscope", []byte(invalidInput))
	require.NotEmpty(t, diagsErr)
	// The error should be in the second frame (after line 2)
	assert.Greater(t, diagsErr[0].Range.Start.Line, 2)

	// 5. Test DocumentSymbols and Hover on multi-frame inputs
	symbolInput := `1: 150
---
2: 30
`
	symbols, diagsSym := DocumentSymbols("symbols.protoscope", []byte(symbolInput))
	require.Empty(t, diagsSym)
	require.Len(t, symbols, 2)
	// Symbol 1 starts on line 1
	assert.Equal(t, 1, symbols[0].Range.Start.Line)
	// Symbol 2 starts on line 3 (after ---)
	assert.Equal(t, 3, symbols[1].Range.Start.Line)

	// Hover test
	hover1, err := Inspect("symbols.protoscope", []byte(symbolInput), 1, 1)
	require.NoError(t, err)
	require.NotNil(t, hover1)
	assert.Equal(t, 1, hover1.Range.Start.Line)

	hover2, err := Inspect("symbols.protoscope", []byte(symbolInput), 3, 1)
	require.NoError(t, err)
	require.NotNil(t, hover2)
	assert.Equal(t, 3, hover2.Range.Start.Line)

	// 6. Test multi-frame raw framing error (no framing)
	multiRawInput := "1: 150\n---\n2: \"hello\"\n"
	_, rawDiags := AssembleWithOptions("raw.protoscope", []byte(multiRawInput), AssembleOptions{Framing: FramingNone})
	require.NotEmpty(t, rawDiags)
	assert.Equal(t, "multiple frames are not supported for no framing", rawDiags[0].Message)
	assert.Equal(t, 2, rawDiags[0].Range.Start.Line)
}

func TestAllVariantsRoundtrip(t *testing.T) {
	t.Parallel()

	input := `1: 150
---
2: {"hello"}
`

	framings := []Framing{
		FramingGRPC,
		FramingConnect,
		FramingVarint,
	}

	for _, framing := range framings {
		t.Run(framing.String(), func(t *testing.T) {
			t.Parallel()
			// Assemble
			binary, diags := AssembleWithOptions("test.protoscope", []byte(input), AssembleOptions{Framing: framing})
			require.Empty(t, diags)
			require.NotEmpty(t, binary)

			// Disassemble
			disassembled, err := Disassemble(binary, DisassembleOptions{Framing: framing})
			require.NoError(t, err)

			// The output should contain our fields and be properly split by ---
			assert.Contains(t, disassembled, "1: 150")
			assert.Contains(t, disassembled, "---")
			assert.Contains(t, disassembled, `2: {"hello"}`)
		})
	}
}

func TestDisassembleFallback(t *testing.T) {
	t.Parallel()

	// gRPC message `1: 55` has bytes:
	// 00 00 00 00 02 08 37
	grpcBytes := []byte{0x00, 0x00, 0x00, 0x00, 0x02, 0x08, 0x37}

	// Disassembling without framing should trigger invalid tag 0 error fallback comment
	disassembled, err := Disassemble(grpcBytes, DisassembleOptions{})
	require.NoError(t, err)
	assert.Contains(t, disassembled, "# Error: invalid tag 0; this might be using a different framing (e.g. gRPC)")
	assert.Contains(t, disassembled, "`00 00 00 00 02 08 37`")
}

func TestParseFraming(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected Framing
		hasErr   bool
	}{
		{input: "none", expected: FramingNone},
		{input: "raw", expected: FramingNone},
		{input: "", expected: FramingNone},
		{input: "grpc", expected: FramingGRPC},
		{input: "GRPC", expected: FramingGRPC},
		{input: "connect", expected: FramingConnect},
		{input: "connectrpc", expected: FramingConnect},
		{input: "Connect-RPC", expected: FramingConnect},
		{input: "varint", expected: FramingVarint},
		{input: "varintdelimited", expected: FramingVarint},
		{input: "varint-delimited", expected: FramingVarint},
		{input: "varint delimited", expected: FramingVarint},
		{input: "invalid", hasErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			actual, err := ParseFraming(tc.input)
			if tc.hasErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expected, actual)
			}
		})
	}
}

func TestFramingString(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "none", FramingNone.String())
	assert.Equal(t, "grpc", FramingGRPC.String())
	assert.Equal(t, "connect", FramingConnect.String())
	assert.Equal(t, "varint", FramingVarint.String())
	assert.Equal(t, "unknown(-1)", Framing(-1).String())
}

func TestFramingDeepValidation(t *testing.T) {
	t.Parallel()

	t.Run("exact bytes mapping", func(t *testing.T) {
		t.Parallel()
		input := "1: 150"

		// 1. None (raw wire format)
		binNone, diags := AssembleWithOptions("test.protoscope", []byte(input), AssembleOptions{Framing: FramingNone})
		require.Empty(t, diags)
		assert.Equal(t, []byte{0x08, 0x96, 0x01}, binNone)

		// 2. gRPC
		binGRPC, diags := AssembleWithOptions("test.protoscope", []byte(input), AssembleOptions{Framing: FramingGRPC})
		require.Empty(t, diags)
		assert.Equal(t, []byte{0x00, 0x00, 0x00, 0x00, 0x03, 0x08, 0x96, 0x01}, binGRPC)

		// 3. Connect
		binConnect, diags := AssembleWithOptions("test.protoscope", []byte(input), AssembleOptions{Framing: FramingConnect})
		require.Empty(t, diags)
		assert.Equal(t, []byte{0x00, 0x00, 0x00, 0x00, 0x03, 0x08, 0x96, 0x01}, binConnect)

		// 4. Varint delimited
		binVarint, diags := AssembleWithOptions("test.protoscope", []byte(input), AssembleOptions{Framing: FramingVarint})
		require.Empty(t, diags)
		assert.Equal(t, []byte{0x03, 0x08, 0x96, 0x01}, binVarint)
	})

	t.Run("flags parsing and roundtrip", func(t *testing.T) {
		t.Parallel()
		input := "# flags: 5\n1: 150"

		bin, diags := AssembleWithOptions("test.protoscope", []byte(input), AssembleOptions{Framing: FramingGRPC})
		require.Empty(t, diags)
		// gRPC header should have flag byte = 5
		assert.Equal(t, []byte{0x05, 0x00, 0x00, 0x00, 0x03, 0x08, 0x96, 0x01}, bin)

		text, err := Disassemble(bin, DisassembleOptions{Framing: FramingGRPC})
		require.NoError(t, err)
		assert.Contains(t, text, "# flags: 5")
		assert.Contains(t, text, "1: 150")
	})

	t.Run("error handling on truncated/corrupted inputs", func(t *testing.T) {
		t.Parallel()

		// 1. Truncated gRPC header
		_, err := Disassemble([]byte{0x00, 0x00}, DisassembleOptions{Framing: FramingGRPC})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unexpected EOF reading header")

		// 2. Truncated gRPC payload
		_, err = Disassemble([]byte{0x00, 0x00, 0x00, 0x00, 0x05, 0x08, 0x96}, DisassembleOptions{Framing: FramingGRPC})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "length 5 out of bounds")

		// 3. Truncated/invalid Varint header
		_, err = Disassemble([]byte{0x80}, DisassembleOptions{Framing: FramingVarint})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid varint length prefix")

		// 4. Truncated Varint payload
		_, err = Disassemble([]byte{0x05, 0x08, 0x96}, DisassembleOptions{Framing: FramingVarint})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "length 5 out of bounds")
	})
}
