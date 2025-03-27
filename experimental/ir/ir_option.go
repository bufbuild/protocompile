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

package ir

import (
	"math"

	"google.golang.org/protobuf/encoding/protowire"

	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/ast/predeclared"
	"github.com/bufbuild/protocompile/experimental/internal"
	"github.com/bufbuild/protocompile/experimental/ir/presence"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/internal/arena"
	"github.com/bufbuild/protocompile/internal/ext/slicesx"
	"github.com/bufbuild/protocompile/internal/intern"
)

// Option is an option in a Protobuf file.
type Option struct {
	withContext
	raw *rawOption
}

type rawOption struct {
	ast ast.DeclDef

	// This is an alternating sequence of path parts, where the first is
	// a non-extension, the second is a fully-qualified extension path, and so
	// on.
	//
	// For example, foo.(bar.baz).bang.bonk is represented as
	// ["foo", "bar.baz", "bang", "", "bonk"]. The empty string is used as a
	// separator for adjacent components of the same type.
	//
	// In particular, (foo.bar).baz is represented as ["", "foo.bar", "baz"].
	//
	// The names of extensions therein are to be interpreted as absolute (i.e.,
	// fully-qualified), although they will not have leading dots.
	//
	// If this slice is nil, that means that name resolution has not happened
	// yet, and the ast node contains the partially-qualified extension names
	// to use for name resolution..
	name []intern.ID //nolint:unused // Will become used in a followup.

	field ref[rawField] // Set if this option matches a field.

	// TODO: it may be worth inlining small integer values in the common case?
	value rawValue
}

// Field returns the message field that this option corresponds to.
//
// This is primarily used for type-checking; the path of fields from the
// *Options descriptor at the root is not stored.
func (o Option) Field() Field {
	return wrapField(o.Context(), o.raw.field)
}

// Value returns the value this option is set to.
func (o Option) Value() Value {
	return Value{
		withContext: o.withContext,
		field:       wrapField(o.Context(), o.raw.field),
		ast:         o.raw.ast.Value(),
		raw:         o.raw.value,
	}
}

func wrapOption(c *Context, p arena.Pointer[rawOption]) Option {
	if c == nil || p.Nil() {
		return Option{}
	}
	return Option{
		withContext: internal.NewWith(c),
		raw:         c.arenas.options.Deref(p),
	}
}

// Value is an evaluated expression assigned to an [Option].
type Value struct {
	withContext
	field Field // The context of the field need not be the context of the value.
	ast   ast.ExprAny
	raw   rawValue
}

// rawValue is used to represent the actual value for all types, according to
// the following encoding:
//  1. All numeric types, including bool and enums. This holds the bits.
//  2. String and bytes. This holds an intern.ID.
//  3. Messages. This holds an arena.Pointer[rawMessage].
//  4. Repeated fields. This holds an arena.Pointer[[]rawValue], where each
//     value is interpreted as a non-array with this value's type.
//     This exploits the fact that arrays cannot contain other arrays.
//     Note that within the IR, map fields do not exist, and are represented as
//     the repeated message fields that they will ultimately become.
type rawValue uint64

// AST returns the expression that evaluated to this value.
func (v Value) AST() ast.ExprAny {
	return v.ast
}

// Type is the element type of this value. For arrays, this is the type of the
// element; Protobuf does not have a concept of an "array type".
func (v Value) Type() Type {
	elems := v.Elements()
	if elems.Len() == 0 {
		return Type{}
	}
	return elems.At(0).Type()
}

// Field returns the field this value sets, which includes the value's type
// information.
//
// This is the zero value if the field is uninterpreted.
func (v Value) Field() Field {
	return v.field
}

// Elements returns an indexer over the elements within this value.
//
// If the value is not an array, it contains the singular element within;
// otherwise, it returns the elements of the array.
func (v Value) Elements() seq.Indexer[Element] {
	slice := slicesx.One(&v.raw)
	if v.Field().Presence() == presence.Repeated {
		slice = *v.Context().arenas.arrays.Deref(arena.Pointer[[]rawValue](v.raw))
	}

	return seq.NewFixedSlice(slice, func(_ int, bits rawValue) Element {
		return Element{v.withContext, v.field, bits}
	})
}

// Element is an element within a [Value].
//
// This exists because array values contain multiple non-array elements; this
// type provides uniform access to such elements.
type Element struct {
	withContext
	field Field
	bits  rawValue
}

