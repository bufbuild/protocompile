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

package ir_test

import (
	"bytes"
	"flag"
	"maps"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
	"gopkg.in/yaml.v3"

	"github.com/bufbuild/protocompile/experimental/ast/predeclared"
	"github.com/bufbuild/protocompile/experimental/incremental"
	"github.com/bufbuild/protocompile/experimental/incremental/queries"
	"github.com/bufbuild/protocompile/experimental/ir"
	"github.com/bufbuild/protocompile/experimental/ir/presence"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/internal/ext/cmpx"
	"github.com/bufbuild/protocompile/internal/ext/iterx"
	"github.com/bufbuild/protocompile/internal/ext/slicesx"
	compilerpb "github.com/bufbuild/protocompile/internal/gen/buf/compiler/v1alpha1"
	"github.com/bufbuild/protocompile/internal/golden"
	"github.com/bufbuild/protocompile/internal/prototest"
)

var (
	tracing = flag.Int("ir.tracing", 0, "trace depth for diagnostics")
)

// Test is the type that a test case for the compiler is deserialized from.
//
// If the test is defined as a .yaml file, that file is expected to conform to
// this type. If the test is defined as a .proto file, all comments starting
// with the string '//% ' will be concatenated to form configuration for the
// test, and the .proto file itself will be added to Files.
//
//nolint:tagliatelle
type Test struct {
	// The files under test.
	Files []File `yaml:"files"`

	// Regular expressions that the messages of diagnostics we're interested in
	// must match. If empty, all diagnostics are accepted.
	Filters List[*regexp.Regexp] `yaml:"filters"`
	Exclude List[*regexp.Regexp] `yaml:"exclude"`

	// Whether to exclude the WKT sources in the default opener, and whether to
	// output WKT
	ExcludeWKTSources bool `yaml:"exclude_wkt_sources"`

	// Whether to output a FileDescriptorSet.
	Descriptor bool `yaml:"descriptor"`
	// Whether the descriptor should include SourceCodeInfo
	SourceCodeInfo bool `yaml:"source_code_info"`

	// Whether to output a symbol table. Useful for tests that build symbol
	// tables.
	Symtab bool `yaml:"symtab"`
}

func (t *Test) Unmarshal(path string, text string) error {
	switch filepath.Ext(path) {
	case ".proto":
		config := new(bytes.Buffer)
		for line := range strings.Lines(text) {
			if line, ok := strings.CutPrefix(line, "//% "); ok {
				config.WriteString(line)
			}
		}

		if err := yaml.Unmarshal(config.Bytes(), &t); err != nil {
			return err
		}

		t.Files = append(t.Files, File{Path: path, Text: text})

	case ".yaml":
		return yaml.Unmarshal([]byte(text), &t)
	}

	return nil
}

type File struct {
	Path string `yaml:"path"`
	Text string `yaml:"text"`

	// Whether this file should be treated as an import-only file.
	Import bool `yaml:"import"`
}

// List is a YAML deserializable type that can be deserialized either
// as a YAML array, or as a single value.
type List[T any] []T

func (l *List[T]) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind&yaml.SequenceNode == 0 {
		*l = make([]T, 1)
		return value.Decode(&(*l)[0])
	}
	return value.Decode((*[]T)(l))
}

