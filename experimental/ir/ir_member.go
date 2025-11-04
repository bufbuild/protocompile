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

	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/ast/predeclared"
	"github.com/bufbuild/protocompile/experimental/id"
	"github.com/bufbuild/protocompile/experimental/internal/taxa"
	"github.com/bufbuild/protocompile/experimental/ir/presence"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/internal/arena"
	"github.com/bufbuild/protocompile/internal/intern"
)

//go:generate go run github.com/bufbuild/protocompile/internal/enum option_target.yaml

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
type Member id.Node[Member, *File, *rawMember]

type rawMember struct {
	featureInfo   *rawFeatureInfo
	elem          Ref[Type]
	number        int32
	extendee      id.ID[Extend]
	fqn           intern.ID
	name          intern.ID
	def           id.ID[ast.DeclDef]
	parent        id.ID[Type]
	features      id.ID[FeatureSet]
	options       id.ID[Value]
	oneof         int32
	optionTargets uint32
	jsonName      intern.ID
	isGroup       bool
	numberOk      bool
}

// IsMessageField returns whether this is a non-extension message field.
func (m Member) IsMessageField() bool {
	return !m.IsZero() && !m.Raw().elem.IsZero() && m.Raw().extendee.IsZero()
}

// IsExtension returns whether this is a extension message field.
func (m Member) IsExtension() bool {
	return !m.IsZero() && !m.Raw().elem.IsZero() && !m.Raw().extendee.IsZero()
}

// IsEnumValue returns whether this is an enum value.
func (m Member) IsEnumValue() bool {
	return !m.IsZero() && m.Raw().elem.IsZero()
}

// IsGroup returns whether this is a group-encoded field.
func (m Member) IsGroup() bool {
	return !m.IsZero() && m.Raw().isGroup
}

// IsSynthetic returns whether or not this is a synthetic field, such as the
// fields of a map entry.
func (m Member) IsSynthetic() bool {
	return !m.IsZero() && m.AST().IsZero()
}

// IsSingular returns whether this is a singular field; this includes oneof
// members.
func (m Member) IsSingular() bool {
	return m.Presence() != presence.Unknown && m.Presence() != presence.Repeated
}

// IsRepeated returns whether this is a repeated field; this includes map
// fields.
func (m Member) IsRepeated() bool {
	return m.Presence() == presence.Repeated
}

// IsMap returns whether this is a map field.
func (m Member) IsMap() bool {
	return !m.IsZero() && m == m.Element().MapField()
}

// IsPacked returns whether this is a packed message field.
func (m Member) IsPacked() bool {
	if !m.IsRepeated() {
		return false
	}

	builtins := m.Context().builtins()
	option := m.Options().Field(builtins.Packed)
	if packed, ok := option.AsBool(); ok {
		return packed
	}

	feature := m.FeatureSet().Lookup(builtins.FeaturePacked).Value()
	value, _ := feature.AsInt()
	return value == 1 // google.protobuf.FeatureSet.PACKED
}

// IsUnicode returns whether this is a string-typed message field that must
// contain UTF-8 bytes.
func (m Member) IsUnicode() bool {
	if m.Element().Predeclared() != predeclared.String {
		return false
	}

	builtins := m.Context().builtins()
	utf8Feature, _ := m.FeatureSet().Lookup(builtins.FeatureUTF8).Value().AsInt()
	return utf8Feature == 2 // FeatureSet.VERIFY
}

// AsTagRange wraps this member in a TagRange.
func (m Member) AsTagRange() TagRange {
	if m.IsZero() {
		return TagRange{}
	}
	return TagRange{
		id.WrapContext(m.Context()),
		rawTagRange{
			isMember: true,
			ptr:      arena.Untyped(m.Context().arenas.members.Compress(m.Raw())),
		},
	}
}

// AST returns the declaration for this member, if known.
func (m Member) AST() ast.DeclDef {
	if m.IsZero() {
		return ast.DeclDef{}
	}
	return id.Wrap(m.Context().AST(), m.Raw().def)
}

// TypeAST returns the type AST node for this member, if known.
func (m Member) TypeAST() ast.TypeAny {
	decl := m.AST()
	if !decl.IsZero() {
		if m.IsGroup() {
			return ast.TypePath{Path: decl.Name()}.AsAny()
		}
		return decl.Type()
	}

	ty := m.Container()
	if !ty.MapField().IsZero() {
		k, v := ty.AST().Type().RemovePrefixes().AsGeneric().AsMap()
		switch m.Number() {
		case 1:
			return k
		case 2:
			return v
		}
	}

	return ast.TypeAny{}
}