// Field returns the field this value sets, which includes the value's type
// information.
func (e Element) Field() Field {
	return e.field
}

// Type returns the type of this element.
func (e Element) Type() Type {
	return e.Field().Element()
}

// AsBool returns the bool value of this element.
//
// Returns ok == false if this is not a bool.
func (e Element) AsBool() (value, ok bool) {
	if e.Type().Predeclared() != predeclared.Bool {
		return false, false
	}
	return e.bits != 0, true
}

// AsUInt returns the value of this element as an unsigned integer.
//
// Returns false if this is not an unsigned integer.
func (e Element) AsUInt() (uint64, bool) {
	if !e.Type().Predeclared().IsUnsigned() {
		return 0, false
	}
	return uint64(e.bits), true
}

// AsInt returns the value of this element as a signed integer.
//
// Returns false if this is not a signed integer (enums are included as signed
// integers).
func (e Element) AsInt() (int64, bool) {
	if !e.Type().Predeclared().IsSigned() && !e.Type().IsEnum() {
		return 0, false
	}
	return int64(e.bits), true
}

// AsFloat returns the value of this element as a floating-point number.
//
// Returns false if this is not a float.
func (e Element) AsFloat() (float64, bool) {
	if !e.Type().Predeclared().IsFloat() {
		return 0, false
	}
	return math.Float64frombits(uint64(e.bits)), true
}

// AsString returns the value of this element as a string.
//
// Returns false if this is not a string.
func (e Element) AsString() (string, bool) {
	if !e.Type().Predeclared().IsString() {
		return "", false
	}
	return e.Context().session.intern.Value(intern.ID(e.bits)), true
}

// AsMessage returns the value of this element as a message literal.
//
// Returns the zero value if this is not a message.
func (e Element) AsMessage() MessageValue {
	if !e.Type().IsMessage() {
		return MessageValue{}
	}
	return MessageValue{
		e.withContext,
		e.Context().arenas.messages.Deref(arena.Pointer[rawMessageValue](e.bits)),
	}
}

// MessageValue is a message literal, represented as a list of ordered
// key-value pairs.
type MessageValue struct {
	withContext
	raw *rawMessageValue
}

type rawMessageValue struct {
	ast     ast.ExprDict
	ty      ref[rawType]
	entries []rawMessageValueEntry
}

type rawMessageValueEntry struct {
	field ref[rawField]
	value rawValue
}

// Type returns this value's message type.
func (v MessageValue) Type() Type {
	return wrapType(v.Context(), v.raw.ty)
}

// Fields returns the fields within this message literal.
func (v MessageValue) Fields() seq.Indexer[Value] {
	return seq.NewFixedSlice(
		v.raw.entries,
		func(i int, e rawMessageValueEntry) Value {
			return Value{
				withContext: v.withContext,
				field:       wrapField(v.Context(), e.field),
				ast:         v.raw.ast.Elements().At(i).Value(),
				raw:         e.value,
			}
		},
	)
}

