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
	"fmt"
	"iter"
	"math"

	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/ast/predeclared"
	"github.com/bufbuild/protocompile/experimental/internal"
	"github.com/bufbuild/protocompile/experimental/internal/taxa"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/experimental/token/keyword"
	"github.com/bufbuild/protocompile/internal/arena"
	"github.com/bufbuild/protocompile/internal/ext/iterx"
	"github.com/bufbuild/protocompile/internal/intern"
	"github.com/bufbuild/protocompile/internal/interval"
)

// Type is a Protobuf message field type.
type Type struct {
	withContext

	raw *rawType
}

// TagRange is a range of tag numbers in a [Type].
//
// This can represent either a [Member] or a [ReservedRange].
type TagRange struct {
	withContext
	raw rawTagRange
}

// AsMember returns the [Member] this range points to, or zero if it isn't a
// member.
func (r TagRange) AsMember() Member {
	if r.IsZero() || !r.raw.isMember {
		return Member{}
	}

	return wrapMember(r.Context(), ref[rawMember]{ptr: arena.Pointer[rawMember](r.raw.ptr)})
}

// AsReserved returns the [ReservedRange] this range points to, or zero if it
// isn't a member.
func (r TagRange) AsReserved() ReservedRange {
	if r.IsZero() || r.raw.isMember {
		return ReservedRange{}
	}

	return ReservedRange{
		withContext: r.withContext,
		raw:         r.Context().arenas.ranges.Deref(arena.Pointer[rawReservedRange](r.raw.ptr)),
	}
}

type rawType struct {
	def ast.DeclDef

	nested          []arena.Pointer[rawType]
	members         []arena.Pointer[rawMember]
	memberByName    func() intern.Map[arena.Pointer[rawMember]]
	ranges          []arena.Pointer[rawReservedRange]
	rangesByNumber  interval.Intersect[int32, rawTagRange]
	reservedNames   []rawReservedName
	oneofs          []arena.Pointer[rawOneof]
	options         arena.Pointer[rawValue]
	fqn, name       intern.ID // 0 for predeclared types.
	parent          arena.Pointer[rawType]
	extnsStart      uint32
	rangesExtnStart uint32
	mapEntryOf      arena.Pointer[rawMember]
	features        arena.Pointer[rawFeatureSet]

	isEnum, isMessageSet bool
	allowsAlias          bool
	missingRanges        bool // See lower_numbers.go.
	visibility           token.ID
}

type rawTagRange struct {
	isMember bool
	ptr      arena.Untyped
}

// primitiveCtx represents a special file that defines all of the primitive
// types.
var primitiveCtx = func() *Context {
	ctx := new(Context)

	nextPtr := 1
	predeclared.All()(func(n predeclared.Name) bool {
		if n == predeclared.Unknown || !n.IsScalar() {
			// Skip allocating a pointer for the very first value. This ensures
			// that the arena.Pointer value of the Type corresponding to a
			// predeclared name corresponds to is the same as the name's integer
			// value.
			return true
		}

		for nextPtr != int(n) {
			_ = ctx.arenas.types.NewCompressed(rawType{})
			_ = ctx.arenas.symbols.NewCompressed(rawSymbol{})
			nextPtr++
		}
		ptr := ctx.arenas.types.NewCompressed(rawType{})
		ctx.arenas.symbols.NewCompressed(rawSymbol{
			kind: SymbolKindScalar,
			data: ptr.Untyped(),
		})
		nextPtr++

		if int(ptr) != int(n) {
			panic(fmt.Sprintf("IR initialization error: %d != %d; this is a bug in protocompile", ptr, n))
		}

		ctx.types = append(ctx.types, ptr)
		return true
	})
	return ctx
}()

// PredeclaredType returns the type corresponding to a predeclared name.
//
// Returns the zero value if !n.IsScalar().
func PredeclaredType(n predeclared.Name) Type {
	if !n.IsScalar() {
		return Type{}
	}
	return Type{
		withContext: internal.NewWith(primitiveCtx),
		raw:         primitiveCtx.arenas.types.Deref(arena.Pointer[rawType](n)),
	}
}

// AST returns the declaration for this type, if known.
//
// This need not be an [ast.DefMessage] or [ast.DefEnum]; it may be something
// else in the case of e.g. a map field's entry type.
func (t Type) AST() ast.DeclDef {
	if t.IsZero() {
		return ast.DeclDef{}
	}
	return t.raw.def
}

// IsPredeclared returns whether this is a predeclared type.
func (t Type) IsPredeclared() bool {
	return t.Context() == primitiveCtx
}