// Name returns this member's name.
func (m Member) Name() string {
	if m.IsZero() {
		return ""
	}
	return m.Context().session.intern.Value(m.Raw().name)
}

// FullName returns this member's fully-qualified name.
func (m Member) FullName() FullName {
	if m.IsZero() {
		return ""
	}
	return FullName(m.Context().session.intern.Value(m.Raw().fqn))
}

// JSONName returns this member's JSON name, either the default-generated one
// or the one set via the json_name pseudo-option.
func (m Member) JSONName() string {
	if m.IsZero() {
		return ""
	}
	return m.Context().session.intern.Value(m.Raw().jsonName)
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
	return m.Raw().name
}

// InternedFullName returns the intern ID for [Member.FullName].
func (m Member) InternedFullName() intern.ID {
	if m.IsZero() {
		return 0
	}
	return m.Raw().fqn
}

// InternedScope returns the intern ID for [Member.Scope].
func (m Member) InternedScope() intern.ID {
	if m.IsZero() {
		return 0
	}
	if parent := m.Parent(); !parent.IsZero() {
		return parent.InternedFullName()
	}
	return m.Context().InternedPackage()
}

// InternedJSONName returns the intern ID for [Member.JSONName].
func (m Member) InternedJSONName() intern.ID {
	if m.IsZero() {
		return 0
	}
	return m.Raw().jsonName
}

// Number returns the number for this member after expression evaluation.
//
// Defaults to zero if the number is not specified.
func (m Member) Number() int32 {
	if m.IsZero() {
		return 0
	}
	return m.Raw().number
}

// Presence returns this member's presence kind.
//
// Returns [presence.Unknown] for enum values.
func (m Member) Presence() presence.Kind {
	if m.IsZero() {
		return presence.Unknown
	}
	if m.Raw().oneof >= 0 {
		if m.Parent().IsEnum() {
			return presence.Unknown
		}
		return presence.Shared
	}
	return presence.Kind(-m.Raw().oneof)
}

// Parent returns the type this member is syntactically located in. This is the
// type it is declared *in*, but which it is not necessarily part of.
//
// May be zero for extensions declared at the top level.
func (m Member) Parent() Type {
	if m.IsZero() {
		return Type{}
	}
	return id.Wrap(m.Context(), m.Raw().parent)
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
	return GetRef(m.Context(), m.Raw().elem)
}

// Container returns the type which contains this member: this is either
// [Member.Parent], or the extendee if this is an extension. This is the
// type it is declared to be *part of*.
func (m Member) Container() Type {
	if m.IsZero() {
		return Type{}
	}

	extends := id.Wrap(m.Context(), m.Raw().extendee)
	if extends.IsZero() {
		return m.Parent()
	}

	return extends.Extendee()
}

// Extend returns the extend block this member is declared in, if any.
func (m Member) Extend() Extend {
	if m.IsZero() || m.Raw().extendee.IsZero() {
		return Extend{}
	}
	return id.Wrap(m.Context(), m.Raw().extendee)
}

// Oneof returns the oneof that this member is a member of.
//
// Returns the zero value if this member does not have [presence.Shared].
func (m Member) Oneof() Oneof {
	if m.Presence() != presence.Shared {
		return Oneof{}
	}
	return m.Parent().Oneofs().At(int(m.Raw().oneof))
}

// Options returns the options applied to this member.
func (m Member) Options() MessageValue {
	return id.Wrap(m.Context(), m.Raw().options).AsMessage()
}

// PseudoOptions returns this member's pseudo options.
func (m Member) PseudoOptions() PseudoFields {
	return m.Options().pseudoFields()
}

// FeatureSet returns the Editions features associated with this member.
func (m Member) FeatureSet() FeatureSet {
	if m.IsZero() {
		return FeatureSet{}
	}

	return id.Wrap(m.Context(), m.Raw().features)
}

// FeatureInfo returns feature definition information relating to this field
// (for when using this field as a feature).
//
// Returns a zero value if this information does not exist.
func (m Member) FeatureInfo() FeatureInfo {
	if m.IsZero() || m.Raw().featureInfo == nil {
		return FeatureInfo{}
	}

	return FeatureInfo{
		id.WrapContext(m.Context()),
		m.Raw().featureInfo,
	}
}

