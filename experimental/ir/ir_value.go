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

// Field returns the field this value sets, which includes the value's type
// information.
//
// NOTE: [Field.Element] returns google.protobuf.Any, the concrete type of the
// values in [Value.Elements] may be distinct from it.
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

// Singular returns whether this value is singular, i.e., [Value.Elements] will
// contain exactly one value.
func (v Value) Singular() bool {
	return v.Field().Presence() != presence.Repeated
}

// Elements returns an indexer over the elements within this value.
//
// If the value is not an array, it contains the singular element within;
// otherwise, it returns the elements of the array.
//
// The indexer will be nonempty except for the zero Value. That is to say, unset
// fields of [MessageValue]s are not represented as a distinct "empty" Value.
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
	if e.IsZero() {
		return Field{}
	}

	return e.field
}

// Type returns the type of this element.
//
// Note that this may be distinct from [Field.Element]. In the case that this is
// a google.protobuf.Any-typed field, this function will return the concrete
// type if known, rather than Any.
func (e Element) Type() Type {
	if msg := e.AsMessage(); !msg.IsZero() {
		// This will always be the concrete type, except in the case of
		// something naughty like my_any: { type_url: "...", value: "..." };
		// in that case this will be Any.
		return msg.Type()
	}

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
	// Avoid infinite recursion: Type() calls AsMessage().
	if !e.Field().Element().IsMessage() {
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
	// The concrete type of this message. This cannot be implicit from the
	// Field in a Value, because that might be Any.
	ty ref[rawType]

	// Fields set in this message in insertion order.
	entries []arena.Pointer[rawValue]

	// Which entries are already inserted. These are by field number, except for
	// elements of oneofs, which are by negative oneof index. This makes it
	// easy to check if any element of a oneof is already set.
	//
	//nolint:unused // Will be used by options lowering.
	byName intern.Map[uint32]
}

// Type returns this value's message type.
func (v MessageValue) Type() Type {
	return wrapType(v.Context(), v.raw.ty)
}

// Fields returns the fields within this message literal, in insertion order.
func (v MessageValue) Fields() seq.Indexer[Value] {
	return seq.NewFixedSlice(
		v.raw.entries,
		func(_ int, p arena.Pointer[rawValue]) Value {
			return wrapValue(v.Context(), p)
		},
	)
}

// insert adds a new field to this message value, returning a pointer to the
// corresponding entry in the entries array, which can be initialized as-needed.
//
// A conflict occurs if there is already a field with the same number or part of
// the same oneof in this value. To determine whether to diagnose as a duplicate
// field or duplicate oneof, simply compare the field number of entry to that
// of the duplicate. If they are different, they share a oneof.
//
// When a conflict occurs, the existing rawValue pointer will be returned,
// whereas if the value is being inserted for the first time, the returned arena
// pointer will be nil and can be initialized by the caller.
//
//nolint:unused // Will be used by options lowering.
func (v MessageValue) insert(field Field) *arena.Pointer[rawValue] {
	id := field.InternedFullName()
	if o := field.Oneof(); !o.IsZero() {
		id = o.InternedFullName()
	}

	n := len(v.raw.entries)
	if actual, ok := mapsx.Add(v.raw.byName, id, uint32(n)); !ok {
		return &v.raw.entries[actual]
	}

	v.raw.entries = append(v.raw.entries, 0)
	return slicesx.LastPointer(v.raw.entries)
}

// scalar is a type that can be converted into a [rawValueBits].
//
//nolint:unused // Will be used by options lowering.
type scalar interface {
	bool |
		int32 | uint32 | int64 | uint64 |
		float32 | float64 |
		intern.ID | string
}

// newScalar constructs a new scalar value.
//
//nolint:unused // Will be used by options lowering.
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
//
//nolint:unused // Will be used by options lowering.
func appendScalar[T scalar](array Value, v T) {
	slice := array.slice()
	*slice = append(*slice, newScalarBits(array.Context(), v))
}

// newScalar appends a new message value to the given array value, and returns it.
//
// If anyType is not zero, it will be used as the type of the inner message
// value. This is used for Any-typed fields. Otherwise, the type of field is
// used instead.
//
//nolint:unused // Will be used by options lowering.
func appendMessage(array Value, anyType ref[rawType]) MessageValue {
	if anyType.ptr.Nil() {
		anyType = array.Field().raw.elem
	}
	message := array.Context().arenas.messages.New(rawMessageValue{
		ty:     anyType,
		byName: make(intern.Map[uint32]),
	})

	slice := array.slice()
	*slice = append(*slice, rawValueBits(array.Context().arenas.messages.Compress(message)))

	return MessageValue{array.withContext, message}
}

// newMessage constructs a new message value.
//
//nolint:unused // Will be used by options lowering.
func newMessage(c *Context, field ref[rawField], anyType ref[rawType]) Value {
	if anyType.ptr.Nil() {
		anyType = wrapField(c, field).raw.elem
	}

	return Value{
		internal.NewWith(c),
		c.arenas.values.New(rawValue{
			field: field,
			bits: rawValueBits(c.arenas.messages.NewCompressed(rawMessageValue{
				ty:     anyType,
				byName: make(intern.Map[uint32]),
			})),
		}),
	}
}

// newScalarBits converts a scalar into raw bits for storing in a [Value].
//
//nolint:unused // Will be used by options lowering.
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
	default:
		panic("unreachable")
	}
}
