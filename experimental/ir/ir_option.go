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
