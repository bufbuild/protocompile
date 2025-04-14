// Copyright 2020-2025 Buf Technologies, Inc.
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
	"encoding/base64"
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"gopkg.in/yaml.v3"

	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/internal/golden"
)

var ansiEscapePat = regexp.MustCompile("\033\\[([\\d;]*)m")

// ansiToMarkup converts ANSI escapes we care about in `text` into markup that is hopefully
// easier for humans to parse.
func ansiToMarkup(text string) string {
	return ansiEscapePat.ReplaceAllStringFunc(text, func(needle string) string {
		// We only handle a small subset of things we know.
		code := ansiEscapePat.FindStringSubmatch(needle)[1]

		colors := []string{"blk", "red", "grn", "ylw", "blu", "mta", "cyn", "wht"}
		place := []string{"", "", "bg", "", "+", "bg.+"}

		if code == "0" {
			code = "reset"
		} else {
			parts := strings.SplitN(code, ";", 2)
			var name strings.Builder
			if parts[0] == "1" {
				name.WriteString("b.")
			}
			name.WriteString(place[(parts[1][0]-'0')/2])
			name.WriteString(colors[parts[1][1]-'0'])
			code = name.String()
		}

		return "⟨" + code + "⟩"
	})
}

func TestRender(t *testing.T) {
	t.Parallel()

	corpus := golden.Corpus{
		Root:       "testdata",
		Refresh:    "PROTOCOMPILE_REFRESH",
		Extensions: []string{"yaml"},
		Outputs: []golden.Output{
			{Extension: "simple.txt"},
			{Extension: "fancy.txt"},
			{Extension: "color.txt"},
		},
	}

	corpus.Run(t, func(t *testing.T, path, text string, outputs []string) {
		r := new(report.Report)
		err := r.AppendFromProto(func(m proto.Message) error {
			// Convert YAML -> JSON. We don't use protoyaml here because that depends
			// on GRPC and that depends on the universe.
			bag := map[string]any{}
			if err := yaml.Unmarshal([]byte(text), bag); err != nil {
				return err
			}

			// Convert files.text into base64 to appease protojson.
			if files, ok := bag["files"]; ok {
				for _, file := range files.([]any) { //nolint:errcheck
					if file, ok := file.(map[string]any); ok {
						if text, ok := file["text"].(string); ok {
							file["text"] = base64.RawStdEncoding.EncodeToString([]byte(text))
						}
					}
				}
			}

			json, err := json.Marshal(bag)
			if err != nil {
				return err
			}

			return protojson.Unmarshal(json, m)
		})
		if err != nil {
			t.Fatalf("failed to parse input %q: %v", path, err)
		}

		text, _, _ = report.Renderer{
			Compact:     true,
			ShowRemarks: true,
			ShowDebug:   true,
		}.RenderString(r)
		outputs[0] = text
		if text != "" {
			text += "\n"
		}

		text, _, _ = report.Renderer{
			Compact:     false,
			ShowRemarks: true,
			ShowDebug:   true,
		}.RenderString(r)
		outputs[1] = text

		text, _, _ = report.Renderer{
			Colorize:    true,
			Compact:     false,
			ShowRemarks: true,
			ShowDebug:   true,
		}.RenderString(r)
		// This allows colored terminal output to be inspected using -test.v and
		// -test.skip.
		t.Log("\n" + text)
		outputs[2] = ansiToMarkup(text)
	})
}
