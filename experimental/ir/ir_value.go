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
	"github.com/bufbuild/protocompile/internal/ext/mapsx"
	"github.com/bufbuild/protocompile/internal/ext/slicesx"
	"github.com/bufbuild/protocompile/internal/intern"
)

// This silences unused function lints.
//
// These functions will be used by options lowering, which is not yet
// implemented, but are included here now because they are an essential part
// of the data structures in this file.
//
// They are all together here so it's easier to remember to delete them at the
// same time, instead of putting //nolint on each function.
//
// TODO: Delete this.
var _ = []any{
	newScalar[int32], appendScalar[int32],
	newMessage, appendMessage,
	MessageValue.insert,
}

// Value is an evaluated expression, corresponding to an option in a Protobuf
// file.
type Value struct {
	withContext
	raw *rawValue
}

// rawValue is a [rawValueBits] with field information attached to it.
type rawValue struct {
	// The expression that this value was evaluated from.
	//
	// If this is an entry in a [MessageValue], this will be an ast.FieldExpr.
	expr ast.ExprAny

	// The AST node for the path of the option (compact or otherwise) that
	// specifies this value. This is intended for diagnostics.
	//
	// For example, the node
	//
	//  option a.b.c = 9;
	//
	// results in a field a: {b: {c: 9}}, which is four rawFieldValues deep.
	// Each of these will have the same optionPath, for a.b.c.
	optionPath ast.Path

	// The field that this value sets. This is where type information comes
	// from.
	//
	// NOTE: we steal the high bit of the pointer to indicate whether or not
	// bits refers to a slice. If the pointer part is negative, bits is a
	// repeated field with multiple elements.
	field ref[rawField]
	bits  rawValueBits
}

// rawValueBits is used to represent the actual value for all types, according to
// the following encoding:
//  1. All numeric types, including bool and enums. This holds the bits.
//  2. String and bytes. This holds an intern.ID.
//  3. Messages. This holds an arena.Pointer[rawMessage].
//  4. Repeated fields with two or more entries. This holds an
//     arena.Pointer[[]rawValueBits], where each value is interpreted as a
//     non-array with this value's type.
//     This exploits the fact that arrays cannot contain other arrays.
//     Note that within the IR, map fields do not exist, and are represented as
//     the repeated message fields that they will ultimately become.
type rawValueBits uint64

// AST returns the expression that evaluated to this value.
func (v Value) AST() ast.ExprAny {
	if v.IsZero() {
		return ast.ExprAny{}
	}

	if field := v.raw.expr.AsField(); field.IsZero() {
		return field.Value()
	}

	return v.raw.expr
}

// OptionPath returns the AST node for the path of the option (compact or not)
// that caused this value to be set.
func (v Value) OptionPath() ast.Path {
	if v.IsZero() {
		return ast.Path{}
	}
	return v.raw.optionPath
}

// FieldAST returns the field expression that sets this value in a message
// expression, if it is such a value.
func (v Value) FieldAST() ast.ExprField {
	if v.IsZero() {
		return ast.ExprField{}
	}

	return v.raw.expr.AsField()
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
	if v.IsZero() {
		return Field{}
	}

	field := v.raw.field
	if int32(field.ptr) < 0 {
		field.ptr = -field.ptr
	}
	return wrapField(v.Context(), v.raw.field)
}

// Elements returns an indexer over the elements within this value.
//
// If the value is not an array, it contains the singular element within;
// otherwise, it returns the elements of the array.
func (v Value) Elements() seq.Indexer[Element] {
	var slice []rawValueBits
	switch {
	case v.IsZero():
		break
	case int32(v.raw.field.ptr) < 0:
		slice = *v.slice()
	default:
		slice = slicesx.One(&v.raw.bits)
	}

	return seq.NewFixedSlice(slice, func(_ int, bits rawValueBits) Element {
		return Element{v.withContext, v.Field(), bits}
	})
}

// AsBool is a shortcut for [Element.AsBool], if this value is singular.
func (v Value) AsBool() (value, ok bool) {
	if v.IsZero() || v.Field().Presence() == presence.Repeated {
		return false, false
	}
	return v.Elements().At(0).AsBool()
}

// AsUInt is a shortcut for [Element.AsUInt], if this value is singular.
func (v Value) AsUInt() (uint64, bool) {
	if v.IsZero() || v.Field().Presence() == presence.Repeated {
		return 0, false
	}
	return v.Elements().At(0).AsUInt()
}

// AsInt is a shortcut for [Element.AsUnt], if this value is singular.
func (v Value) AsInt() (int64, bool) {
	if v.IsZero() || v.Field().Presence() == presence.Repeated {
		return 0, false
	}
	return v.Elements().At(0).AsInt()
}

// AsFloat is a shortcut for [Element.AsFloat], if this value is singular.
func (v Value) AsFloat() (float64, bool) {
	if v.IsZero() || v.Field().Presence() == presence.Repeated {
		return 0, false
	}
	return v.Elements().At(0).AsFloat()
}

// AsString is a shortcut for [Element.AsString], if this value is singular.
func (v Value) AsString() (string, bool) {
	if v.IsZero() || v.Field().Presence() == presence.Repeated {
		return "", false
	}
	return v.Elements().At(0).AsString()
}

