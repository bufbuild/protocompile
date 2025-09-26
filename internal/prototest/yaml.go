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

package prototest

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/protocolbuffers/protoscope"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/bufbuild/protocompile/experimental/dom"
	"github.com/bufbuild/protocompile/internal/ext/cmpx"
	"github.com/bufbuild/protocompile/internal/ext/iterx"
)

// ToYAMLOptions contains configuration for [ToYAML].
type ToYAMLOptions struct {
	// The maximum column width before wrapping starts to occur.
	MaxWidth int

	// The indentation string to use. If empty, defaults to "  ".
	Indent string
}

// ToYAML converts a Protobuf message into a YAML document in a deterministic
// manner. This is intended for generating YAML for golden outputs.
//
// The result will use a compressed representation where possible.
func ToYAML(m proto.Message, opts ToYAMLOptions) string {
	if opts.MaxWidth == 0 {
		opts.MaxWidth = 80
	}

	if len(opts.Indent) < 2 {
		opts.Indent = "  "
	}

	d := opts.message(m.ProtoReflect())
	d.prepare()

	return dom.Render(dom.Options{
		MaxWidth: opts.MaxWidth,
	}, func(push dom.Sink) { d.render(renderArgs{ToYAMLOptions: opts, root: true}, push) })
}

// message converts a Protobuf message into a [doc], which is used as an
// intermediate processing stage to help make formatting decisions
// (such as compressing nested messages).
func (y ToYAMLOptions) message(m protoreflect.Message) *doc {
	d := new(doc)
	entries := slices.Collect(iterx.Left(m.Range))
	slices.SortFunc(entries, cmpx.Join(
		cmpx.Map(protoreflect.FieldDescriptor.IsExtension, cmpx.Bool),
		cmpx.Key(protoreflect.FieldDescriptor.Index),
		cmpx.Key(protoreflect.FieldDescriptor.Number),
	))

	for _, f := range entries {
		y := y.value(m.Get(f), f)
		if f.IsExtension() {
			d.push("("+f.FullName()+")", y)
		} else {
			d.push(f.Name(), y)
		}
	}

	unknown := m.GetUnknown()
	if len(unknown) > 0 {
		d.pairs = append(d.pairs, [2]any{
			protoreflect.Name("$unknown"),
			protoscopeString(protoscope.Write(unknown, protoscope.WriterOptions{})),
		})
	}

	return d
}

// value converts a Protobuf value into a value that can be placed into a
// [doc].
func (y ToYAMLOptions) value(v protoreflect.Value, f protoreflect.FieldDescriptor) any {
	switch v := v.Interface().(type) {
	case protoreflect.Message:
		return y.message(v)

	case protoreflect.List:
		d := new(doc)
		for i := range v.Len() {
			d.push(nil, y.value(v.Get(i), f))
		}
		return d

	case protoreflect.Map:
		d := new(doc)
		d.needsSort = true
		v.Range(func(k protoreflect.MapKey, v protoreflect.Value) bool {
			d.push(
				y.value(k.Value(), f.MapKey()),
				y.value(v, f.MapValue()),
			)
			return true
		})
		return d

	case protoreflect.EnumNumber:
		enum := f.Enum()
		if value := enum.Values().ByNumber(v); value != nil {
			return value.Name()
		}
		return int32(v)

	case []byte:
		return string(v)

	default:
		return v
	}
}

// prepare prepares a document for printing by compressing elements as
// appropriate.
func (d *doc) prepare() {
	if d.needsSort {
		slices.SortFunc(d.pairs, func(a, b [2]any) int {
			return cmpx.Any(a[0], b[0])
		})
	}

	for i := range d.pairs {
		pair := &d.pairs[i]
		if v, ok := pair[1].(*doc); ok {
			v.prepare()
		}

		for {
			v, ok := pair[1].(*doc)
			if !ok || len(v.pairs) != 1 {
				break
			}

			outer, ok1 := pair[0].(protoreflect.Name)
			inner, ok2 := v.pairs[0][0].(protoreflect.Name)
			if !ok1 || !ok2 {
				break
			}

			//nolint:unconvert // Conversion below is included for readability.
			pair[0] = protoreflect.Name(outer + "." + inner)
			pair[1] = v.pairs[0][1]
		}
	}
}

