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

	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/ast/predeclared"
	"github.com/bufbuild/protocompile/experimental/internal"
	"github.com/bufbuild/protocompile/experimental/internal/taxa"
	"github.com/bufbuild/protocompile/experimental/seq"
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

// IsMessage returns whether this is an enum type.
func (t Type) IsEnum() bool {
	// All of the predeclared types have isEnum set to false, so we don't
	// need to check for them here.
	return !t.IsZero() && t.raw.isEnum
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
		for _, raw := range entry.Values {
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

// noun returns a [taxa.Noun] for diagnostics.
func (t Type) noun() taxa.Noun {
	switch {
	case t.IsPredeclared():
		return taxa.ScalarType
	case t.IsEnum():
		return taxa.EnumType
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
