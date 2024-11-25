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

package prototest

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

const license = `# Copyright 2020-2024 Buf Technologies, Inc.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

`

// ToYAMLOptions contains configuration for [ToYAML].
type ToYAMLOptions struct {
	// If set, zero values of implicit presence fields are set.
	EmitZeros bool

	// The maximum column width before wrapping starts to occur.
	MaxWidth int
}

// ToYAML converts a Protobuf message into a YAML document in a deterministic
// manner. This is intended for generating YAML for golden outputs.
//
// The result will use a compressed representation where possible.
func ToYAML(m proto.Message, opts *ToYAMLOptions) string {
	if opts == nil {
		opts = new(ToYAMLOptions)
	}
	y := &toYAML{
		ToYAMLOptions: opts,
	}
	d := y.message(m.ProtoReflect())
	d.compress()
	y.out.WriteString(license)
	y.write(d)
	return y.out.String()
}

// toYAML is state of an on-going YAML conversion.
type toYAML struct {
	*ToYAMLOptions

	out     strings.Builder
	nesting int
}

// message converts a Protobuf message into a [doc], which is used as an
// intermediate processing stage to help compact make formatting decisions
// (such as compressing nested messages).
func (y *toYAML) message(m protoreflect.Message) *doc {
	desc := m.Descriptor()
	fs := desc.Fields()

	d := new(doc)
	for i := 0; i < fs.Len(); i++ {
		f := fs.Get(i)

		has := m.Has(f)
		if y.EmitZeros && !has && !f.HasPresence() {
			has = true
		}
		if !has {
			continue
		}

		d.push(
			f.Name(),
			y.value(m.Get(f), f),
		)
	}
	return d
}

// value converts a Protobuf value into a value that can be placed into a
// [doc].
func (y *toYAML) value(v protoreflect.Value, f protoreflect.FieldDescriptor) any {
	switch v := v.Interface().(type) {
	case protoreflect.Message:
		return y.message(v)

	case protoreflect.List:
		d := new(doc)
		for i := 0; i < v.Len(); i++ {
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
		} else {
			return int32(v)
		}

	case []byte:
		return string(v)

	default:
		return v
	}
}

// write writes a value returned by [toYAML.value] into the internal output
// buffer.
func (y *toYAML) write(v any) {
	switch v := v.(type) {
	case int32, int64, uint32, uint64, float32, float64, protoreflect.Name:
		fmt.Fprint(&y.out, v)
	case string:
		fmt.Fprintf(&y.out, "%q", v)
	case *doc:
		isOneLine := func(v any) bool {
			maxWidth := y.MaxWidth
			if maxWidth == 0 {
				maxWidth = 80
			}
			maxWidth -= y.nesting * 2

			doc, ok := v.(*doc)
			return !ok || doc.width < maxWidth
		}

		if isOneLine(v) {
			switch {
			case v.isArray:
				y.out.WriteString("[")
				for i, pair := range v.pairs {
					if i > 0 {
						y.out.WriteString(", ")
					}
					y.write(pair[1])
				}
				y.out.WriteString("]")

			case len(v.pairs) == 0:
				y.out.WriteString("{}")

			default:
				// Special case: if we are a list element, and there is only
				// one entry, print it directly.
				if len(v.pairs) == 1 && strings.HasSuffix(y.out.String(), "- ") {
					y.write(v.pairs[0][0])
					y.out.WriteString(": ")
					y.write(v.pairs[0][1])
					return
				}

				y.out.WriteString("{ ")
				for i, pair := range v.pairs {
					if i > 0 {
						y.out.WriteString(", ")
					}
					y.write(pair[0])
					y.out.WriteString(": ")
					y.write(pair[1])
				}
				y.out.WriteString(" }")
			}
			return
		}

		for _, pair := range v.pairs {
			oneLine := isOneLine(pair[1])
			y.indent()

			if pair[0] == nil {
				y.out.WriteString("- ")
			} else {
				y.write(pair[0])
				if !oneLine {
					y.out.WriteString(":\n")
				} else {
					y.out.WriteString(": ")
				}
			}

			if !oneLine {
				y.nesting++
			}

			y.write(pair[1])
			if !oneLine {
				y.nesting--
			} else {
				y.out.WriteString("\n")
			}
		}
	}
}

// indent appends indentation if necessary.
func (y *toYAML) indent() {
	s := y.out.String()
	if s == "" || strings.HasSuffix(s, "\n") {
		for i := 0; i < y.nesting; i++ {
			y.out.WriteString("  ")
		}
	}
}

// doc is a generic document structure used as an intermediate for generating
// the compressed output of ToYAML.
//
// It is composed of an array of pairs of arbitrary values.
type doc struct {
	pairs [][2]any

	width              int
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

// compress prepares a document for printing by compressing elements as
// appropriate.
func (d *doc) compress() {
	if d.needsSort {
		slices.SortFunc(d.pairs, cmpMapKeys)
	}

	if d.isArray || len(d.pairs) == 0 {
		d.width = 2 // Accounts for [] or an empty {}.
	} else {
		d.width = 4 // Accounts for the { ... } delimiters.
	}

	for i := range d.pairs {
		pair := &d.pairs[i]
		if pair[0] != nil {
			// The 2 accounts for the ": " token.
			d.width += len(fmt.Sprint(pair[0])) + 2
		}

		if i > 0 {
			d.width += 2 // Accounts for the ", "
		}

		switch v := pair[1].(type) {
		case int32, int64, uint32, uint64, float32, float64, protoreflect.Name, string:
			d.width += len(fmt.Sprint(v))
		case *doc:
			v.compress()
			d.width += v.width

			if len(v.pairs) == 1 {
				outer, ok1 := pair[0].(protoreflect.Name)
				inner, ok2 := v.pairs[0][0].(protoreflect.Name)
				if ok1 && ok2 {
					//nolint:unconvert // Conversion below is included for readability.
					pair[0] = protoreflect.Name(outer + "." + inner)
					pair[1] = v.pairs[0][1]
				}
			}
		}
	}
}

func cmpMapKeys(a, b [2]any) int {
	// key is a concrete comparable type that is useful for forcing relative
	// order between types.
	type key struct {
		which int
		bool
		int64
		uint64
		string
	}

	any2key := func(v any) key {
		switch v := v.(type) {
		case bool:
			return key{which: 0, bool: v}
		case int32:
			return key{which: 1, int64: int64(v)}
		case int64:
			return key{which: 1, int64: v}
		case uint32:
			return key{which: 1, int64: int64(v)}
		case uint64:
			if v <= math.MaxInt64 {
				return key{which: 1, int64: int64(v)}
			}
			return key{which: 2, uint64: v}
		case protoreflect.Name:
			return key{which: 3, string: string(v)}
		case string:
			return key{which: 3, string: v}
		default:
			return key{}
		}
	}

	ak := any2key(b)
	bk := any2key(a)
	if n := cmp.Compare(ak.which, bk.which); n != 0 {
		return n
	}

	switch ak.which {
	case 0:
		if !ak.bool {
			return -1
		}
		return 1
	case 1:
		return cmp.Compare(ak.int64, bk.int64)
	case 2:
		return cmp.Compare(ak.uint64, bk.uint64)
	case 3:
		return cmp.Compare(ak.string, bk.string)
	default:
		return 0
	}
}
