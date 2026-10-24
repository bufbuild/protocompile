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
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"

	"github.com/bufbuild/protocompile"
	"github.com/bufbuild/protocompile/experimental/internal/protoscope/disassembler"
	"github.com/bufbuild/protocompile/experimental/internal/protoscope/parser"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/source"
)

func TestTestDataIntegration(t *testing.T) {
	files, err := filepath.Glob("../testdata/*.protoscope")
	if err != nil {
		t.Fatal(err)
	}

	for _, file := range files {
		name := filepath.Base(file)
		t.Run(name, func(t *testing.T) {
			content, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}

			src := source.NewFile(name, string(content))
			r := &report.Report{}
			parsed, ok := parser.Parse(name, src, r)
			if !ok {
				t.Fatalf("failed to parse: %v", r.Diagnostics)
			}

			gotBytes := Assemble(parsed)

			pbFile := strings.TrimSuffix(file, ".protoscope") + ".pb"
			if _, err := os.Stat(pbFile); err == nil {
				expectedBytes, err := os.ReadFile(pbFile)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(gotBytes, expectedBytes) {
					t.Errorf("assembled bytes mismatch for %s\ngot:  %x\nwant: %x", name, gotBytes, expectedBytes)
				}
			} else {
				t.Logf("no corresponding .pb file for %s", name)
			}
		})
	}
}

func TestDisassemblerRoundTrip(t *testing.T) {
	files, err := filepath.Glob("../testdata/*.pb")
	if err != nil {
		t.Fatal(err)
	}

	for _, pbFile := range files {
		name := filepath.Base(pbFile)
		t.Run(name, func(t *testing.T) {
			originalBytes, err := os.ReadFile(pbFile)
			if err != nil {
				t.Fatal(err)
			}

			// 1. Disassemble
			var buf bytes.Buffer
			if err := disassembler.Disassemble(originalBytes, &buf); err != nil {
				t.Fatalf("Disassemble failed: %v", err)
			}
			disassembledText := buf.String()
			t.Logf("Disassembled text:\n%s", disassembledText)

			// 2. Parse
			src := source.NewFile(name+".protoscope", disassembledText)
			r := &report.Report{}
			parsed, ok := parser.Parse(name+".protoscope", src, r)
			if !ok {
				t.Fatalf("failed to parse disassembled text: %v", r.Diagnostics)
			}

			// 3. Assemble
			gotBytes := Assemble(parsed)

			// 4. Compare
			if !bytes.Equal(gotBytes, originalBytes) {
				t.Errorf("round-trip failed for %s\ngot:  %x\nwant: %x", name, gotBytes, originalBytes)
			}
		})
	}
}

func TestAllTypesDynamic(t *testing.T) {
	// Compile the two proto files dynamically
	compiler := &protocompile.Compiler{
		Resolver: &protocompile.SourceResolver{
			ImportPaths: []string{"../testdata"},
		},
	}
	ctx := context.Background()
	fds, err := compiler.Compile(ctx, "all_types.proto", "all_types_proto2.proto")
	if err != nil {
		t.Fatalf("failed to compile proto files: %v", err)
	}

	var allTypesFD, allTypesProto2FD protoreflect.FileDescriptor
	for _, fd := range fds {
		if fd.Path() == "all_types.proto" {
			allTypesFD = fd
		} else if fd.Path() == "all_types_proto2.proto" {
			allTypesProto2FD = fd
		}
	}

	if allTypesFD == nil || allTypesProto2FD == nil {
		t.Fatal("failed to find compiled file descriptors")
	}

	// 1. Fill and test AllTypes (proto3)
	t.Run("AllTypes (proto3)", func(t *testing.T) {
		md := allTypesFD.Messages().ByName("AllTypes")
		msg := dynamicpb.NewMessage(md)

		// Set values for every single field type
		msg.Set(md.Fields().ByNumber(1), protoreflect.ValueOfFloat64(123.456))
		msg.Set(md.Fields().ByNumber(2), protoreflect.ValueOfFloat32(78.9))
		msg.Set(md.Fields().ByNumber(3), protoreflect.ValueOfInt32(-42))
		msg.Set(md.Fields().ByNumber(4), protoreflect.ValueOfInt64(-123456789))
		msg.Set(md.Fields().ByNumber(5), protoreflect.ValueOfUint32(42))
		msg.Set(md.Fields().ByNumber(6), protoreflect.ValueOfUint64(123456789))
		msg.Set(md.Fields().ByNumber(7), protoreflect.ValueOfInt32(-10))
		msg.Set(md.Fields().ByNumber(8), protoreflect.ValueOfInt64(-1000))
		msg.Set(md.Fields().ByNumber(9), protoreflect.ValueOfUint32(55))
		msg.Set(md.Fields().ByNumber(10), protoreflect.ValueOfUint64(66))
		msg.Set(md.Fields().ByNumber(11), protoreflect.ValueOfInt32(-77))
		msg.Set(md.Fields().ByNumber(12), protoreflect.ValueOfInt64(-88))
		msg.Set(md.Fields().ByNumber(13), protoreflect.ValueOfBool(true))
		msg.Set(md.Fields().ByNumber(14), protoreflect.ValueOfString("hello world"))
		msg.Set(md.Fields().ByNumber(15), protoreflect.ValueOfBytes([]byte{0x01, 0x02, 0x03}))

		nestedMd := md.Fields().ByNumber(16).Message()
		nestedMsg := dynamicpb.NewMessage(nestedMd)
		nestedMsg.Set(nestedMd.Fields().ByNumber(1), protoreflect.ValueOfInt32(999))
		msg.Set(md.Fields().ByNumber(16), protoreflect.ValueOfMessage(nestedMsg))

		msg.Set(md.Fields().ByNumber(17), protoreflect.ValueOfEnum(1))

		repeatedField := msg.Mutable(md.Fields().ByNumber(18)).List()
		repeatedField.Append(protoreflect.ValueOfInt32(11))
		repeatedField.Append(protoreflect.ValueOfInt32(22))
		repeatedField.Append(protoreflect.ValueOfInt32(33))

		mapField := msg.Mutable(md.Fields().ByNumber(19)).Map()
		mapField.Set(protoreflect.MapKey(protoreflect.ValueOfString("key1")), protoreflect.ValueOfString("val1"))
		mapField.Set(protoreflect.MapKey(protoreflect.ValueOfString("key2")), protoreflect.ValueOfString("val2"))

		originalBytes, err := proto.Marshal(msg)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}

		verifyRoundTrip(t, "all_types_proto3", originalBytes)
	})

	// 2. Fill and test AllTypesProto2 (proto2) for groups and unpacked fields
	t.Run("AllTypesProto2 (proto2)", func(t *testing.T) {
		md2 := allTypesProto2FD.Messages().ByName("AllTypesProto2")
		msg2 := dynamicpb.NewMessage(md2)

		groupMd := md2.Fields().ByNumber(1).Message()
		groupList := msg2.Mutable(md2.Fields().ByNumber(1)).List()

		groupMsg1 := dynamicpb.NewMessage(groupMd)
		groupMsg1.Set(groupMd.Fields().ByNumber(2), protoreflect.ValueOfInt32(50))
		groupList.Append(protoreflect.ValueOfMessage(groupMsg1))

		groupMsg2 := dynamicpb.NewMessage(groupMd)
		groupMsg2.Set(groupMd.Fields().ByNumber(2), protoreflect.ValueOfInt32(60))
		groupList.Append(protoreflect.ValueOfMessage(groupMsg2))

		unpackedList := msg2.Mutable(md2.Fields().ByNumber(3)).List()
		unpackedList.Append(protoreflect.ValueOfInt32(70))
		unpackedList.Append(protoreflect.ValueOfInt32(80))

		originalBytes, err := proto.Marshal(msg2)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}

		verifyRoundTrip(t, "all_types_proto2", originalBytes)
	})
}

