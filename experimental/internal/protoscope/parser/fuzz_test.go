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

package parser

import (
	"testing"

	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/source"
)

func FuzzParse(f *testing.F) {
	f.Add("1: 150")
	f.Add("2: \"testing\"")
	f.Add("3: [ 1 2 3 ]")
	f.Add("4: !{ 1: 42 }")
	f.Add("5:I32 100i32")

	f.Fuzz(func(_ *testing.T, input string) {
		src := source.NewFile("fuzz.protoscope", input)
		r := &report.Report{}
		_, _ = Parse("fuzz.protoscope", src, r)
	})
}