// Marshal encodes a value as a Protobuf wire format record and writes
// it to buf.
//
// The record will include the number of the field this value is for.
func (v Value) Marshal(b []byte) []byte {
	if v.IsZero() || v.Elements().Len() == 0 {
		return b
	}

	n := protowire.Number(v.Field().Number())
	t := v.Type().Predeclared()
	if t == predeclared.Unknown && v.Type().IsEnum() {
		t = predeclared.Int32
	}

	switch t {
	case predeclared.Int32, predeclared.Int64,
		predeclared.UInt32, predeclared.UInt64,
		predeclared.SInt32, predeclared.SInt64, predeclared.Bool:
		zigzag := t == predeclared.SInt32 || t == predeclared.SInt64

		if v.Elements().Len() == 1 {
			x := uint64(v.Elements().At(0).bits)
			if zigzag {
				x = protowire.EncodeZigZag(int64(x))
			}

			b = protowire.AppendTag(b, n, protowire.VarintType)
			b = protowire.AppendVarint(b, x)
			break
		}

		var bytes int
		for e := range seq.Values(v.Elements()) {
			x := uint64(e.bits)
			if zigzag {
				x = protowire.EncodeZigZag(int64(x))
			}
			bytes += protowire.SizeVarint(x)
		}

		b = protowire.AppendTag(b, n, protowire.BytesType)
		b = protowire.AppendVarint(b, uint64(bytes))
		for e := range seq.Values(v.Elements()) {
			x := uint64(e.bits)
			if zigzag {
				x = protowire.EncodeZigZag(int64(x))
			}
			b = protowire.AppendVarint(b, x)
		}

	case predeclared.Fixed32, predeclared.SFixed32, predeclared.Float32:
		if v.Elements().Len() == 1 {
			e := v.Elements().At(0)
			b = protowire.AppendTag(b, n, protowire.Fixed32Type)
			b = protowire.AppendFixed32(b, uint32(e.bits))
			break
		}

		b = protowire.AppendTag(b, n, protowire.BytesType)
		b = protowire.AppendVarint(b, uint64(v.Elements().Len())*4)
		for e := range seq.Values(v.Elements()) {
			b = protowire.AppendFixed32(b, uint32(e.bits))
		}

	case predeclared.Fixed64, predeclared.SFixed64, predeclared.Float64:
		if v.Elements().Len() == 1 {
			e := v.Elements().At(0)
			b = protowire.AppendTag(b, n, protowire.Fixed32Type)
			b = protowire.AppendFixed64(b, uint64(e.bits))
			break
		}

		b = protowire.AppendTag(b, n, protowire.BytesType)
		b = protowire.AppendVarint(b, uint64(v.Elements().Len())*8)
		for e := range seq.Values(v.Elements()) {
			b = protowire.AppendFixed64(b, uint64(e.bits))
		}

	case predeclared.String, predeclared.Bytes:
		for e := range seq.Values(v.Elements()) {
			s, _ := e.AsString()
			b = protowire.AppendTag(b, n, protowire.Fixed32Type)
			b = protowire.AppendString(b, s)
		}

	default:
		// Is this value an any, i.e., was the AST of the form { [...]: {...} }?
		// This is the case if v's field is itself the magic Any type, but v
		// has a type that is *not* Any itself. This is necessary to deal with
		// the case where someone writes down an Any explicitly as
		// { type_url: ..., value: ... }.
		isAny := v.Field().Element().raw.isAny &&
			v.Field().Element() != v.Type()

		v.Type()
		for e := range seq.Values(v.Elements()) {
			b = protowire.AppendTag(b, n, protowire.BytesType)
			if !isAny {
				b = marshalMessage(b, func(b []byte) []byte {
					for f := range seq.Values(e.AsMessage().Fields()) {
						b = f.Marshal(b)
					}
					return b
				})
				continue
			}

			b = marshalMessage(b, func(b []byte) []byte {
				/*
					syntax = "proto3";
					message Any {
						string type_url = 1;
						bytes value = 2;
					}
				*/
				//nolint:stylecheck,revive // Trying to make the names below match their Protobuf names.
				const (
					Any_type_url = 1
					Any_value    = 2
				)

				b = protowire.AppendTag(b, Any_type_url, protowire.BytesType)

				// A valid, type-checked Any will always be a dict with one
				// element. Its key will be an array with one element.
				entry := v.AST().AsDict().Elements().At(0)
				url := entry.Key().AsArray().Elements().At(0).AsPath()
				b = protowire.AppendString(b, url.Canonicalized())

				b = protowire.AppendTag(b, Any_value, protowire.BytesType)
				b = marshalMessage(b, func(b []byte) []byte {
					for f := range seq.Values(e.AsMessage().Fields()) {
						b = f.Marshal(b)
					}
					return b
				})
				return b
			})
		}
	}

	return b
}

func marshalMessage(b []byte, cb func([]byte) []byte) []byte {
	// TODO: Group encoding.
	prefixAt := len(b)
	// Messages can only be 2GB at most so we can pre-allocate five
	// bytes for the length prefix and then always use an over-long
	// prefix.
	b = append(b, 0x80, 0x80, 0x80, 0x80, 0x00)
	start := len(b)
	b = cb(b)
	total := len(b) - start
	if total > math.MaxInt32 {
		// File size limits mean this error will be diagnosed long
		// before we reach this panic.
		panic("protocompile/ir: message value for option is too large")
	}
	protowire.AppendVarint(b[prefixAt:], uint64(total))
	for i := range 4 {
		b[prefixAt+i] |= 0x80
	}
	return b
}
