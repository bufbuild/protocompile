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
	"reflect"
	"testing"
)

func TestPossibilitiesVarint(t *testing.T) {
	// Value 150 (varint encoded is 0x96, 0x01)
	payload := []byte{0x96, 0x01}
	res := Possibilities(wireVarint, payload)

	expected := []Representation{
		{Type: "varint", Text: "150", Description: "Varint", Likelihood: 0.9},
		{Type: "zigzag", Text: "75", Description: "Zigzag Varint", Likelihood: 0.7},
	}
	if !reflect.DeepEqual(res, expected) {
		t.Errorf("expected %#v, got %#v", expected, res)
	}

	// Value 1 (varint encoded is 0x01)
	payload = []byte{0x01}
	res = Possibilities(wireVarint, payload)
	expected = []Representation{
		{Type: "varint", Text: "1", Description: "Varint", Likelihood: 0.9},
		{Type: "bool", Text: "true", Description: "Boolean", Likelihood: 0.8},
		{Type: "zigzag", Text: "-1", Description: "Zigzag Varint", Likelihood: 0.7},
	}
	if !reflect.DeepEqual(res, expected) {
		t.Errorf("expected %#v, got %#v", expected, res)
	}
}

func TestPossibilitiesI32(t *testing.T) {
	// Little-endian float 1.0 (binary 0x3f800000)
	payload := []byte{0x00, 0x00, 0x80, 0x3f}
	res := Possibilities(wireI32, payload)

	foundFloat := false
	for _, r := range res {
		if r.Type == "float32" {
			foundFloat = true
			if r.Text != "1.0i32" && r.Text != "1i32" {
				t.Errorf("unexpected float32 format: %s", r.Text)
			}
		}
	}
	if !foundFloat {
		t.Errorf("float32 possibility not found in %#v", res)
	}
}

func TestPossibilitiesI64(t *testing.T) {
	// Little-endian float64 1.0 (binary 0x3ff0000000000000)
	payload := []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xf0, 0x3f}
	res := Possibilities(wireI64, payload)

	foundFloat := false
	for _, r := range res {
		if r.Type == "float64" {
			foundFloat = true
			if r.Text != "1.0" && r.Text != "1" {
				t.Errorf("unexpected float64 format: %s", r.Text)
			}
		}
	}
	if !foundFloat {
		t.Errorf("float64 possibility not found in %#v", res)
	}
}

func TestPossibilitiesLen(t *testing.T) {
	// String "hello" (binary 0x68, 0x65, 0x6c, 0x6c, 0x6f)
	payload := []byte("hello")
	res := Possibilities(wireLen, payload)

	var hasStr, hasBytes, hasPackedVarint bool
	for _, r := range res {
		switch r.Type {
		case "string":
			hasStr = true
			if r.Text != `{"hello"}` {
				t.Errorf("unexpected string text: %q", r.Text)
			}
		case "bytes":
			hasBytes = true
			if r.Text != "{`68 65 6c 6c 6f`}" {
				t.Errorf("unexpected bytes text: %q", r.Text)
			}
		case "packed_varint":
			hasPackedVarint = true
			if r.Text != "[ 104 101 108 108 111 ]" {
				t.Errorf("unexpected packed varints text: %q", r.Text)
			}
		}
	}

	if !hasStr || !hasBytes || !hasPackedVarint {
		t.Errorf("missing possibilities for 'hello': string=%v, bytes=%v, packed_varint=%v", hasStr, hasBytes, hasPackedVarint)
	}

	// Message {1: 150} (binary 0x08, 0x96, 0x01)
	payload = []byte{0x08, 0x96, 0x01}
	res = Possibilities(wireLen, payload)

	var hasMsg bool
	for _, r := range res {
		if r.Type == "message" {
			hasMsg = true
			if r.Text != "{ 1: 150 }" {
				t.Errorf("unexpected message text: %q", r.Text)
			}
		}
	}
	if !hasMsg {
		t.Errorf("missing message possibility for {1: 150}")
	}
}