func TestIR(t *testing.T) {
	t.Parallel()

	corpus := golden.Corpus{
		Root:       "testdata",
		Refresh:    "PROTOCOMPILE_REFRESH",
		Extensions: []string{"proto", "proto.yaml"},
		Outputs: []golden.Output{
			{Extension: "stderr.txt"},
			{Extension: "fds.yaml"},
			{Extension: "symtab.yaml"},
		},
	}

	corpus.Run(t, func(t *testing.T, path, text string, outputs []string) {
		test := new(Test)
		require.NoError(t, test.Unmarshal(path, text))

		var files source.Opener = source.NewMap(maps.Collect(iterx.Map1To2(
			slices.Values(test.Files),
			func(f File) (string, *source.File) {
				return f.Path, source.NewFile(f.Path, f.Text)
			},
		)))

		if !test.ExcludeWKTSources {
			files = &source.Openers{files, source.WKTs()}
		}

		exec := incremental.New(
			incremental.WithParallelism(1),
			incremental.WithReportOptions(report.Options{Tracing: *tracing}),
		)

		session := new(ir.Session)
		queries := slices.Collect(iterx.FilterMap(
			slices.Values(test.Files),
			func(f File) (incremental.Query[*ir.File], bool) {
				if f.Import {
					return nil, false
				}
				return queries.IR{
					Opener:  files,
					Session: session,
					Path:    f.Path,
				}, true
			},
		))

		results, r, err := incremental.Run(t.Context(), exec, queries...)
		require.NoError(t, err)

		r.Diagnostics = slices.DeleteFunc(r.Diagnostics, func(d report.Diagnostic) bool {
			matches := func(r *regexp.Regexp) bool {
				return r.MatchString(d.Message())
			}

			return slices.ContainsFunc(test.Exclude, matches) ||
				(test.Filters != nil && !slices.ContainsFunc(test.Filters, matches))
		})

		stderr, _, _ := report.Renderer{
			Colorize:  true,
			ShowDebug: true,
		}.RenderString(r)
		t.Log(stderr)
		outputs[0], _, _ = report.Renderer{}.RenderString(r)
		assert.NotContains(t, outputs[0], "unexpected panic; this is a bug")
		if !test.Descriptor && !test.Symtab {
			require.NotEmpty(t, outputs[0], "test must emit diagnostics")
			return
		}

		irs := slicesx.Transform(results, func(r incremental.Result[*ir.File]) *ir.File { return r.Value })
		irs = slices.DeleteFunc(irs, func(f *ir.File) bool { return f == nil })

		if test.Descriptor {
			bytes, err := ir.DescriptorSetBytes(irs,
				ir.IncludeSourceCodeInfo(test.SourceCodeInfo),
				ir.ExcludeFiles((*ir.File).IsDescriptorProto),
			)
			require.NoError(t, err)

			fds := new(descriptorpb.FileDescriptorSet)
			require.NoError(t, proto.Unmarshal(bytes, fds))
			assert.False(t, iterx.Empty2(fds.ProtoReflect().Range), "empty descriptor")

			outputs[1] = prototest.ToYAML(fds, prototest.ToYAMLOptions{})
		}

		if test.Symtab {
			symtab := symtabProto(irs)
			assert.False(t, iterx.Empty2(symtab.ProtoReflect().Range), "empty symtab")

			outputs[2] = prototest.ToYAML(symtab, prototest.ToYAMLOptions{})
		}
	})
}

