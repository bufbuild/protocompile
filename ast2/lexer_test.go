// Copyright 2020-2024 Buf Technologies, Inc.
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

package ast2_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/bufbuild/protocompile/ast2"
	"github.com/bufbuild/protocompile/report2"
)

type t struct {
	Kind       string
	Text       string
	Start, End int
	Value      any `json:",omitempty"`
}

func Lex(text string) ([]t, report2.Report) {
	var r report2.Report
	file := ast2.Parse(report2.File{Path: "input.proto", Text: text}, &r)

	var tokens []t
	for _, token := range file.Context().Iter {
		start, end := token.Span().Offsets()
		var value any
		if i, ok := token.AsInt(); ok {
			value = i
		} else if f, ok := token.AsFloat(); ok {
			value = f
		} else if s, ok := token.AsString(); ok {
			value = s
		}
		tokens = append(tokens, t{
			Text:  token.Text(),
			Start: start,
			End:   end,
			Kind:  token.Kind().String(),
			Value: value,
		})
	}

	return tokens, r
}

func TestLexer(t *testing.T) {
	tokens, report := Lex(`// Copyright 2024 Buf Technologies, Inc.
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

syntax = "proto3";

package buf.plugin.generate.v1;

import "buf/plugin/descriptor/v1/file_descriptor.proto";
import "buf/plugin/file/v1/file.proto";
import "buf/plugin/option/v1/option.proto";
import "buf/validate/validate.proto";

option go_package = "buf.build/gen/go/bufbuild/bufplugin/p" "rotocolbuffers/go/buf/plugin/generate/v1";

// The service that defines generate operations.
service GenerateService {
  // Generate.
  rpc Generate(GenerateRequest) returns (GenerateResponse) {
    option idempotency_level = NO_SIDE_EFFECTS;
  }
  // Get information about a plugin.
  rpc GetInfo(GetInfoRequest) returns (GetInfoResponse) {
    option idempotency_level = NO_SIDE_EFFECTS;
  }
}

// A request for generation.
message GenerateRequest {
  // The FileDescriptors to generate for.
  //
  // Required.
  //
  // FileDescriptors are guaranteed to be unique by file_descriptor_proto.name.
  //
  // FileDescriptors will be self-contained, that is any import of a FileDescriptor will be
  // contained within file_descriptors.
  //
  // Content should only be generated for non-imports.
  //
  // FileDescriptors will appear in topological order, that is each FileDescriptor will appear
  // before any FileDescriptor that imports it.
  //
  // FileDescriptors will contain both runtime-retention options and source-retention options.
  repeated buf.plugin.descriptor.v1.FileDescriptor file_descriptors = 1 [
    (buf.validate.field).repeated.min_items = 1,
    (buf.validate.field).cel = {
      id: "file_descriptor_names_unique"
      message: "FileDescriptor names must be unique"
      expression: "
	  	this.filter(file_descriptor, has(file_descriptor.file_descriptor_proto))
			.map(file_descriptor, file_descriptor.file_descriptor_proto.name)
			.unique()
	  "
    }
  ];
  // Options for generation.
  //
  // Optional.
  //
  // For now, callers are expected to know what keys are available, and how to construct Values
  // for each key. In the future, we may add a ListOptions or otherwise to GenerateService that will
  // describe the available options keys, which can then be validated against.
  //
  // It is acceptable for a plugin to return an error on an unknown or misconstructed Option.
  repeated buf.plugin.option.v1.Option options = 2;
}

// A response containing generated Files.
message GenerateResponse {
  // The generated Files.
  //
  // File are guaranteed to be unique by path.
  repeated buf.plugin.file.v1.File files = 1 [(buf.validate.field).cel = {
    id: "file_paths_unique"
    message: "File paths must be unique"
    expression: "this.map(file, file.path).unique()"
  }];
}

// A request for info.
message GetInfoRequest {}

// The information about the plugin.
message GetInfoResponse {
  // The maximum edition that the plugin supports.
  int32 maximum_edition = 1;
}`)
	for _, t := range tokens {
		j, _ := json.Marshal(t)
		fmt.Printf("%s\n", string(j))
	}
	if len(report) > 0 {
		fmt.Println(report.Render(report2.Colored))
	}
}
