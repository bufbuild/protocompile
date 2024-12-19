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
	// This is an alternating sequence of path parts, where the first is
	// a non-extension, the second is an extension, and so on.
	//
	// For example, foo.(bar.baz).bang.bonk is represented as
	// ["foo", "bar.baz", "bang", "", "bonk"]. The empty string is used as a
	// separator for adjacent components of the same type.
	name []intern.ID //nolint:unused // Will become used in a followup.

	field ref[rawField] // Set if this option matches a field.

	// TODO: it may be worth inlining small integer values in the common case?
	value arena.Pointer[rawValue]
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
	return wrapValue(o.Context(), o.raw.value)
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
	raw *rawValue
}

type rawValue struct {
	ast  ast.ExprAny
	bits rawValueBits
	ty   predeclared.Name

	isArray                  bool // If set, value is an arena.Pointer[[]rawValueBits].
	isUninterpretedPath      bool // If set, value is an intern.ID.
	isUninterpretedAggregate bool // If set, value is an intern.ID.
}

// rawValueBits is used to represent the actual value for all types, according to
// the following encoding:
//  1. All numeric types, including bool and enums. This holds the bits.
//  2. String and bytes. This holds an intern.ID.
//  3. Messages. This holds an arena.Pointer[rawMessage].
//  4. Repeated fields. This holds an arena.Pointer[[]rawValueBits], where each
//     value is interpreted as a non-array with this value's type.
//     This exploits the fact that arrays cannot contain other arrays.
type rawValueBits uint64

// AST returns the expression that evaluated to this value.
func (v Value) AST() ast.ExprAny {
	return v.raw.ast
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

// Elements returns an indexer over the elements within this value.
//
// If the value is not an array, it contains the singular element within;
// otherwise, it returns the elements of the array.
//
// If this is an uninterpreted option, returns an empty indexer.
func (v Value) Elements() seq.Indexer[Element] {
	var slice []rawValueBits
	switch {
	case v.raw.isUninterpretedPath || v.raw.isUninterpretedAggregate:
		slice = nil
	case v.raw.isArray:
		slice = *v.Context().arenas.arrays.Deref(arena.Pointer[[]rawValueBits](v.raw.bits))
	default:
		slice = slicesx.One(&v.raw.bits)
	}

	return seq.Slice[Element, rawValueBits]{
		Slice: slice,
		Wrap: func(bits *rawValueBits) Element {
			return Element{v.withContext, *bits, v.raw.ty}
		},
	}
}

// Element is an element within a [Value].
//
// This exists because array values contain multiple non-array elements; this
// type provides uniform access to such elements.
type Element struct {
	withContext
	bits rawValueBits
	// Unknown means this is a message type.
	// Enum types are predeclared.Int32.
	ty predeclared.Name
}

// Type returns the type of this element.
func (e Element) Type() Type {
	switch {
	case e.IsZero():
		return Type{}
	case e.ty == predeclared.Unknown:
		return e.AsMessage().Type()
	default:
		return PredeclaredType(e.ty)
	}
}

// AsBool returns the bool value of this element.
//
// Returns ok == false if this is not a bool.
func (e Element) AsBool() (value, ok bool) {
	if e.ty != predeclared.Bool {
		return false, false
	}
	return e.bits != 0, true
}

// AsUInt returns the value of this element as an unsigned integer.
//
// Returns false if this is not an unsigned integer.
func (e Element) AsUInt() (uint64, bool) {
	if !e.ty.IsUnsigned() {
		return 0, false
	}
	return uint64(e.bits), true
}

// AsInt returns the value of this element as a signed integer.
//
// Returns false if this is not a signed integer.
func (e Element) AsInt() (int64, bool) {
	if !e.ty.IsSigned() {
		return 0, false
	}
	return int64(e.bits), true
}

// AsFloat returns the value of this element as a floating-point number.
//
// Returns false if this is not a float.
func (e Element) AsFloat() (float64, bool) {
	if !e.ty.IsFloat() {
		return 0, false
	}
	return math.Float64frombits(uint64(e.bits)), true
}

// AsString returns the value of this element as a string.
//
// Returns false if this is not a string.
func (e Element) AsString() (string, bool) {
	if !e.ty.IsString() {
		return "", false
	}
	return e.Context().intern.Value(intern.ID(e.bits)), true
}

// AsMessage returns the value of this element as a message literal.
//
// Returns the zero value if this is not a message.
func (e Element) AsMessage() MessageValue {
	if e.ty != predeclared.Unknown {
		return MessageValue{}
	}
	return MessageValue{
		e.withContext,
		e.Context().arenas.messages.Deref(arena.Pointer[rawMessageValue](e.bits)),
	}
}

func wrapValue(c *Context, p arena.Pointer[rawValue]) Value {
	if c == nil || p.Nil() {
		return Value{}
	}

	return Value{
		withContext: internal.NewWith(c),
		raw:         c.arenas.values.Deref(p),
	}
}

// MessageValue is a message literal, represented as a list of ordered
// key-value pairs.
type MessageValue struct {
	withContext
	raw *rawMessageValue
}

type rawMessageValue struct {
	ty      ref[rawType]
	entries []rawMessageValueEntry
}

type rawMessageValueEntry struct {
	key   intern.ID
	field int32 // Index of the field within rawMessageValue.ty; -1 if unknown.
	value arena.Pointer[rawValue]
}

// Type returns this value's message type.
func (v MessageValue) Type() Type {
	return wrapType(v.Context(), v.raw.ty)
}

// Fields returns the fields within this message literal.
func (v MessageValue) Fields() seq.Indexer[FieldValue] {
	ty := v.Type()
	return seq.Slice[FieldValue, rawMessageValueEntry]{
		Slice: v.raw.entries,
		Wrap: func(e *rawMessageValueEntry) FieldValue {
			field := FieldValue{
				withContext: v.withContext,
				key:         e.key,
				value:       v.Context().arenas.values.Deref(e.value),
			}
			if e.field >= 0 {
				field.field = ty.Fields().At(int(e.field))
			}
			return field
		},
	}
}

// FieldValue is an entry within a [MessageValue].
type FieldValue struct {
	withContext
	key   intern.ID
	field Field // The context of the field need not be the context of the value.
	value *rawValue
}

// Name returns this field's name.
func (v FieldValue) Name() string {
	return v.Context().intern.Value(v.key)
}

// InternedName returns the intern ID for this field's name.
func (v FieldValue) InternedName() intern.ID {
	return v.key
}

// Field returns the field this field value corresponds to, if known.
func (v FieldValue) Field() Field {
	return v.field
}

// Value returns the value of this field.
func (v FieldValue) Value() Value {
	if v.IsZero() {
		return Value{}
	}
	return Value{v.withContext, v.value}
}
