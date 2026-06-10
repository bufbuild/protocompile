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

package protoscope_test

import (
	"testing"

	"github.com/bufbuild/protocompile/experimental/protoscope"
)

func FuzzAssemble(f *testing.F) {
	f.Add("1: 150", 0)
	f.Add("1: 150\n---\n2: \"hello\"", 3)
	f.Add("# flags: 5\n1: 150\n", 1)

	f.Fuzz(func(_ *testing.T, text string, framingVal int) {
		framings := []protoscope.Framing{
			protoscope.FramingNone,
			protoscope.FramingGRPC,
			protoscope.FramingConnect,
			protoscope.FramingVarint,
		}
		framing := framings[uint(framingVal)%uint(len(framings))]

		// Assemble
		binary, diags := protoscope.AssembleWithOptions("fuzz.protoscope", []byte(text), protoscope.AssembleOptions{Framing: framing})
		if len(diags) > 0 || len(binary) == 0 {
			return
		}

		// Round-trip disassemble
		_, _ = protoscope.Disassemble(binary, protoscope.DisassembleOptions{Framing: framing})
	})
}

func FuzzDisassemble(f *testing.F) {
	// Add some typical payloads
	f.Add([]byte{0x08, 0x96, 0x01}, 0)
	f.Add([]byte{0x00, 0x00, 0x00, 0x00, 0x03, 0x08, 0x96, 0x01}, 1)
	f.Add([]byte{0x03, 0x08, 0x96, 0x01}, 3)

	f.Fuzz(func(_ *testing.T, binary []byte, framingVal int) {
		framings := []protoscope.Framing{
			protoscope.FramingNone,
			protoscope.FramingGRPC,
			protoscope.FramingConnect,
			protoscope.FramingVarint,
		}
		framing := framings[uint(framingVal)%uint(len(framings))]

		// Disassemble
		text, err := protoscope.Disassemble(binary, protoscope.DisassembleOptions{
			Framing: framing,
		})
		if err != nil || len(text) == 0 {
			return
		}

		// Round-trip assemble
		_, _ = protoscope.AssembleWithOptions("fuzz.protoscope", []byte(text), protoscope.AssembleOptions{Framing: framing})
	})
}
