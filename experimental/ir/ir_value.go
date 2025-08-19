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
	"iter"
	"math"

	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/ast/predeclared"
	"github.com/bufbuild/protocompile/experimental/internal"
	"github.com/bufbuild/protocompile/experimental/ir/presence"
	"github.com/bufbuild/protocompile/experimental/report"
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
	// results in a field a: {b: {c: 9}}, which is four rawValues deep.
	// Each of these will have the same optionPath, for a.b.c.
	optionPath ast.Path

	// The field that this value sets. This is where type information comes
	// from.
	//
	// NOTE: we steal the high bit of the pointer to indicate whether or not
	// bits refers to a slice. If the pointer part is negative, bits is a
	// repeated field with multiple elements.
	field ref[rawMember]
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

// key returns an AST node that best approximates where this value's field was
// set.
func (v Value) key() report.Spanner {
	if field := v.FieldAST(); !field.IsZero() {
		return field.Key()
	}

	return v.OptionPath()
}

// Field returns the field this value sets, which includes the value's type
// information.
//
// NOTE: [Member.Element] returns google.protobuf.Any, the concrete type of the
// values in [Value.Elements] may be distinct from it.
func (v Value) Field() Member {
	if v.IsZero() {
		return Member{}
	}

	field := v.raw.field
	if int32(field.ptr) < 0 {
		field.ptr = -field.ptr
	}
	return wrapMember(v.Context(), field)
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
	return seq.NewFixedSlice(v.getElements(), func(_ int, bits rawValueBits) Element {
		return Element{v.withContext, v.Field(), bits}
	})
}

// Outlined to promote inlining of Elements().
func (v Value) getElements() []rawValueBits {
	var slice []rawValueBits
	switch {
	case v.IsZero():
		break
	case int32(v.raw.field.ptr) < 0:
		slice = *v.slice()
	default:
		slice = slicesx.One(&v.raw.bits)
	}
	return slice
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
	if v.IsZero() {
		return MessageValue{}
	}

	m := v.Elements().At(0).AsMessage()

	if m.TypeURL() != "" {
		// If this is the concrete version of an Any message, it is effectively
		// singular: even if the reported field is a repeated g.p.Any, we treat
		// Any as having a singular "concrete" field that contains the actual
		// value (see [MessageValue.Concrete]).
		return m
	}

	if v.Field().Presence() == presence.Repeated {
		return MessageValue{}
	}
	return m
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
	v.raw.field.ptr = -v.raw.field.ptr
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
	field Member
	bits  rawValueBits
}

// Field returns the field this value sets, which includes the value's type
// information.
func (e Element) Field() Member {
	if e.IsZero() {
		return Member{}
	}

	return e.field
}

// Type returns the type of this element.
//
// Note that this may be distinct from [Member.Element]. In the case that this is
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
	// The [Value] this message corresponds to.
	self arena.Pointer[rawValue]

	// The type of this message. If concrete is not nil, this may be distinct
	// from AsValue().Field().Element().
	ty  ref[rawType]
	url intern.ID // The type URL for the above, if this is an Any.

	// If present, this is the concrete version of this value if it is an Any
	// constructed from a concrete type. This may itself be an Any with a
	// non-nil concrete, for the pathological value
	//
	//   any: { [types.com/google.protobuf.Any]: { [types.com/my.Type]: { ... } }}
	concrete arena.Pointer[rawMessageValue]

	// Fields set in this message in insertion order.
	entries []arena.Pointer[rawValue]

	// Which entries are already inserted. These are by interned full name
	// of either the field or its containing oneof.
	byName intern.Map[uint32]
}

// AsValue returns the [Value] corresponding to this message.
//
// This value can be used to retrieve the associated [Member] and from it the
// message's declared [Type].
func (v MessageValue) AsValue() Value {
	if v.IsZero() {
		return Value{}
	}
	return wrapValue(v.Context(), v.raw.self)
}

// Type returns this value's message type.
//
// If v was returned from [MessageValue.Concrete], its type need not be the
// same as v.AsValue()'s (although it can be, in the case of pathological
// Any-within-an-Any messages).
func (v MessageValue) Type() Type {
	if v.IsZero() {
		return Type{}
	}
	return wrapType(v.Context(), v.raw.ty)
}

// TypeURL returns this value's type URL, if it is the concrete value of an
// Any.
func (v MessageValue) TypeURL() string {
	if v.IsZero() {
		return ""
	}
	return v.Context().session.intern.Value(v.raw.url)
}

// Concrete returns the concrete version of this value if it is an Any.
//
// If it isn't an Any, or a "raw" Any (one not specified with the special type
// URL syntax), this returns v.
func (v MessageValue) Concrete() MessageValue {
	if v.IsZero() || v.raw.concrete.Nil() {
		return v
	}
	v.raw = v.Context().arenas.messages.Deref(v.raw.concrete)
	return v
}

// Fields yields the fields within this message literal, in insertion order.
func (v MessageValue) Fields() iter.Seq[Value] {
	return func(yield func(Value) bool) {
		for _, p := range v.raw.entries {
			v := wrapValue(v.Context(), p)
			if !v.IsZero() && !yield(v) {
				return
			}
		}
	}
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
func (v MessageValue) insert(field Member) *arena.Pointer[rawValue] {
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
type scalar interface {
	bool |
		int32 | uint32 | int64 | uint64 |
		float32 | float64 |
		intern.ID | string
}

// newZeroScalar constructs a new scalar value.
func newZeroScalar(c *Context, field ref[rawMember]) Value {
	return Value{
		internal.NewWith(c),
		c.arenas.values.New(rawValue{
			field: field,
		}),
	}
}

// appendRaw appends a scalar value to the given array value.
func appendRaw(array Value, bits rawValueBits) {
	slice := array.slice()
	*slice = append(*slice, bits)
}

// newScalar appends a new message value to the given array value, and returns it.
//
// If anyType is not zero, it will be used as the type of the inner message
// value. This is used for Any-typed fields. Otherwise, the type of field is
// used instead.
func appendMessage(array Value) MessageValue {
	message := array.Context().arenas.messages.New(rawMessageValue{
		self:   array.Context().arenas.values.Compress(array.raw),
		ty:     array.Field().raw.elem,
		byName: make(intern.Map[uint32]),
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
func newMessage(c *Context, field ref[rawMember]) MessageValue {
	msg := c.arenas.messages.New(rawMessageValue{
		ty:     wrapMember(c, field).raw.elem,
		byName: make(intern.Map[uint32]),
	})
	v := c.arenas.values.NewCompressed(rawValue{
		field: field,
		bits:  rawValueBits(c.arenas.messages.Compress(msg)),
	})
	msg.self = v
	return MessageValue{internal.NewWith(c), msg}
}

// newConcrete constructs a new value to be the concrete representation of
// v with the given type.
func newConcrete(m MessageValue, ty Type, url string) MessageValue {
	if !m.raw.concrete.Nil() {
		panic("protocompile/ir: set a concrete type more than once")
	}
	if !m.Type().IsAny() {
		panic("protocompile/ir: set concrete type on non-Any")
	}

	field := m.AsValue().raw.field
	if int32(field.ptr) < 0 {
		field.ptr = -field.ptr
	}

	msg := newMessage(m.Context(), field)
	msg.raw.ty = compressType(m.Context(), ty)
	msg.raw.url = m.Context().session.intern.Intern(url)
	m.raw.concrete = m.Context().arenas.messages.Compress(msg.raw)
	return msg
}

// newScalarBits converts a scalar into raw bits for storing in a [Value].
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