// IsMessage returns whether this is a message type.
func (t Type) IsMessage() bool {
	return !t.IsZero() && !t.IsPredeclared() && !t.raw.isEnum
}

// IsMessageSet returns whether this is a message type using the message set
// encoding.
func (t Type) IsMessageSet() bool {
	return !t.IsZero() && t.raw.isMessageSet
}

// IsMap returns whether this is a map type's entry.
func (t Type) IsMapEntry() bool {
	return !t.MapField().IsZero()
}

// IsMessage returns whether this is an enum type.
func (t Type) IsEnum() bool {
	// All of the predeclared types have isEnum set to false, so we don't
	// need to check for them here.
	return !t.IsZero() && t.raw.isEnum
}

func (t Type) IsClosedEnum() bool {
	if !t.IsEnum() {
		return false
	}

	builtins := t.Context().builtins()
	n, _ := t.FeatureSet().Lookup(builtins.FeatureEnum).Value().AsInt()
	return n == 2 // FeatureSet.CLOSED
}

// IsPackable returns whether this type can be the element of a packed repeated
// field.
func (t Type) IsPackable() bool {
	return t.IsEnum() || t.Predeclared().IsPackable()
}

// AllowsAlias returns whether this is an enum type with the allow_alias
// option set.
func (t Type) AllowsAlias() bool {
	return !t.IsZero() && t.raw.allowsAlias
}

// IsAny returns whether this is the type google.protobuf.Any, which gets special
// treatment in the language.
func (t Type) IsAny() bool {
	return t.InternedFullName() == t.Context().session.builtins.AnyPath
}

// IsExported returns whether this type is exported for the purposes of
// visibility in other files.
//
// Returns whether this was set explicitly via the export or local keywords.
func (t Type) IsExported() (exported, explicit bool) {
	if t.IsZero() {
		return false, false
	}

	// This is explicitly set via keyword.
	if !t.raw.visibility.IsZero() {
		return t.raw.visibility.In(t.AST().Context()).Keyword() == keyword.Export, true
	}

	// Look up the feature.
	if key := t.Context().builtins().FeatureVisibility; !key.IsZero() {
		feature := t.FeatureSet().Lookup(key)
		switch v, _ := feature.Value().AsInt(); v {
		case 0, 1: // DEFAULT_SYMBOL_VISIBILITY_UNKNOWN, EXPORT_ALL
			return true, false
		case 2: // EXPORT_TOP_LEVEL
			return t.Parent().IsZero(), false
		case 3, 4: // LOCAL_ALL, STRICT
			return false, false
		}
	}

	// If descriptor.proto is too old to have this feature, assume this
	// type is exported.
	return true, false
}

// Predeclared returns the predeclared type that this Type corresponds to, if any.
//
// Returns either [predeclared.Unknown] or a value such that [predeclared.Name.IsScalar]
// returns true. For example, this will *not* return [predeclared.Map] for map
// fields.
func (t Type) Predeclared() predeclared.Name {
	if !t.IsPredeclared() {
		return predeclared.Unknown
	}

	return predeclared.Name(
		// NOTE: The code that allocates all the primitive types in the
		// primitive context ensures that the pointer value equals the
		// predeclared.Name value.
		t.Context().arenas.types.Compress(t.raw),
	)
}

// Name returns this type's declared name, i.e. the last component of its
// full name.
func (t Type) Name() string {
	return t.FullName().Name()
}

// FullName returns this type's fully-qualified name.
//
// If t is zero, returns "". Otherwise, the returned name will be absolute
// unless this is a primitive type.
func (t Type) FullName() FullName {
	if t.IsZero() {
		return ""
	}
	if p := t.Predeclared(); p != predeclared.Unknown {
		return FullName(p.String())
	}
	return FullName(t.Context().session.intern.Value(t.raw.fqn))
}

// Scope returns the scope in which this type is defined.
func (t Type) Scope() FullName {
	if t.IsZero() {
		return ""
	}
	return FullName(t.Context().session.intern.Value(t.InternedScope()))
}

// InternedName returns the intern ID for [Type.FullName]().Name()
//
// Predeclared types do not have an interned name.
func (t Type) InternedName() intern.ID {
	if t.IsZero() {
		return 0
	}
	return t.raw.name
}

// InternedName returns the intern ID for [Type.FullName]
//
// Predeclared types do not have an interned name.
func (t Type) InternedFullName() intern.ID {
	if t.IsZero() {
		return 0
	}
	return t.raw.fqn
}

