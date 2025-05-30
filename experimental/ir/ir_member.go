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
	"github.com/bufbuild/protocompile/experimental/internal/taxa"
	"github.com/bufbuild/protocompile/experimental/ir/presence"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/internal/arena"
	"github.com/bufbuild/protocompile/internal/intern"
)

// Member is a Protobuf message field, enum value, or extension field.
//
// A member has three types associated with it. The English language struggles
// to give these succinct names, so we review them here.
//
//  1. Its _element_, i.e. the type it contains. This is the type that a member
//     is declared to be _of_. Not present for enum values.
//
//  2. Its _parent_, i.e., the type it is syntactically defined within.
//     Extensions appear syntactically within their parent.
//
//  3. Its _container_, i.e., the type which it is part of for the purposes of
//     serialization. Extensions are fields of their container, but are declared
//     within their parent.
type Member struct {
	withContext
	raw *rawMember
}

type rawMember struct {
	def ast.DeclDef

	options   arena.Pointer[rawValue]
	elem      ref[rawType]
	extendee  arena.Pointer[rawExtendee]
	fqn, name intern.ID
	number    int32
	parent    arena.Pointer[rawType]

	// If negative, this is the negative of a presence.Kind. Otherwise, it's
	// a oneof index.
	oneof int32
}

// Returns whether this is a non-extension message field.
func (m Member) IsMessageField() bool {
	return !m.IsZero() && !m.raw.elem.ptr.Nil() && m.raw.extendee.Nil()
}

// Returns whether this is a extension message field.
func (m Member) IsExtension() bool {
	return !m.IsZero() && !m.raw.elem.ptr.Nil() && !m.raw.extendee.Nil()
}

// Returns whether this is an enum value.
func (m Member) IsEnumValue() bool {
	return !m.IsZero() && m.raw.elem.ptr.Nil()
}

// AST returns the declaration for this member, if known.
func (m Member) AST() ast.DeclDef {
	return m.raw.def
}

// FullName returns this members's name.
func (m Member) Name() string {
	if m.IsZero() {
		return ""
	}
	return m.Context().session.intern.Value(m.raw.name)
}

// FullName returns this members's fully-qualified name.
func (m Member) FullName() FullName {
	if m.IsZero() {
		return ""
	}
	return FullName(m.Context().session.intern.Value(m.raw.fqn))
}

// Scope returns the scope in which this member is defined.
func (m Member) Scope() FullName {
	if m.IsZero() {
		return ""
	}
	return FullName(m.Context().session.intern.Value(m.InternedScope()))
}

// InternedName returns the intern ID for [Member.FullName]().Name().
func (m Member) InternedName() intern.ID {
	if m.IsZero() {
		return 0
	}
	return m.raw.name
}

// InternedName returns the intern ID for [Member.FullName].
func (m Member) InternedFullName() intern.ID {
	if m.IsZero() {
		return 0
	}
	return m.raw.fqn
}

// InternedScope returns the intern ID for [Member.Scope].
func (m Member) InternedScope() intern.ID {
	if m.IsZero() {
		return 0
	}
	if parent := m.Parent(); !parent.IsZero() {
		return parent.InternedFullName()
	}
	return m.Context().File().InternedPackage()
}

// Number returns the number for this member after expression evaluation.
//
// Defaults to zero if the number is not specified.
func (m Member) Number() int32 {
	return m.raw.number
}

// Presence returns this member's presence kind.
//
// Returns [presence.Unknown] for enum values.
func (m Member) Presence() presence.Kind {
	if m.IsZero() {
		return presence.Unknown
	}
	if m.raw.oneof >= 0 {
		if m.Parent().IsEnum() {
			return presence.Unknown
		}
		return presence.Shared
	}
	return presence.Kind(-m.raw.oneof)
}

// Parent returns the type this member is syntactically located in. This is the
// type it is declared *in*, but which it is not necessarily part of.
//
// May be zero for extensions declared at the top level.
func (m Member) Parent() Type {
	if m.IsZero() {
		return Type{}
	}
	return wrapType(m.Context(), ref[rawType]{ptr: m.raw.parent})
}

// Element returns the this member's element type. This is the type it is
// declared to be *of*, such as in the phrase "a string field's type is string".
//
// This does not include the member's presence: for example, a repeated int32
// member will report the type as being the int32 primitive, not an int32 array.
//
// This is zero for enum values.
func (m Member) Element() Type {
	if m.IsZero() {
		return Type{}
	}
	return wrapType(m.Context(), m.raw.elem)
}

