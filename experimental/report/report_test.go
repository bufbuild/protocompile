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

package report_test

import (
	"fmt"
	"testing"

	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/stretchr/testify/assert"
)

func TestReport(t *testing.T) {
	file1 := report.NewIndexedFile(report.File{
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
	file2 := report.NewIndexedFile(report.File{
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

	var r report.Report
	r.Errorf("input file was not valid UTF-8").With(
		report.InFile("input1.proto"),
		report.Note("encountered 0xff byte at offset 24"),
	)
	r.Errorf("the `syntax` keyword is deprecated").With(
		report.SnippetAtf(file1.NewSpan(0, 17), "change this to `edition = \"buf\"`"),
	)
	r.Errorf("previously allocated field number allocated again").With(
		report.SnippetAtf(file1.NewSpan(96, 97), "field number `1` is already allocated"),
		report.SnippetAtf(file1.NewSpan(71, 72), "first allocation occurs here"),
	)
	r.Errorf("only one of `optional`, `repeated`, or `required` is allowed per field").With(
		report.SnippetAtf(file1.NewSpan(101, 109), "required is deprecated >:("),
		report.SnippetAtf(file1.NewSpan(110, 118), "remove this modifier"),
		report.SnippetAtf(file1.NewSpan(119, 127), "already specified here"),
	)
	r.Warnf("the name `MyOtherCoolMessage` kinda sucks").With(
		report.SnippetAtf(file2.NewSpan(27, 45), "this one here"),
		report.Help("the \"My\" prefix is so Java"),
	)
	r.Errorf("the name `MyCoolEnum` defined multiple times").With(
		report.SnippetAtf(file2.NewSpan(112, 122), "redefined here"),
		report.SnippetAtf(file1.NewSpan(145, 192), "first definition is here"),
	)

	assert.Equal(t, ``, r.Render(report.Simple))
	assert.Equal(t, ``, r.Render(report.Monochrome))

	fmt.Print(r.Render(report.Colored))
}
