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

package disassembler

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func FuzzDisassemble(f *testing.F) {
	// Add baseline corpus with option permutations
	baselineSeeds := [][]byte{
		{0x08, 0x96, 0x01},             // simple varint
		{0x0a, 0x03, 0x01, 0x02, 0x03}, // packed
		{0x0b, 0x10, 0x03, 0x0c},       // group
	}
	for _, seed := range baselineSeeds {
		for _, explicitWireTypes := range []bool{false, true} {
			for _, explicitLengthPrefixes := range []bool{false, true} {
				for _, noGroups := range []bool{false, true} {
					f.Add(seed, explicitWireTypes, explicitLengthPrefixes, noGroups)
				}
			}
		}
	}

	// Dynamically find and load complex .pb files from testdata directory
	files, err := filepath.Glob("../testdata/*.pb")
	if err == nil {
		for _, file := range files {
			data, err := os.ReadFile(file)
			if err != nil {
				continue
			}
			for _, explicitWireTypes := range []bool{false, true} {
				for _, explicitLengthPrefixes := range []bool{false, true} {
					for _, noGroups := range []bool{false, true} {
						f.Add(data, explicitWireTypes, explicitLengthPrefixes, noGroups)
					}
				}
			}
		}
	}

	f.Fuzz(func(_ *testing.T, data []byte, explicitWireTypes, explicitLengthPrefixes, noGroups bool) {
		opts := Options{
			ExplicitWireTypes:      explicitWireTypes,
			ExplicitLengthPrefixes: explicitLengthPrefixes,
			NoGroups:               noGroups,
		}
		_ = DisassembleWithOptions(data, io.Discard, opts)
	})
}