// InternedScope returns the intern ID for [Type.Scope]
//
// Predeclared types do not have an interned name.
func (t Type) InternedScope() intern.ID {
	if t.IsZero() {
		return 0
	}
	if parent := t.Parent(); !parent.IsZero() {
		return parent.InternedFullName()
	}
	return t.Context().File().InternedPackage()
}

// Parent returns the type that this type is declared inside of, if it isn't
// at the top level.
func (t Type) Parent() Type {
	if t.IsZero() || t.raw.parent.Nil() {
		return Type{}
	}
	return wrapType(t.Context(), ref[rawType]{ptr: t.raw.parent})
}

// Nested returns those types which are nested within this one.
//
// Only message types have nested types.
func (t Type) Nested() seq.Indexer[Type] {
	var slice []arena.Pointer[rawType]
	if !t.IsZero() {
		slice = t.raw.nested
	}
	return seq.NewFixedSlice(
		slice,
		func(_ int, p arena.Pointer[rawType]) Type {
			// Nested types are always in the current file.
			return wrapType(t.Context(), ref[rawType]{ptr: p})
		},
	)
}

// MapField returns the map field that generated this type, if any.
func (t Type) MapField() Member {
	if t.IsZero() || t.raw.mapEntryOf.Nil() {
		return Member{}
	}
	return wrapMember(t.Context(), ref[rawMember]{ptr: t.raw.mapEntryOf})
}

// EntryFields returns the key and value fields for this map entry type.
func (t Type) EntryFields() (key, value Member) {
	if !t.IsMapEntry() {
		return Member{}, Member{}
	}

	return wrapMember(t.Context(), ref[rawMember]{ptr: t.raw.members[0]}),
		wrapMember(t.Context(), ref[rawMember]{ptr: t.raw.members[1]})
}

// Members returns the members of this type.
//
// Predeclared types have no members; message and enum types do.
func (t Type) Members() seq.Indexer[Member] {
	var slice []arena.Pointer[rawMember]
	if !t.IsZero() {
		slice = t.raw.members[:t.raw.extnsStart]
	}
	return seq.NewFixedSlice(
		slice,
		func(_ int, p arena.Pointer[rawMember]) Member {
			return wrapMember(t.Context(), ref[rawMember]{ptr: p})
		},
	)
}

// MemberByName looks up a member with the given name.
//
// Returns a zero member if there is no such member.
func (t Type) MemberByName(name string) Member {
	if t.IsZero() {
		return Member{}
	}
	id, ok := t.Context().session.intern.Query(name)
	if !ok {
		return Member{}
	}
	return t.MemberByInternedName(id)
}

// MemberByInternedName is like [Type.MemberByName], but takes an interned string.
func (t Type) MemberByInternedName(name intern.ID) Member {
	if t.IsZero() {
		return Member{}
	}
	return wrapMember(t.Context(), ref[rawMember]{ptr: t.raw.memberByName()[name]})
}

// TagRange returns an iterator over [TagRange]s that contain number.
func (t Type) Ranges(number int32) iter.Seq[TagRange] {
	return func(yield func(TagRange) bool) {
		if t.IsZero() {
			return
		}

		entry := t.raw.rangesByNumber.Get(number)
		for _, raw := range entry.Value {
			if !yield(TagRange{t.withContext, raw}) {
				return
			}
		}
	}
}

// MemberByNumber looks up a member with the given number.
//
// Returns a zero member if there is no such member.
func (t Type) MemberByNumber(number int32) Member {
	if t.IsZero() {
		return Member{}
	}

	_, member := iterx.Find(t.Ranges(number), func(r TagRange) bool {
		return !r.AsMember().IsZero()
	})
	return member.AsMember()
}

// membersByNameFunc creates the MemberByName map. This is used to keep
// construction of this map lazy.
func (t Type) makeMembersByName() intern.Map[arena.Pointer[rawMember]] {
	table := make(intern.Map[arena.Pointer[rawMember]], t.Members().Len())
	for _, ptr := range t.raw.members[:t.raw.extnsStart] {
		field := wrapMember(t.Context(), ref[rawMember]{ptr: ptr})
		table[field.InternedName()] = ptr
	}
	return table
}

// Extensions returns any extensions nested within this type.
func (t Type) Extensions() seq.Indexer[Member] {
	var slice []arena.Pointer[rawMember]
	if !t.IsZero() {
		slice = t.raw.members[t.raw.extnsStart:]
	}
	return seq.NewFixedSlice(
		slice,
		func(_ int, p arena.Pointer[rawMember]) Member {
			return wrapMember(t.Context(), ref[rawMember]{ptr: p})
		},
	)
}

