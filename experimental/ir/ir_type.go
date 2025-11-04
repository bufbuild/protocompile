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
	"github.com/bufbuild/protocompile/experimental/id"
	"github.com/bufbuild/protocompile/experimental/internal/taxa"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/internal/arena"
	"github.com/bufbuild/protocompile/internal/ext/iterx"
	"github.com/bufbuild/protocompile/internal/intern"
	"github.com/bufbuild/protocompile/internal/interval"
)

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
	return id.Wrap(r.Context(), id.ID[Member](r.raw.ptr))
}

// AsReserved returns the [ReservedRange] this range points to, or zero if it
// isn't a member.
func (r TagRange) AsReserved() ReservedRange {
	if r.IsZero() || r.raw.isMember {
		return ReservedRange{}
	}
	return id.Wrap(r.Context(), id.ID[ReservedRange](r.raw.ptr))
}

// Type is a Protobuf message field type.
type Type id.Node[Type, *File, *rawType]

type rawType struct {
	nested          []id.ID[Type]
	members         []id.ID[Member]
	memberByName    func() intern.Map[id.ID[Member]]
	ranges          []id.ID[ReservedRange]
	rangesByNumber  interval.Intersect[int32, rawTagRange]
	reservedNames   []rawReservedName
	oneofs          []id.ID[Oneof]
	extends         []id.ID[Extend]
	def             id.ID[ast.DeclDef]
	options         id.ID[Value]
	fqn, name       intern.ID // 0 for predeclared types.
	parent          id.ID[Type]
	extnsStart      uint32
	rangesExtnStart uint32
	mapEntryOf      id.ID[Member]
	features        id.ID[FeatureSet]

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
var primitiveCtx = func() *File {
	ctx := new(File)

	nextPtr := 1
	for n := range predeclared.All() {
		if n == predeclared.Unknown || !n.IsScalar() {
			// Skip allocating a pointer for the very first value. This ensures
			// that the arena.Pointer value of the Type corresponding to a
			// predeclared name corresponds to is the same as the name's integer
			// value.
			continue
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

		ctx.types = append(ctx.types, id.ID[Type](ptr))
	}
	return ctx
}()

// PredeclaredType returns the type corresponding to a predeclared name.
//
// Returns the zero value if !n.IsScalar().
func PredeclaredType(n predeclared.Name) Type {
	if !n.IsScalar() {
		return Type{}
	}
	return id.Wrap(primitiveCtx, id.ID[Type](n))
}

// AST returns the declaration for this type, if known.
//
// This need not be an [ast.DefMessage] or [ast.DefEnum]; it may be something
// else in the case of e.g. a map field's entry type.
func (t Type) AST() ast.DeclDef {
	if t.IsZero() {
		return ast.DeclDef{}
	}
	return id.Wrap(t.Context().AST(), t.Raw().def)
}

// IsPredeclared returns whether this is a predeclared type.
func (t Type) IsPredeclared() bool {
	return t.Context() == primitiveCtx
}

// IsMessage returns whether this is a message type.
func (t Type) IsMessage() bool {
	return !t.IsZero() && !t.IsPredeclared() && !t.Raw().isEnum
}

// IsMessageSet returns whether this is a message type using the message set
// encoding.
func (t Type) IsMessageSet() bool {
	return !t.IsZero() && t.Raw().isMessageSet
}

// IsMapEntry returns whether this is a map type's entry.
func (t Type) IsMapEntry() bool {
	return !t.MapField().IsZero()
}

// IsEnum returns whether this is an enum type.
func (t Type) IsEnum() bool {
	// All of the predeclared types have isEnum set to false, so we don't
	// need to check for them here.
	return !t.IsZero() && t.Raw().isEnum
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
	return !t.IsZero() && t.Raw().allowsAlias
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
		t.Context().arenas.types.Compress(t.Raw()),
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
	return FullName(t.Context().session.intern.Value(t.Raw().fqn))
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
	return t.Raw().name
}

// InternedName returns the intern ID for [Type.FullName]
//
// Predeclared types do not have an interned name.
func (t Type) InternedFullName() intern.ID {
	if t.IsZero() {
		return 0
	}
	return t.Raw().fqn
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
	return t.Context().InternedPackage()
}

// Parent returns the type that this type is declared inside of, if it isn't
// at the top level.
func (t Type) Parent() Type {
	if t.IsZero() {
		return Type{}
	}
	return id.Wrap(t.Context(), t.Raw().parent)
}

// Nested returns those types which are nested within this one.
//
// Only message types have nested types.
func (t Type) Nested() seq.Indexer[Type] {
	var slice []id.ID[Type]
	if !t.IsZero() {
		slice = t.Raw().nested
	}
	return seq.NewFixedSlice(
		slice,
		func(_ int, p id.ID[Type]) Type {
			return id.Wrap(t.Context(), p)
		},
	)
}

// MapField returns the map field that generated this type, if any.
func (t Type) MapField() Member {
	if t.IsZero() {
		return Member{}
	}
	return id.Wrap(t.Context(), t.Raw().mapEntryOf)
}

// EntryFields returns the key and value fields for this map entry type.
func (t Type) EntryFields() (key, value Member) {
	if !t.IsMapEntry() {
		return Member{}, Member{}
	}

	return id.Wrap(t.Context(), t.Raw().members[0]), id.Wrap(t.Context(), t.Raw().members[1])
}

// Members returns the members of this type.
//
// Predeclared types have no members; message and enum types do.
func (t Type) Members() seq.Indexer[Member] {
	var slice []id.ID[Member]
	if !t.IsZero() {
		slice = t.Raw().members[:t.Raw().extnsStart]
	}
	return seq.NewFixedSlice(
		slice,
		func(_ int, p id.ID[Member]) Member {
			return id.Wrap(t.Context(), p)
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
	return id.Wrap(t.Context(), t.Raw().memberByName()[name])
}

// Ranges returns an iterator over [TagRange]s that contain number.
func (t Type) Ranges(number int32) iter.Seq[TagRange] {
	return func(yield func(TagRange) bool) {
		if t.IsZero() {
			return
		}

		entry := t.Raw().rangesByNumber.Get(number)
		for _, raw := range entry.Value {
			if !yield(TagRange{id.WrapContext(t.Context()), raw}) {
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
func (t Type) makeMembersByName() intern.Map[id.ID[Member]] {
	table := make(intern.Map[id.ID[Member]], t.Members().Len())
	for _, p := range t.Raw().members[:t.Raw().extnsStart] {
		field := id.Wrap(t.Context(), p)
		table[field.InternedName()] = p
	}
	return table
}

// Extensions returns any extensions nested within this type.
func (t Type) Extensions() seq.Indexer[Member] {
	var slice []id.ID[Member]
	if !t.IsZero() {
		slice = t.Raw().members[t.Raw().extnsStart:]
	}
	return seq.NewFixedSlice(
		slice,
		func(_ int, p id.ID[Member]) Member {
			return id.Wrap(t.Context(), p)
		},
	)
}

// AllRanges returns all reserved/extension ranges declared in this type.
//
// This does not include reserved field names; see [Type.ReservedNames].
func (t Type) AllRanges() seq.Indexer[ReservedRange] {
	slice := t.Raw().ranges
	return seq.NewFixedSlice(slice, func(_ int, p id.ID[ReservedRange]) ReservedRange {
		return id.Wrap(t.Context(), p)
	})
}

// ReservedRanges returns the reserved ranges declared in this type.
//
// This does not include reserved field names; see [Type.ReservedNames].
func (t Type) ReservedRanges() seq.Indexer[ReservedRange] {
	slice := t.Raw().ranges[:t.Raw().rangesExtnStart]
	return seq.NewFixedSlice(slice, func(_ int, p id.ID[ReservedRange]) ReservedRange {
		return id.Wrap(t.Context(), p)
	})
}

// ExtensionRanges returns the extension ranges declared in this type.
func (t Type) ExtensionRanges() seq.Indexer[ReservedRange] {
	slice := t.Raw().ranges[t.Raw().rangesExtnStart:]
	return seq.NewFixedSlice(slice, func(_ int, p id.ID[ReservedRange]) ReservedRange {
		return id.Wrap(t.Context(), p)
	})
}

// ReservedNames returns the reserved named declared in this type.
func (t Type) ReservedNames() seq.Indexer[ReservedName] {
	return seq.NewFixedSlice(
		t.Raw().reservedNames,
		func(i int, _ rawReservedName) ReservedName {
			return ReservedName{id.WrapContext(t.Context()), &t.Raw().reservedNames[i]}
		},
	)
}

// Oneofs returns the options applied to this type.
func (t Type) Oneofs() seq.Indexer[Oneof] {
	return seq.NewFixedSlice(
		t.Raw().oneofs,
		func(_ int, p id.ID[Oneof]) Oneof {
			return id.Wrap(t.Context(), p)
		},
	)
}

// Extends returns the options applied to this type.
func (t Type) Extends() seq.Indexer[Extend] {
	return seq.NewFixedSlice(
		t.Raw().extends,
		func(_ int, p id.ID[Extend]) Extend {
			return id.Wrap(t.Context(), p)
		},
	)
}

// Options returns the options applied to this type.
func (t Type) Options() MessageValue {
	return id.Wrap(t.Context(), t.Raw().options).AsMessage()
}

// FeatureSet returns the Editions features associated with this type.
func (t Type) FeatureSet() FeatureSet {
	if t.IsZero() {
		return FeatureSet{}
	}
	return id.Wrap(t.Context(), t.Raw().features)
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
func (t Type) toRef(file *File) Ref[Type] {
	return Ref[Type]{id: t.ID()}.ChangeContext(t.Context(), file)
}
