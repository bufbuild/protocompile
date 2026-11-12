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

// Package protoscope provides high-level APIs for parsing, assembling, disassembling,
// inspecting, and diagnosing the Protoscope text format.
//
// Protoscope is a human-readable text format for raw Protobuf binary wire payloads,
// allowing payloads to be authored, edited, compiled (assembled), and decompiled (disassembled)
// without requiring a schema (`.proto` file).
//
// # Framing Support
//
// Protoscope supports multi-frame documents (separated by `---`) and transport framing formats:
//   - [FramingNone] (raw Protobuf wire bytes)
//   - [FramingGRPC] (gRPC 5-byte prefix framing: 1-byte flags + 4-byte length)
//   - [FramingConnect] (ConnectRPC stream framing)
//   - [FramingVarint] (Varint-delimited length prefix framing)
//
// # Key Functions
//
//   - [Assemble] / [AssembleWithOptions]: Compiles Protoscope text into raw or framed Protobuf wire format.
//   - [Disassemble]: Decompiles raw or framed Protobuf binary wire bytes back into Protoscope text format.
//   - [Diagnostics]: Parses Protoscope text and returns a [report.Report] containing syntactic and structural diagnostics.
//   - [Inspect]: Provides context-aware tooltip information (tag numbers, wire types, numerical representations, zigzag values) for IDE hover integration.
//   - [Possibilities]: Analyzes raw binary payload bytes for a given wire type and returns alternative interpretation possibilities sorted by likelihood.
package protoscope