// Container returns the type which contains this member: this is either
// [Member.Parent], or the extendee if this is an extension. This is the
// type it is declared to be *part of*.
func (m Member) Container() Type {
	if m.IsZero() {
		return Type{}
	}

	if m.raw.extendee.Nil() {
		return m.Parent()
	}

	extends := m.Context().arenas.extendees.Deref(m.raw.extendee)
	return wrapType(m.Context(), extends.ty)
}

// Oneof returns the oneof that this member is a member of.
//
// Returns the zero value if this member does not have [presence.Shared].
func (m Member) Oneof() Oneof {
	if m.Presence() != presence.Shared {
		return Oneof{}
	}
	return m.Parent().Oneofs().At(int(m.raw.oneof))
}

// Options returns the options applied to this member.
func (m Member) Options() MessageValue {
	return wrapValue(m.Context(), m.raw.options).AsMessage()
}

// noun returns a [taxa.Noun] for diagnostics.
func (m Member) noun() taxa.Noun {
	switch {
	case m.IsEnumValue():
		return taxa.EnumValue
	case m.IsExtension():
		return taxa.Extension
	default:
		return taxa.Field
	}
}

func wrapMember(c *Context, r ref[rawMember]) Member {
	if r.ptr.Nil() || c == nil {
		return Member{}
	}

	c = r.context(c)
	return Member{
		withContext: internal.NewWith(c),
		raw:         c.arenas.members.Deref(r.ptr),
	}
}

// rawExtendee represents an extends block.
//
// Rather than each field carrying a reference to its extends block's AST, we
// have a level of indirection to amortize symbol lookups.
type rawExtendee struct {
	def    ast.DeclDef
	ty     ref[rawType]
	parent arena.Pointer[rawType]
}

func (e *rawExtendee) Scope(c *Context) FullName {
	return FullName(c.session.intern.Value(e.InternedScope(c)))
}

func (e *rawExtendee) InternedScope(c *Context) intern.ID {
	if !e.parent.Nil() {
		return wrapType(c, ref[rawType]{ptr: e.parent}).InternedFullName()
	}
	return c.File().InternedPackage()
}

// Oneof represents a oneof within a message definition.
type Oneof struct {
	withContext
	raw *rawOneof
}

type rawOneof struct {
	def       ast.DeclDef
	fqn, name intern.ID
	index     uint32
	container arena.Pointer[rawType]
	members   []arena.Pointer[rawMember]
	options   arena.Pointer[rawValue]
}

// AST returns the declaration for this oneof, if known.
func (o Oneof) AST() ast.DeclDef {
	return o.raw.def
}

// Name returns this oneof's declared name.
func (o Oneof) Name() string {
	if o.IsZero() {
		return ""
	}
	return o.Context().session.intern.Value(o.raw.name)
}

// FullName returns this oneof's fully-qualified name.
func (o Oneof) FullName() FullName {
	if o.IsZero() {
		return ""
	}
	return FullName(o.Context().session.intern.Value(o.raw.fqn))
}

// InternedName returns the intern ID for [Oneof.FullName]().Name().
func (o Oneof) InternedName() intern.ID {
	if o.IsZero() {
		return 0
	}
	return o.raw.name
}

// InternedName returns the intern ID for [Oneof.FullName].
func (o Oneof) InternedFullName() intern.ID {
	if o.IsZero() {
		return 0
	}
	return o.raw.fqn
}

// Container returns the message type which contains it.
func (o Oneof) Container() Type {
	if o.IsZero() {
		return Type{}
	}

	return wrapType(o.Context(), ref[rawType]{ptr: o.raw.container})
}

// Index returns this oneof's index in its containing message.
func (o Oneof) Index() int {
	if o.IsZero() {
		return 0
	}
	return int(o.raw.index)
}

// Members returns this oneof's member fields.
func (o Oneof) Members() seq.Indexer[Member] {
	return seq.NewFixedSlice(
		o.raw.members,
		func(_ int, p arena.Pointer[rawMember]) Member {
			return wrapMember(o.Context(), ref[rawMember]{ptr: p})
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
	return wrapValue(o.Context(), o.raw.options).AsMessage()
}

func wrapOneof(c *Context, raw arena.Pointer[rawOneof]) Oneof {
	return Oneof{
		withContext: internal.NewWith(c),
		raw:         c.arenas.oneofs.Deref(raw),
	}
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

// Name returns the name (i.e., an identifier) that was reserved.
func (r ReservedName) Name() string {
	if r.IsZero() {
		return ""
	}
	return r.Context().session.intern.Value(r.raw.name)
}

// InternedName returns the intern ID for [ReservedName.Name].
func (r ReservedName) InternedName() intern.ID {
	if r.IsZero() {
		return 0
	}
	return r.raw.name
}
