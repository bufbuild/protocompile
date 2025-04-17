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
	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/internal"
	"github.com/bufbuild/protocompile/experimental/ir/presence"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/internal/arena"
	"github.com/bufbuild/protocompile/internal/intern"
)

// Field is a Protobuf message field, enum value, or extension field.
//
// A field has three types associated with it. The English language struggles
// to give these succinct names, so we review them here.
//
//  1. Its _element_, i.e. the type it contains. This is the type that a field is
//     declared to be _of_.
//
//  2. Its _parent_, i.e., the type it is syntactically defined within.
//     Extensions appear syntactically within their parent.
//
//  3. Its _container_, i.e., the type which it is part of for the purposes of
//     serialization. Extensions are fields of their container, but are declared
//     within their parent.
type Field struct {
	withContext
	raw *rawField
}

type rawField struct {
	def        ast.DeclDef
	options    arena.Pointer[rawValue]
	elem, extn ref[rawType]
	fqn, name  intern.ID
	number     int32
	parent     arena.Pointer[rawType]

	// If nonpositive, this is the negative of a presence.Kind. Otherwise, it's
	// a oneof index.
	oneof int32
}

// Returns whether this is a non-extension message field.
func (f Field) IsMessageField() bool {
	return !f.raw.elem.ptr.Nil() && f.raw.extn.ptr.Nil()
}

// Returns whether this is a extension message field.
func (f Field) IsExtension() bool {
	return !f.raw.elem.ptr.Nil() && !f.raw.extn.ptr.Nil()
}

// Returns whether this is an enum value.
func (f Field) IsEnumValue() bool {
	return f.raw.elem.ptr.Nil()
}

// AST returns the declaration for this field, if known.
func (f Field) AST() ast.DeclDef {
	return f.raw.def
}

// FullName returns this fields's name.
func (f Field) Name() string {
	if f.IsZero() {
		return ""
	}
	return f.Context().session.intern.Value(f.raw.name)
}

// FullName returns this fields's fully-qualified name.
func (f Field) FullName() FullName {
	if f.IsZero() {
		return ""
	}
	return FullName(f.Context().session.intern.Value(f.raw.fqn))
}

// InternedName returns the intern ID for [Field.FullName]().Name().
func (f Field) InternedName() intern.ID {
	if f.IsZero() {
		return 0
	}
	return f.raw.name
}

// InternedName returns the intern ID for [Field.FullName].
func (f Field) InternedFullName() intern.ID {
	if f.IsZero() {
		return 0
	}
	return f.raw.fqn
}

// Number returns the number for this field after expression evaluation.
//
// Defaults to zero if the number is not specified.
func (f Field) Number() int32 {
	return f.raw.number
}

// Presence returns this field's presence kind.
func (f Field) Presence() presence.Kind {
	if f.IsZero() {
		return presence.Unknown
	}
	if f.raw.oneof > 0 {
		return presence.Shared
	}
	return presence.Kind(-f.raw.oneof)
}

// Parent returns the type this field is syntactically located in. This is the
// type it is declared *in*, but which it is not necessarily part of.
//
// May be zero for extensions declared at the top level.
func (f Field) Parent() Type {
	return wrapType(f.Context(), ref[rawType]{ptr: f.raw.parent})
}

// Element returns the this field's element type. This is the type it is
// declared to be *of*, such as in the phrase "a string field's type is string".
//
// This does not include the field's presence: for example, a repeated int32
// field will report the type as being the int32 primitive, not an int32 array.
//
// This is zero for enum values.
func (f Field) Element() Type {
	return wrapType(f.Context(), f.raw.elem)
}

// Container returns the type which contains this field: this is either
// [Field.Parent], or the extendee if this is an extension. This is the
// type it is declared to be *part of*.
func (f Field) Container() Type {
	if f.raw.extn.ptr.Nil() {
		return f.Parent()
	}

	return wrapType(f.Context(), f.raw.extn)
}

// Oneof returns the oneof that this field is a member of.
//
// Returns the zero value if this field does not have [presence.Shared].
func (f Field) Oneof() Oneof {
	if f.Presence() != presence.Shared {
		return Oneof{}
	}
	return Oneof{
		f.withContext,
		int(f.raw.oneof),
		f.Parent().raw, // Extension fields are not part of oneofs.
	}
}