func symtabProto(files []*ir.File) *compilerpb.SymbolSet {
	set := new(compilerpb.SymbolSet)
	set.Tables = make(map[string]*compilerpb.SymbolTable)

	for _, file := range files {
		// All features relevant to this file.
		featureExtns := make(map[ir.Member]struct{})
		dumpFeatureExtns := func(options ir.MessageValue) {
			for value := range options.Fields() {
				if value.Field().IsExtension() {
					featureExtns[value.Field()] = struct{}{}
				}
			}
		}
		dumpFeatureExtns(file.FeatureSet().Options())
		for ty := range seq.Values(file.AllTypes()) {
			dumpFeatureExtns(ty.FeatureSet().Options())

			for v := range seq.Values(ty.Members()) {
				dumpFeatureExtns(v.FeatureSet().Options())
			}
			for v := range seq.Values(ty.Oneofs()) {
				dumpFeatureExtns(v.FeatureSet().Options())
			}
			for v := range seq.Values(ty.ExtensionRanges()) {
				dumpFeatureExtns(v.FeatureSet().Options())
			}
		}
		for v := range seq.Values(file.AllExtensions()) {
			dumpFeatureExtns(v.FeatureSet().Options())
		}

		dumpFeatures := func(features ir.FeatureSet, target ir.OptionTarget) []*compilerpb.Feature {
			var out []*compilerpb.Feature
			dumpMessage := func(extn ir.Member, ty ir.Type) {
				for field := range seq.Values(ty.Members()) {
					if field.FeatureInfo().IsZero() || !field.CanTarget(target) {
						continue
					}

					feature := features.LookupCustom(extn, field)
					ty := feature.Type()
					var valueString string
					switch {
					case feature.IsZero():
						continue
					case ty.IsEnum():
						n, _ := feature.Value().AsInt()
						ev := ty.MemberByNumber(int32(n))
						if !ev.IsZero() {
							valueString = ev.Name()
						} else {
							valueString = strconv.Itoa(int(n))
						}
					case ty.Predeclared() == predeclared.Bool:
						b, _ := feature.Value().AsBool()
						valueString = strconv.FormatBool(b)
					default:
						valueString = "<invalid type>"
					}

					out = append(out, &compilerpb.Feature{
						Name:     feature.Field().Name(),
						Extn:     string(extn.FullName()),
						Value:    valueString,
						Explicit: !feature.IsInherited(),
					})
				}
			}

			dumpMessage(ir.Member{}, file.FindSymbol("google.protobuf.FeatureSet").AsType())
			for extn := range featureExtns {
				dumpMessage(extn, extn.Element())
			}

			slices.SortStableFunc(out, cmpx.Join(
				cmpx.Map(func(f *compilerpb.Feature) bool { return !f.Explicit }, cmpx.Bool),
				cmpx.Key((*compilerpb.Feature).GetExtn),
				cmpx.Key((*compilerpb.Feature).GetName),
			))
			return out
		}

		// Don't bother if the file only has a single symbol for its
		// package, and no options.
		if file.Options().IsZero() {
			switch file.Symbols().Len() {
			case 0:
				continue
			case 1:
				if file.Symbols().At(0).Kind() == ir.SymbolKindPackage {
					continue
				}
			}
		}

		symtab := &compilerpb.SymbolTable{
			Options:  new(optionWalker).message(file.Options()),
			Features: dumpFeatures(file.FeatureSet(), ir.OptionTargetFile),
		}

		for imp := range seq.Values(file.TransitiveImports()) {
			symtab.Imports = append(symtab.Imports, &compilerpb.Import{
				Path:       imp.Path(),
				Public:     imp.Public,
				Weak:       imp.Weak,
				Transitive: !imp.Direct,
				Visible:    imp.Visible,
			})
		}
		slices.SortFunc(symtab.Imports, cmpx.Key(func(x *compilerpb.Import) string { return x.Path }))

		for sym := range seq.Values(file.Symbols()) {
			if strings.HasPrefix(sym.Context().Path(), "google/protobuf/") {
				continue
			}

			var options ir.MessageValue
			switch sym.Kind() {
			case ir.SymbolKindMessage, ir.SymbolKindEnum:
				options = sym.AsType().Options()
			case ir.SymbolKindField, ir.SymbolKindExtension, ir.SymbolKindEnumValue:
				options = sym.AsMember().Options()
			case ir.SymbolKindOneof:
				options = sym.AsOneof().Options()
			}

			symtab.Symbols = append(symtab.Symbols, &compilerpb.Symbol{
				Fqn:        string(sym.FullName()),
				Kind:       compilerpb.Symbol_Kind(sym.Kind()),
				File:       sym.Context().Path(),
				Index:      uint32(sym.RawData()),
				Visible:    sym.Kind() != ir.SymbolKindPackage && sym.Visible(file, false),
				OptionOnly: sym.Kind() != ir.SymbolKindPackage && !sym.Visible(file, true) && sym.Visible(file, true),
				Options:    new(optionWalker).message(options),
				Features:   dumpFeatures(sym.FeatureSet(), sym.Kind().OptionTarget()),
			})
		}
		slices.SortFunc(symtab.Symbols,
			cmpx.Join(
				cmpx.Key(func(x *compilerpb.Symbol) string { return x.File }),
				cmpx.Key(func(x *compilerpb.Symbol) compilerpb.Symbol_Kind { return x.Kind }),
				cmpx.Key(func(x *compilerpb.Symbol) uint32 { return x.Index }),
			),
		)

		set.Tables[file.Path()] = symtab
	}

	return set
}

