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

package report2_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/bufbuild/protocompile/report2"
	"github.com/stretchr/testify/assert"
)

func TestReport(t *testing.T) {
	file1 := report2.NewIndexedFile(report2.File{
		Path: "input1.proto",
		Text: `syntax = "proto2"

// Blah
message MyCoolMessage {
	optional int32 x = 1;

	repeated double y = 1;

	required optional repeated string z = 3;
}

enum MyCoolEnum {
	FOO = 1; BAR = 2;
	BAZ = 3;
}
`,
	})
	file2 := report2.NewIndexedFile(report2.File{
		Path: "input2.proto",
		Text: `syntax = "proto2"

message MyOtherCoolMessage {
	repeated MyCoolMessage y = 1;

	optional string z = 1;
}

enum MyCoolEnum {
	FOO = 1; BAR = 2;
	BAZ = 3;
}
`,
	})

	var r report2.Report
	r.Error(
		errors.New("input file was not valid UTF-8"),
		report2.MentionFile("input1.proto"),
		report2.Note("encountered 0xff byte at offset 24"),
	)
	r.Error(
		errors.New("the `syntax` keyword is deprecated"),
		report2.SnippetAt(file1.NewSpan(0, 17), "change this to `edition = \"buf\"`"),
	)
	r.Error(
		errors.New("previously allocated field number allocated again"),
		report2.SnippetAt(file1.NewSpan(96, 97), "field number `1` is already allocated"),
		report2.SnippetAt(file1.NewSpan(71, 72), "first allocation occurs here"),
	)
	r.Error(
		errors.New("only one of `optional`, `repeated`, or `required` is allowed per field"),
		report2.SnippetAt(file1.NewSpan(101, 109), "required is deprecated >:("),
		report2.SnippetAt(file1.NewSpan(110, 118), "remove this modifier"),
		report2.SnippetAt(file1.NewSpan(119, 127), "already specified here"),
	)
	r.Warn(
		errors.New("the name `MyOtherCoolMessage` kinda sucks"),
		report2.SnippetAt(file2.NewSpan(27, 45), "this one here"),
		report2.Help("the \"My\" prefix is so Java"),
	)
	r.Error(
		errors.New("the name `MyCoolEnum` defined multiple times"),
		report2.SnippetAt(file2.NewSpan(112, 122), "redefined here"),
		report2.SnippetAt(file1.NewSpan(145, 192), "first definition is here"),
	)

	assert.Equal(t, ``, r.Render(report2.Simple))
	assert.Equal(t, ``, r.Render(report2.Monochrome))

	fmt.Print(r.Render(report2.Colored))
}