func TestHeuristicEdgeCases(t *testing.T) {
	// Edge Case 1: Printable string looking like 4 fields (false positive for isMessage)
	// ASCII string: " 0(10283"
	// Parsed as fields:
	// - tag 4, wire 0, value 48 (0x20 0x30)
	// - tag 5, wire 0, value 49 (0x28 0x31)
	// - tag 6, wire 0, value 50 (0x30 0x32)
	// - tag 7, wire 0, value 51 (0x38 0x33)
	// Wrap as length-delimited field 1. Total bytes: 0a 08 20 30 28 31 30 32 38 33
	t.Run("printable string false positive message", func(t *testing.T) {
		bytesVal := []byte{0x0a, 0x08, 0x20, 0x30, 0x28, 0x31, 0x30, 0x32, 0x38, 0x33}
		verifyRoundTrip(t, "printable_str_looks_like_msg", bytesVal)
	})

	// Edge Case 2: Submessage looking like printable string (false negative for isMessage)
	// Bytes: 0a 02 30 32
	// (Field 1, length 2, content: 0x30 0x32)
	// Inside submessage: tag 6, wire 0, value 50 (ASCII "02").
	// Classified as string because length <= 3 fields and 100% printable.
	t.Run("submessage false negative string", func(t *testing.T) {
		bytesVal := []byte{0x0a, 0x02, 0x30, 0x32}
		verifyRoundTrip(t, "submsg_looks_like_str", bytesVal)
	})

	// Edge Case 3: Submessage containing a group (false negative for isMessage)
	// Bytes: 0a 04 0b 10 03 0c
	// (Field 1, length 4, content: 0x0b 0x10 0x03 0x0c)
	// Inside submessage: tag 1, wireType 3 (SGROUP), tag 2, wireType 0, value 3, tag 1, wireType 4 (EGROUP)
	// Classified as hex string because contains groups.
	t.Run("submessage containing group", func(t *testing.T) {
		bytesVal := []byte{0x0a, 0x04, 0x0b, 0x10, 0x03, 0x0c}
		verifyRoundTrip(t, "submsg_with_group", bytesVal)
	})
}

func verifyRoundTrip(t *testing.T, name string, originalBytes []byte) {
	// 1. Disassemble
	var buf bytes.Buffer
	if err := disassembler.Disassemble(originalBytes, &buf); err != nil {
		t.Fatalf("Disassemble failed: %v", err)
	}
	disassembledText := buf.String()
	t.Logf("[%s] Disassembled:\n%s", name, disassembledText)

	// 2. Parse
	src := source.NewFile(name+".protoscope", disassembledText)
	r := &report.Report{}
	parsed, ok := parser.Parse(name+".protoscope", src, r)
	if !ok {
		t.Fatalf("failed to parse disassembled text: %v", r.Diagnostics)
	}

	// 3. Assemble
	gotBytes := Assemble(parsed)

	// 4. Compare
	if !bytes.Equal(gotBytes, originalBytes) {
		t.Errorf("round-trip failed for %s\ngot:  %x\nwant: %x", name, gotBytes, originalBytes)
	}
}