// Options returns the options applied to this field.
func (f Field) Options() MessageValue {
	return wrapValue(f.Context(), f.raw.options).AsMessage()
}

func wrapField(c *Context, r ref[rawField]) Field {
	if r.ptr.Nil() || c == nil {
		return Field{}
	}

	file := c.File()
	if r.file > 0 {
		file = c.imports.files[r.file-1]
	}

	return Field{
		withContext: internal.NewWith(file.Context()),
		raw:         file.Context().arenas.fields.Deref(r.ptr),
	}
}

// Oneof represents a oneof within a message definition.
type Oneof struct {
	withContext
	index     int
	container *rawType
}

type rawOneof struct {
	def       ast.DeclDef
	fqn, name intern.ID
	members   []arena.Pointer[rawField]
	options   arena.Pointer[rawValue]
}

// AST returns the declaration for this oneof, if known.
func (o Oneof) AST() ast.DeclDef {
	return o.raw().def
}

// Name returns this oneof's declared name.
func (o Oneof) Name() string {
	if o.IsZero() {
		return ""
	}
	return o.Context().session.intern.Value(o.raw().name)
}

// FullName returns this oneof's fully-qualified name.
func (o Oneof) FullName() FullName {
	if o.IsZero() {
		return ""
	}
	return FullName(o.Context().session.intern.Value(o.raw().fqn))
}

// InternedName returns the intern ID for [Oneof.FullName]().Name().
func (o Oneof) InternedName() intern.ID {
	if o.IsZero() {
		return 0
	}
	return o.raw().name
}

// InternedName returns the intern ID for [Oneof.FullName].
func (o Oneof) InternedFullName() intern.ID {
	if o.IsZero() {
		return 0
	}
	return o.raw().fqn
}

// Container returns the message type which contains it.
func (o Oneof) Container() Type {
	if o.IsZero() {
		return Type{}
	}

	return Type{o.withContext, o.container}
}

// Index returns this oneof's index in its containing message.
func (o Oneof) Index() int {
	if o.IsZero() {
		return 0
	}
	return o.index
}

// Members returns this oneof's member fields.
func (o Oneof) Members() seq.Indexer[Field] {
	return seq.NewFixedSlice(
		o.raw().members,
		func(_ int, p arena.Pointer[rawField]) Field {
			return wrapField(o.Context(), ref[rawField]{ptr: p})
		},
	)
}

// Parent returns the type that this oneof is declared within,.
func (o Oneof) Parent() Type {
	// Empty oneofs are not permitted, so this will always succeed.
	return o.Members().At(0).Parent()
}

// Options returns the options applied to this oneof.
func (o Oneof) Options() MessageValue {
	return wrapValue(o.Context(), o.raw().options).AsMessage()
}

func (o Oneof) raw() *rawOneof {
	return &o.container.oneofs[o.index]
}

// TagRange is a range of reserved field or enum numbers, either from a reserved
// or extensions declaration.
type TagRange struct {
	withContext
	raw *rawRange
}

type rawRange struct {
	ast         ast.ExprAny
	first, last int32
	options     arena.Pointer[rawValue]
}

// AST returns the expression that this range was evaluated from, if known.
func (r TagRange) AST() ast.ExprAny {
	return r.raw.ast
}

// Range returns the start and end of the range.
//
// Unlike how it appears in descriptor.proto, this range is exclusive: end is
// not included.
func (r TagRange) Range() (start, end int32) {
	return r.raw.first, r.raw.last + 1
}

// Options returns the options applied to this range.
//
// Reserved ranges cannot carry options; only extension ranges do.
func (r TagRange) Options() MessageValue {
	return wrapValue(r.Context(), r.raw.options).AsMessage()
}

// ReservedName is a name for a field or enum value that has been reserved for
// future use.
type ReservedName struct {
	withContext
	raw *rawReservedName
}

type rawReservedName struct {
	ast  ast.ExprAny
	name intern.ID
}

// AST returns the expression that this name was evaluated from, if known.
func (r ReservedName) AST() ast.ExprAny {
	return r.raw.ast
}

// Name returns the name that was reserved.
func (r ReservedName) Name() string {
	return r.Context().session.intern.Value(r.raw.name)
}
