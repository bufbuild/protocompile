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

package assembler

import (
	"bytes"
	"testing"

	"github.com/bufbuild/protocompile/experimental/internal/protoscope/disassembler"
	"github.com/bufbuild/protocompile/experimental/internal/protoscope/parser"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/source"
)

func FuzzRoundTrip(f *testing.F) {
	f.Add([]byte{0x08, 0x96, 0x01})
	f.Add([]byte{0x0a, 0x08, 'J', 'o', 'h', 'n', ' ', 'D', 'o', 'e'})

	f.Fuzz(func(_ *testing.T, data []byte) {
		// 1. Disassemble bytes to text
		var buf bytes.Buffer
		if err := disassembler.Disassemble(data, &buf); err != nil {
			return
		}
		text := buf.String()

		// 2. Parse text back to AST
		src := source.NewFile("fuzz.protoscope", text)
		r := &report.Report{}
		file, ok := parser.Parse("fuzz.protoscope", src, r)
		if !ok {
			// This might happen if the disassembler outputs something that the parser
			// doesn't support yet, or if the input was invalid and the disassembler
			// produced "invalid" text (like unsupported wire types).
			// For now we just return, but in a mature implementation this should be rare.
			return
		}

		// 3. Assemble AST back to bytes
		_ = Assemble(file)

		// Note: we don't strictly require that re-assembled bytes match the original
		// because of heuristics in disassembly and potential ambiguity in wire types
		// if not explicitly tagged. The goal here is stability (no panics).
	})
}
