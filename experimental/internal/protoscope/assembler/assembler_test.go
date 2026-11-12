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

	"github.com/bufbuild/protocompile/experimental/internal/protoscope/parser"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/source"
)

func TestAssemble(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []byte
	}{
		{
			name:     "simple varint",
			input:    "1: 150",
			expected: []byte{0x08, 0x96, 0x01},
		},
		{
			name:     "boolean true",
			input:    "1: true",
			expected: []byte{0x08, 0x01},
		},
		{
			name:     "string literal",
			input:    `1: "testing"`,
			expected: []byte{0x0a, 0x07, 't', 'e', 's', 't', 'i', 'n', 'g'},
		},
		{
			name:     "nested block",
			input:    "4: [ 1: 42 ]",
			expected: []byte{0x22, 0x02, 0x08, 0x2a},
		},
		{
			name:     "nested group",
			input:    "5: !{ 1: 42 }",
			expected: []byte{0x2b, 0x08, 0x2a, 0x2c},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := source.NewFile("test.protoscope", tt.input)
			r := &report.Report{}
			file, ok := parser.Parse("test.protoscope", src, r)
			if !ok {
				t.Fatalf("failed to parse: %v", r.Diagnostics)
			}

			got := Assemble(file)
			if !bytes.Equal(got, tt.expected) {
				t.Errorf("Assemble() = %x, want %x", got, tt.expected)
			}
		})
	}
}