// AsMessage is a shortcut for [Element.AsMessage], if this value is singular.
func (v Value) AsMessage() MessageValue {
	if v.IsZero() || v.Field().Presence() == presence.Repeated {
		return MessageValue{}
	}
	return v.Elements().At(0).AsMessage()
}

// slice returns the underlying slice for this value.
//
// If this value isn't already in slice form, this puts it into it.
func (v Value) slice() *[]rawValueBits {
	if int32(v.raw.field.ptr) < 0 {
		return v.Context().arenas.arrays.Deref(arena.Pointer[[]rawValueBits](v.raw.bits))
	}

	slice := v.Context().arenas.arrays.New([]rawValueBits{v.raw.bits})
	v.raw.bits = rawValueBits(v.Context().arenas.arrays.Compress(slice))
	return slice
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

// Element is an element within a [Value].
//
// This exists because array values contain multiple non-array elements; this
// type provides uniform access to such elements. See [Value.Elements].
type Element struct {
	withContext
	field Field
	bits  rawValueBits
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
	ty      ref[rawType]
	entries []arena.Pointer[rawValue]

	// Which entries are already inserted. These are by field number, except for
	// elements of oneofs, which are by negative oneof index. This makes it
	// easy to check if any element of a oneof is already set.
	byNumber map[int32]uint32
}

// Type returns this value's message type.
func (v MessageValue) Type() Type {
	return wrapType(v.Context(), v.raw.ty)
}

// Fields returns the fields within this message literal.
func (v MessageValue) Fields() seq.Indexer[Value] {
	return seq.NewFixedSlice(
		v.raw.entries,
		func(_ int, p arena.Pointer[rawValue]) Value {
			return wrapValue(v.Context(), p)
		},
	)
}

// insert adds a new field to this message value.
//
// A conflict occurs if there is already a field with the same number or part of
// the same oneof in this value. To determine whether to diagnose as a duplicate
// field or duplicate oneof, simply compare the field number of entry to that
// of the duplicate. If they are different, they share a oneof.
func (v MessageValue) insert(entry Value) (idx int, inserted bool) {
	number := entry.Field().Number()
	if o := entry.Field().Oneof(); !o.IsZero() {
		number = -int32(o.Index())
	}

	n := len(v.raw.entries)
	if actual, ok := mapsx.Add(v.raw.byNumber, number, uint32(n)); !ok {
		return int(actual), false
	}

	v.raw.entries = append(v.raw.entries, v.Context().arenas.values.Compress(entry.raw))
	return n, true
}

// scalar is a type that can be converted into a rawValueBits.
type scalar interface {
	bool |
		int32 | uint32 | int64 | uint64 |
		float32 | float64 |
		intern.ID | string
}

// newScalar constructs a new scalar value.
func newScalar[T scalar](c *Context, field ref[rawField], v T) Value {
	return Value{
		internal.NewWith(c),
		c.arenas.values.New(rawValue{
			field: field,
			bits:  newScalarBits(c, v),
		}),
	}
}

// newScalar appends a scalar value to the given array value.
func appendScalar[T scalar](array Value, v T) {
	slice := array.slice()
	*slice = append(*slice, newScalarBits(array.Context(), v))
}

// newScalar appends a new message value to the given array value, and returns it.
//
// anyType is as in [newMessage].
func appendMessage(array Value, anyType ref[rawType]) MessageValue {
	if anyType.ptr.Nil() {
		anyType = array.Field().raw.elem
	}
	message := array.Context().arenas.messages.New(rawMessageValue{
		ty:       anyType,
		byNumber: make(map[int32]uint32),
	})

	slice := array.slice()
	*slice = append(*slice, rawValueBits(array.Context().arenas.messages.Compress(message)))

	return MessageValue{array.withContext, message}
}

// newMessage constructs a new message value.
//
// If anyType is not zero, it will be used as the type of the inner message
// value. This is used for Any-typed fields. Otherwise, the type of field is
// used instead.
func newMessage(c *Context, field ref[rawField], anyType ref[rawType]) Value {
	if anyType.ptr.Nil() {
		anyType = wrapField(c, field).raw.elem
	}

	return Value{
		internal.NewWith(c),
		c.arenas.values.New(rawValue{
			field: field,
			bits: rawValueBits(c.arenas.messages.NewCompressed(rawMessageValue{
				ty:       anyType,
				byNumber: make(map[int32]uint32),
			})),
		}),
	}
}

func newScalarBits[T scalar](c *Context, v T) rawValueBits {
	switch v := any(v).(type) {
	case bool:
		if v {
			return 1
		}
		return 0

	case int32:
		return rawValueBits(v)
	case uint32:
		return rawValueBits(v)
	case int64:
		return rawValueBits(v)
	case uint64:
		return rawValueBits(v)

	case float32:
		// All float values are stored as binary64. All binary32 floats can be
		// losslessly encoded as binary64, so this conversion does not result
		// in precision loss.
		return rawValueBits(math.Float64bits(float64(v)))
	case float64:
		return rawValueBits(math.Float64bits(v))

	case intern.ID:
		return rawValueBits(v)
	case string:
		return rawValueBits(c.session.intern.Intern(v))
	}

	return 0
}