type optionWalker struct {
	path  map[ir.MessageValue]int
	depth int
}

func (ow *optionWalker) message(v ir.MessageValue) *compilerpb.Value {
	if v.IsZero() {
		return nil
	}
	if depth, ok := ow.path[v]; ok {
		return &compilerpb.Value{Value: &compilerpb.Value_Cycle{Cycle: int32(ow.depth - depth)}}
	}

	if ow.path == nil {
		ow.path = make(map[ir.MessageValue]int)
	}
	ow.path[v] = ow.depth
	ow.depth++
	defer func() {
		ow.depth--
		delete(ow.path, v)
	}()

	if concrete := v.Concrete(); concrete != v {
		return &compilerpb.Value{Value: &compilerpb.Value_Any_{Any: &compilerpb.Value_Any{
			Url:   concrete.TypeURL(),
			Value: ow.value(concrete.AsValue()),
		}}}
	}

	m := new(compilerpb.Value_Message)
	for elem := range v.Fields() {
		if elem.Field().IsExtension() {
			if m.Extns == nil {
				m.Extns = make(map[string]*compilerpb.Value)
			}
			m.Extns[string(elem.Field().FullName())] = ow.value(elem)
		} else {
			if m.Fields == nil {
				m.Fields = make(map[string]*compilerpb.Value)
			}
			m.Fields[elem.Field().Name()] = ow.value(elem)
		}
	}

	return &compilerpb.Value{Value: &compilerpb.Value_Message_{Message: m}}
}

func (ow *optionWalker) value(v ir.Value) *compilerpb.Value {
	if v.IsZero() {
		return nil
	}

	element := func(v ir.Element) *compilerpb.Value {
		switch v.Field().Element().Predeclared() {
		case predeclared.Int32, predeclared.SInt32, predeclared.SFixed32:
			x, _ := v.AsInt()
			return &compilerpb.Value{Value: &compilerpb.Value_I32{I32: int32(x)}}
		case predeclared.UInt32, predeclared.Fixed32:
			x, _ := v.AsUInt()
			return &compilerpb.Value{Value: &compilerpb.Value_U32{U32: uint32(x)}}
		case predeclared.Float32:
			x, _ := v.AsFloat()
			return &compilerpb.Value{Value: &compilerpb.Value_F32{F32: float32(x)}}

		case predeclared.Int64, predeclared.SInt64, predeclared.SFixed64:
			x, _ := v.AsInt()
			return &compilerpb.Value{Value: &compilerpb.Value_I64{I64: x}}
		case predeclared.UInt64, predeclared.Fixed64:
			x, _ := v.AsUInt()
			return &compilerpb.Value{Value: &compilerpb.Value_U64{U64: x}}
		case predeclared.Float64:
			x, _ := v.AsFloat()
			return &compilerpb.Value{Value: &compilerpb.Value_F64{F64: x}}

		case predeclared.String, predeclared.Bytes:
			x, _ := v.AsString()
			return &compilerpb.Value{Value: &compilerpb.Value_String_{String_: []byte(x)}}

		case predeclared.Bool:
			x, _ := v.AsBool()
			return &compilerpb.Value{Value: &compilerpb.Value_Bool{Bool: x}}
		}

		if v.Field().Element().IsEnum() {
			x, _ := v.AsInt()
			return &compilerpb.Value{Value: &compilerpb.Value_I32{I32: int32(x)}}
		}

		return ow.message(v.AsMessage())
	}

	if v.AsMessage().TypeURL() == "" && v.Field().Presence() == presence.Repeated {
		r := new(compilerpb.Value_Repeated)
		for elem := range seq.Values(v.Elements()) {
			r.Values = append(r.Values, element(elem))
		}
		return &compilerpb.Value{Value: &compilerpb.Value_Repeated_{Repeated: r}}
	}

	return element(v.Elements().At(0))
}