type renderArgs struct {
	ToYAMLOptions

	root   bool
	inList bool
}

type protoscopeString string

func (d *doc) render(args renderArgs, push dom.Sink) {
	value := func(args renderArgs, v any, push dom.Sink) {
		switch v := v.(type) {
		case protoscopeString:
			{
				v := strings.TrimSpace(string(v))
				if strings.Contains(v, "\n") {
					push(
						dom.Text("|"), dom.Text("\n"),
						dom.Indent(args.Indent, func(push dom.Sink) {
							for chunk := range strings.SplitSeq(v, "\n") {
								push(dom.Text(chunk), dom.Text("\n"))
							}
						}),
					)
				} else {
					push(dom.Text(strconv.Quote(v)))
				}
			}
		case string:
			push(dom.Text(strconv.Quote(v)))
		case []byte:
			push(dom.Text(strconv.Quote(string(v))))
		case *doc:
			v.render(args, push)
		default:
			push(dom.Text(fmt.Sprint(v)))
		}
	}

	if d.isArray {
		push(dom.Group(0, func(push dom.Sink) {
			push(
				dom.TextIf(dom.Flat, "["),
				dom.TextIf(dom.Broken, "\n"),
				dom.Indent(args.Indent[2:], func(push dom.Sink) {
					for i, pair := range d.pairs {
						if i > 0 {
							push(dom.TextIf(dom.Flat, ","), dom.TextIf(dom.Flat, " "))
						}

						push(
							dom.TextIf(dom.Broken, "-"), dom.TextIf(dom.Broken, " "),
							dom.Indent("  ", func(push dom.Sink) {
								if v, ok := pair[1].(*doc); ok && len(v.pairs) == 1 {
									push(
										dom.GroupIf(dom.Broken, 0, func(push dom.Sink) {
											args := args
											args.root = false
											args.inList = false

											pair := v.pairs[0]
											value(args, pair[0], push)
											push(dom.Text(":"), dom.Text(" "))
											value(args, pair[1], push)
										}),
										dom.GroupIf(dom.Flat, 0, func(push dom.Sink) {
											args := args
											args.root = false
											args.inList = true
											value(args, pair[1], push)
										}),
									)
								} else {
									args := args
									args.root = false
									args.inList = true
									value(args, pair[1], push)
								}
								push(dom.TextIf(dom.Broken, "\n"))
							}))
					}
				}),
				dom.TextIf(dom.Flat, "]"))
		}))

		return
	}

	if len(d.pairs) == 0 {
		push(dom.Text("{}"))
		return
	}

	push(dom.Group(0, func(push dom.Sink) {
		if !args.root && !args.inList {
			push(dom.TextIf(dom.Broken, "\n"))
		}

		indent := args.Indent
		if args.root || args.inList {
			indent = ""
		}

		push(
			dom.TextIf(dom.Flat, "{"), dom.TextIf(dom.Flat, " "),
			dom.Indent(indent, func(push dom.Sink) {
				for i, pair := range d.pairs {
					if i > 0 {
						push(dom.TextIf(dom.Flat, ","), dom.TextIf(dom.Flat, " "))
					}

					args := args
					args.root = false
					args.inList = false

					value(args, pair[0], push)
					push(dom.Text(":"), dom.Text(" "))
					value(args, pair[1], push)

					push(dom.TextIf(dom.Broken, "\n"))
				}
			}),
			dom.TextIf(dom.Flat, " "), dom.TextIf(dom.Flat, "}"),
		)

		if args.root {
			push(dom.Text("\n"))
		}
	}))
}

// doc is a generic document structure used as an intermediate for generating
// the compressed output of ToYAML.
//
// It is composed of an array of pairs of arbitrary values.
type doc struct {
	pairs              [][2]any
	isArray, needsSort bool
}

// push adds a new entry to this document.
//
// All pushes entries must either have a non-nil key OR a nil key.
func (d *doc) push(k, v any) {
	if len(d.pairs) == 0 {
		d.isArray = k == nil
	} else if d.isArray != (k == nil) {
		panic("misuse of doc.push()")
	}

	d.pairs = append(d.pairs, [2]any{k, v})
}