// Deprecated returns whether this member is deprecated, by returning the
// relevant option value for setting deprecation.
func (m Member) Deprecated() Value {
	if m.IsZero() {
		return Value{}
	}
	builtins := m.Context().builtins()
	field := builtins.FieldDeprecated
	if m.IsEnumValue() {
		field = builtins.EnumValueDeprecated
	}
	d := m.Options().Field(field)
	if b, _ := d.AsBool(); b {
		return d
	}
	return Value{}
}

// CanTarget returns whether this message field can be set as an option for the
// given option target type.
//
// This is mediated by the option FieldOptions.targets, which controls whether
// this field can be set (transitively) on the options of a given entity type.
// This is useful for options which re-use the same message type for different
// option types, such as FeatureSet.
func (m Member) CanTarget(target OptionTarget) bool {
	if m.IsZero() {
		return false
	}

	return m.Raw().optionTargets == 0 ||
		(m.Raw().optionTargets>>uint(target))&1 != 0 // Check if the target-th bit is set.
}

// Targets returns an iterator over the valid option targets for this member.
func (m Member) Targets() iter.Seq[OptionTarget] {
	return func(yield func(OptionTarget) bool) {
		if m.IsZero() {
			return
		}
		if m.Raw().optionTargets == 0 {
			OptionTargets()(yield)
			return
		}

		bits := m.Raw().optionTargets
		for t := range OptionTargets() {
			if bits == 0 {
				return
			}

			mask := uint32(1) << t
			if bits&mask != 0 && !yield(t) {
				return
			}
			bits &^= mask
		}
	}
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

// toRef returns a ref to this member relative to the given context.
func (m Member) toRef(f *File) Ref[Member] {
	return Ref[Member]{id: m.ID()}.ChangeContext(m.Context(), f)
}

// Extend represents an extend block associated with some extension field.
type Extend id.Node[Extend, *File, *rawExtend]

// rawExtend represents an extends block.
//
// Rather than each field carrying a reference to its extends block's AST, we
// have a level of indirection to amortize symbol lookups.
type rawExtend struct {
	def     id.ID[ast.DeclDef]
	ty      Ref[Type]
	parent  id.ID[Type]
	members []id.ID[Member]
}

// AST returns the declaration for this extend block, if known.
func (e Extend) AST() ast.DeclDef {
	if e.IsZero() {
		return ast.DeclDef{}
	}
	return id.Wrap(e.Context().AST(), e.Raw().def)
}

// Scope returns the scope that symbol lookups in this block should be performed
// against.
func (e Extend) Scope() FullName {
	if e.IsZero() {
		return ""
	}

	return FullName(e.Context().session.intern.Value(e.InternedScope()))
}

// InternedScope returns the intern ID for [Extend.Scope].
func (e Extend) InternedScope() intern.ID {
	if e.IsZero() {
		return 0
	}
	if ty := e.Parent(); !ty.IsZero() {
		return ty.InternedFullName()
	}
	return e.Context().InternedPackage()
}

// Extendee returns the extendee type of this extend block.
func (e Extend) Extendee() Type {
	if e.IsZero() {
		return Type{}
	}
	return GetRef(e.Context(), e.Raw().ty)
}

// Parent returns the type this extend block is declared in.
func (e Extend) Parent() Type {
	if e.IsZero() {
		return Type{}
	}
	return id.Wrap(e.Context(), e.Raw().parent)
}

// Extensions returns the extensions declared in this block.
func (e Extend) Extensions() seq.Indexer[Member] {
	var members []id.ID[Member]
	if !e.IsZero() {
		members = e.Raw().members
	}
	return seq.NewFixedSlice(members, func(_ int, p id.ID[Member]) Member {
		return id.Wrap(e.Context(), p)
	})
}

// Oneof represents a oneof within a message definition.
type Oneof id.Node[Oneof, *File, *rawOneof]

type rawOneof struct {
	def       id.ID[ast.DeclDef]
	fqn, name intern.ID
	index     uint32
	container id.ID[Type]
	members   []id.ID[Member]
	options   id.ID[Value]
	features  id.ID[FeatureSet]
}

// AST returns the declaration for this oneof, if known.
func (o Oneof) AST() ast.DeclDef {
	if o.IsZero() {
		return ast.DeclDef{}
	}
	return id.Wrap(o.Context().AST(), o.Raw().def)
}

// Name returns this oneof's declared name.
func (o Oneof) Name() string {
	if o.IsZero() {
		return ""
	}
	return o.Context().session.intern.Value(o.Raw().name)
}

// FullName returns this oneof's fully-qualified name.
func (o Oneof) FullName() FullName {
	if o.IsZero() {
		return ""
	}
	return FullName(o.Context().session.intern.Value(o.Raw().fqn))
}

// InternedName returns the intern ID for [Oneof.FullName]().Name().
func (o Oneof) InternedName() intern.ID {
	if o.IsZero() {
		return 0
	}
	return o.Raw().name
}

// InternedFullName returns the intern ID for [Oneof.FullName].
func (o Oneof) InternedFullName() intern.ID {
	if o.IsZero() {
		return 0
	}
	return o.Raw().fqn
}

// Container returns the message type which contains it.
func (o Oneof) Container() Type {
	if o.IsZero() {
		return Type{}
	}

	return id.Wrap(o.Context(), o.Raw().container)
}

// Index returns this oneof's index in its containing message.
func (o Oneof) Index() int {
	if o.IsZero() {
		return 0
	}
	return int(o.Raw().index)
}

// Members returns this oneof's member fields.
func (o Oneof) Members() seq.Indexer[Member] {
	return seq.NewFixedSlice(
		o.Raw().members,
		func(_ int, p id.ID[Member]) Member {
			return id.Wrap(o.Context(), p)
		},
	)
}

// Parent returns the type that this oneof is declared within,.
func (o Oneof) Parent() Type {
	if o.IsZero() {
		return Type{}
	}
	// Empty oneofs are not permitted, so this will always succeed.
	return o.Members().At(0).Parent()
}

// Options returns the options applied to this oneof.
func (o Oneof) Options() MessageValue {
	if o.IsZero() {
		return MessageValue{}
	}
	return id.Wrap(o.Context(), o.Raw().options).AsMessage()
}

// FeatureSet returns the Editions features associated with this oneof.
func (o Oneof) FeatureSet() FeatureSet {
	if o.IsZero() {
		return FeatureSet{}
	}
	return id.Wrap(o.Context(), o.Raw().features)
}

// ReservedRange is a range of reserved field or enum numbers,
// either from a reserved or extensions declaration.
type ReservedRange id.Node[ReservedRange, *File, *rawReservedRange]

type rawReservedRange struct {
	value         id.Dyn[ast.ExprAny, ast.ExprKind]
	decl          id.ID[ast.DeclRange]
	first         int32
	last          int32
	options       id.ID[Value]
	features      id.ID[FeatureSet]
	forExtensions bool
	rangeOk       bool
}

// AST returns the expression that this range was evaluated from, if known.
func (r ReservedRange) AST() ast.ExprAny {
	if r.IsZero() {
		return ast.ExprAny{}
	}

	return id.WrapDyn(r.Context().AST(), r.Raw().value)
}

// DeclAST returns the declaration this range came from. Multiple ranges may
// have the same declaration.
func (r ReservedRange) DeclAST() ast.DeclRange {
	if r.IsZero() {
		return ast.DeclRange{}
	}

	return id.Wrap(r.Context().AST(), r.Raw().decl)
}

// Range returns the start and end of the range.
func (r ReservedRange) Range() (start, end int32) {
	if r.IsZero() {
		return 0, 0
	}

	return r.Raw().first, r.Raw().last
}

// ForExtensions returns whether this is an extension range.
func (r ReservedRange) ForExtensions() bool {
	return !r.IsZero() && r.Raw().forExtensions
}

// AsTagRange wraps this range in a TagRange.
func (r ReservedRange) AsTagRange() TagRange {
	if r.IsZero() {
		return TagRange{}
	}
	return TagRange{
		id.WrapContext(r.Context()),
		rawTagRange{
			isMember: true,
			ptr:      arena.Untyped(r.ID()),
		},
	}
}

// Options returns the options applied to this range.
//
// Reserved ranges cannot carry options; only extension ranges do.
func (r ReservedRange) Options() MessageValue {
	if r.IsZero() {
		return MessageValue{}
	}

	return id.Wrap(r.Context(), r.Raw().options).AsMessage()
}

// FeatureSet returns the Editions features associated with this file.
func (r ReservedRange) FeatureSet() FeatureSet {
	if r.IsZero() {
		return FeatureSet{}
	}
	return id.Wrap(r.Context(), r.Raw().features)
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