// ReservedRanges returns the reserved ranges declared in this type.
//
// This does not include reserved field names; see [Type.ReservedNames].
func (t Type) ReservedRanges() seq.Indexer[ReservedRange] {
	slice := t.raw.ranges[:t.raw.rangesExtnStart]
	return seq.NewFixedSlice(slice, func(_ int, p arena.Pointer[rawReservedRange]) ReservedRange {
		return ReservedRange{t.withContext, t.Context().arenas.ranges.Deref(p)}
	})
}

// ExtensionRanges returns the extension ranges declared in this type.
func (t Type) ExtensionRanges() seq.Indexer[ReservedRange] {
	slice := t.raw.ranges[t.raw.rangesExtnStart:]
	return seq.NewFixedSlice(slice, func(_ int, p arena.Pointer[rawReservedRange]) ReservedRange {
		return ReservedRange{t.withContext, t.Context().arenas.ranges.Deref(p)}
	})
}

// ReservedNames returns the reserved named declared in this type.
func (t Type) ReservedNames() seq.Indexer[ReservedName] {
	return seq.NewFixedSlice(
		t.raw.reservedNames,
		func(i int, _ rawReservedName) ReservedName {
			return ReservedName{t.withContext, &t.raw.reservedNames[i]}
		},
	)
}

// AbsoluteRange returns the smallest and largest number a member of this type
// can have.
//
// This range is inclusive.
func (t Type) AbsoluteRange() (start, end int32) {
	switch {
	case t.IsEnum():
		return math.MinInt32, math.MaxInt32
	case t.IsMessageSet():
		return 1, messageSetNumberMax
	default:
		return 1, fieldNumberMax
	}
}

// OccupiedRanges returns ranges of member numbers currently in use in this
// type. The pairs of numbers are inclusive ranges.
func (t Type) OccupiedRanges() iter.Seq2[[2]int32, seq.Indexer[TagRange]] {
	return func(yield func([2]int32, seq.Indexer[TagRange]) bool) {
		if t.IsZero() {
			return
		}

		for e := range t.raw.rangesByNumber.Contiguous(true) {
			ranges := seq.NewFixedSlice(e.Value, func(_ int, v rawTagRange) TagRange {
				return TagRange{t.withContext, v}
			})
			if !yield([2]int32{e.Start, e.End}, ranges) {
				return
			}
		}
	}
}

// Options returns the options applied to this type.
func (t Type) Oneofs() seq.Indexer[Oneof] {
	return seq.NewFixedSlice(
		t.raw.oneofs,
		func(_ int, p arena.Pointer[rawOneof]) Oneof {
			return wrapOneof(t.Context(), p)
		},
	)
}

// Options returns the options applied to this type.
func (t Type) Options() MessageValue {
	return wrapValue(t.Context(), t.raw.options).AsMessage()
}

// FeatureSet returns the Editions features associated with this type.
func (t Type) FeatureSet() FeatureSet {
	if t.IsZero() || t.raw.features.Nil() {
		return FeatureSet{}
	}

	return FeatureSet{
		internal.NewWith(t.Context()),
		t.Context().arenas.features.Deref(t.raw.features),
	}
}

// Deprecated returns whether this type is deprecated, by returning the
// relevant option value for setting deprecation.
func (t Type) Deprecated() Value {
	if t.IsZero() || t.IsPredeclared() {
		return Value{}
	}
	builtins := t.Context().builtins()
	field := builtins.MessageDeprecated
	if t.IsEnum() {
		field = builtins.EnumDeprecated
	}
	d := t.Options().Field(field)
	if b, _ := d.AsBool(); b {
		return d
	}
	return Value{}
}

// noun returns a [taxa.Noun] for diagnostics.
func (t Type) noun() taxa.Noun {
	switch {
	case t.IsPredeclared():
		return taxa.ScalarType
	case t.IsEnum():
		return taxa.EnumType
	case t.IsMapEntry():
		return taxa.EntryType
	default:
		return taxa.MessageType
	}
}

// toRef returns a ref to this type relative to the given context.
func (t Type) toRef(c *Context) ref[rawType] {
	return ref[rawType]{
		ptr: t.Context().arenas.types.Compress(t.raw),
	}.changeContext(t.Context(), c)
}

func wrapType(c *Context, r ref[rawType]) Type {
	if r.ptr.Nil() || c == nil {
		return Type{}
	}

	c = r.context(c)
	return Type{
		withContext: internal.NewWith(c),
		raw:         c.arenas.types.Deref(r.ptr),
	}
}
