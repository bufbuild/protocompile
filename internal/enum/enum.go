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

// enum is a helper for generating boilerplate related to Go enums.
//
// To generate boilerplate for a given file, use
//
//	//go:generate go run github.com/bufbuild/protocompile/internal/enum
//
// There should be a file with the same name as the file to generate with a
// .yaml prefix. E.g., if the generate directive appears in foo.go, it should
// there should be a foo.go.yaml file, which must contain an array of the
// Enum type defined in this package.
//
//nolint:revive // We use _ in field names to disambiguate them from methods, while still exporting them.
package main

import (
	"bytes"
	"debug/buildinfo"
	_ "embed"
	"errors"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"

	"github.com/bufbuild/protocompile/internal/ext/osx"
	"github.com/bufbuild/protocompile/internal/ext/slicesx"
)

//nolint:tagliatelle // Weird opinions on YAML fields.
type Enum struct {
	Name    string   `yaml:"name"`  // The name of the new type.
	Type    string   `yaml:"type"`  // The underlying type.
	Docs    string   `yaml:"docs"`  // Documentation for the type.
	Total   string   `yaml:"total"` // The name of a "total values" constant.
	Methods []Method `yaml:"methods"`
	Values  []Value  `yaml:"values"`

	UseMapsForLookup bool `yaml:"use_maps_for_lookup"`
	DocIsString      bool `yaml:"doc_is_string"`
}

func (e *Enum) Init() {
	for i := range e.Values {
		e.Values[i].Parent = e
		e.Values[i].Idx = i
	}
	for i := range e.Methods {
		e.Methods[i].Parent = e
	}
}

type Value struct {
	Name    string   `yaml:"name"`   // The name of the value.
	Alias   string   `yaml:"alias"`  // Another value this value aliases, if any.
	String_ string   `yaml:"string"` // The string representation of this value.
	Docs_   string   `yaml:"docs"`   // Documentation for the value.
	From    []string `yaml:"from"`   // Names for use in from-string instead of String_.
	Expr    string   `yaml:"expr"`   // The Go expression to use for this value.

	Parent *Enum `yaml:"-"`
	Idx    int   `yaml:"-"`
}

func (v Value) HasSuffixDocs() bool {
	next, ok := slicesx.Get(v.Parent.Values, v.Idx+1)
	return v.Docs() != "" && !strings.Contains(v.Docs(), "\n") && (!ok || next.Docs() != "")
}

func (v Value) String() string {
	if v.String_ == "" {
		return v.Name
	}
	return v.String_
}

func (v Value) Docs() string {
	if v.Docs_ != "" {
		return v.Docs_
	}
	if v.Parent.DocIsString {
		return v.String_
	}
	return ""
}

type Method struct {
	Kind  MethodKind `yaml:"kind"` // The kind of method to generate.
	Name_ string     `yaml:"name"` // The method's name; optional for some methods.
	Docs_ string     `yaml:"docs"` // Documentation for the method.
	Skip  []string   `yaml:"skip"` // Enum values to ignore in this method.

	Parent *Enum `yaml:"-"`
}

func (m Method) Name() (string, error) {
	if m.Name_ != "" {
		return m.Name_, nil
	}

	switch m.Kind {
	case MethodFromString, MethodAll:
		return "", fmt.Errorf("missing name for kind: %#v", m.Kind)
	case MethodGoString:
		return "GoString", nil
	case MethodString:
		return "String", nil
	default:
		return "", fmt.Errorf("unexpected kind: %#v", m.Kind)
	}
}

func (m Method) Docs() string {
	if m.Docs_ != "" {
		return m.Docs_
	}

	switch m.Kind {
	case MethodGoString:
		return "GoString implements [fmt.GoStringer]."
	case MethodString:
		return "String implements [fmt.Stringer]."
	default:
		return ""
	}
}

type Range struct{ Start, End int }

// Returns the ranges (exclusive) of values not skipped by this method.
func (m Method) Ranges() []Range {
	var out []Range //nolint:prealloc // This lint is completely wrong here.
	var aliases int
	for i, v := range m.Parent.Values {
		if v.Alias != "" {
			aliases++
			continue
		}

		if slices.Contains(m.Skip, v.Name) {
			continue
		}

		i -= aliases
		last := slicesx.LastPointer(out)
		if last != nil && last.End == i {
			last.End++
			continue
		}

		out = append(out, Range{i, i + 1})
	}
	return out
}

type MethodKind string

const (
	MethodString     MethodKind = "string"
	MethodGoString   MethodKind = "go-string"
	MethodFromString MethodKind = "from-string"
	MethodAll        MethodKind = "all"
)

//go:embed enum.go.tmpl
var tmplText string

// makeDocs converts a data into doc comments.
func makeDocs(data, indent string) string {
	if data == "" {
		return ""
	}

	var out strings.Builder
	for _, line := range strings.Split(strings.TrimSpace(data), "\n") {
		out.WriteString(indent)
		if line == "" {
			out.WriteString("//\n")
			continue
		}
		out.WriteString("// ")
		out.WriteString(line)
		out.WriteString("\n")
	}
	return out.String()
}

func Main(config string) error {
	if filepath.Ext(config) != ".yaml" {
		return errors.New("file argument must end in .yaml")
	}

	var input struct {
		Binary, Package, Path, Config string
		YAML                          []Enum
	}
	input.Package = os.Getenv("GOPACKAGE")
	input.Config = config
	input.Path = strings.TrimSuffix(config, ".yaml") + ".go"

	buildinfo, err := buildinfo.ReadFile(os.Args[0])
	if err != nil {
		return err
	}
	input.Binary = buildinfo.Path

	text, err := os.ReadFile(config)
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(text, &input.YAML); err != nil {
		return err
	}
	for i := range input.YAML {
		input.YAML[i].Init()
	}

	tmpl, err := template.New("enum.go.tmpl").Funcs(template.FuncMap{
		"makeDocs": makeDocs,
		"contains": slices.Contains[[]string],
	}).Parse(tmplText)
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, input); err != nil {
		return err
	}

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return err
	}

	return os.WriteFile(input.Path, formatted, osx.PermAR|osx.PermAW)
}

func main() {
	var failed bool
	for _, config := range os.Args[1:] {
		if err := Main(config); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %s", config, err)
			failed = true
		}
	}

	if failed {
		os.Exit(1)
	}
}
