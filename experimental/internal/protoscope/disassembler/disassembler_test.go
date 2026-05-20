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
	"bytes"
	"testing"
)

func TestMaxDepth(t *testing.T) {
	// Nested groups: 0x0b = SGROUP tag 1, 0x0c = EGROUP tag 1
	// 3 levels of nesting
	data := []byte{0x0b, 0x0b, 0x0b, 0x0c, 0x0c, 0x0c}

	var buf bytes.Buffer
	// With MaxDepth: 2, should exceed the limit and error
	err := DisassembleWithOptions(data, &buf, Options{MaxDepth: 2})
	if err == nil {
		t.Error("expected error with MaxDepth: 2, got nil")
	} else if err.Error() != "max depth exceeded" {
		t.Errorf("expected 'max depth exceeded', got %v", err)
	}

	// With MaxDepth: 1, should also exceed and error
	err = DisassembleWithOptions(data, &buf, Options{MaxDepth: 1})
	if err == nil {
		t.Error("expected error with MaxDepth: 1, got nil")
	} else if err.Error() != "max depth exceeded" {
		t.Errorf("expected 'max depth exceeded', got %v", err)
	}
}
