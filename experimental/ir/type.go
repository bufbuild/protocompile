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

	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/ast/predeclared"
	"github.com/bufbuild/protocompile/experimental/internal"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/internal/arena"
	"github.com/bufbuild/protocompile/internal/intern"
)

// Type is a Protobuf field type.
type Type struct {
	withContext

	// This is either a pointer into an arena, *or* into ir.primitives.
	raw *rawType
}

type rawType struct {
	def             ast.DeclDef
	nested          []arena.Pointer[rawType]
	fields          []arena.Pointer[rawField]
	ranges          []rawRange
	reservedNames   []rawReservedName
	oneofs          []arena.Pointer[rawOneof]
	options         []arena.Pointer[rawOption]
	fqn             intern.ID
	fieldsExtnStart uint32
	rangesExtnStart uint32
	isEnum          bool
}

var primitiveCtx = func() *Context {
	ctx := new(Context)
	ctx.intern = new(intern.Table)

	predeclared.All()(func(n predeclared.Name) bool {
		if n == predeclared.Unknown || !n.IsScalar() {
			// Skip allocating a pointer for the very first value. This ensures
			// that the arena.Pointer value of the Type corresponding to a
			// predeclared name corresponds to is the same as the name's integer
			// value.
			return true
		}

		ptr := ctx.arenas.types.NewCompressed(rawType{
			fqn: ctx.intern.Intern(n.String()),
		})

		if int(ptr) != int(n) {
			panic(fmt.Sprintf("IR initialization error: %d != %d; this is a bug in protocompile", ptr, n))
		}

		ctx.file.types = append(ctx.file.types, ptr)
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
func (t Type) AST() ast.DeclDef {
	return t.raw.def
}

// IsPredeclared returns whether this is a predeclared type.
func (t Type) IsPredeclared() bool {
	return t.Context() == primitiveCtx
}

// IsMessage returns whether this is a message type.
func (t Type) IsMessage() bool {
	return !t.IsPredeclared() && !t.raw.isEnum
}

// IsMessage returns whether this is an enum type.
func (t Type) IsEnum() bool {
	// All of the predeclared types have isEnum set to false, so we don't
	// need to check for them here.
	return t.raw.isEnum
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

// Name returns this type's fully-qualified name.
//
// If t is zero, returns "". If t is a primitive type, the returned name will
// not have a leading dot; otherwise, if it is a user-defined type, it will.
func (t Type) Name() string {
	if t.IsZero() {
		return ""
	}
	if p := t.Predeclared(); p != predeclared.Unknown {
		return p.String()
	}
	return t.Context().intern.Value(t.raw.fqn)
}

// InternedName returns this type's fully-qualified name, if it has been
// interned.
//
// Predeclared types do not have an interned name.
func (t Type) InternedName() intern.ID {
	if t.IsPredeclared() {
		return 0
	}
	return t.raw.fqn
}

// Nested returns those types which are nested within this one.
//
// Only message types have nested types.
func (t Type) Nested() seq.Indexer[Type] {
	return seq.Slice[Type, arena.Pointer[rawType]]{
		Slice: t.raw.nested,
		Wrap: func(p *arena.Pointer[rawType]) Type {
			// Nested types are always in the current file.
			return wrapType(t.Context(), ref[rawType]{ptr: *p})
		},
	}
}

// Fields returns the fields of this type.
//
// Predeclared types have no fields; message and enum types do. For enums, a
// field corresponds to an enum value, and will report the zero value for its
// type.
func (t Type) Fields() seq.Indexer[Field] {
	return seq.Slice[Field, arena.Pointer[rawField]]{
		Slice: t.raw.fields[:t.raw.fieldsExtnStart],
		Wrap: func(p *arena.Pointer[rawField]) Field {
			return wrapField(t.Context(), ref[rawField]{ptr: *p})
		},
	}
}

// Extensions returns any extensions nested within this type.
func (t Type) Extensions() seq.Indexer[Field] {
	return seq.Slice[Field, arena.Pointer[rawField]]{
		Slice: t.raw.fields[t.raw.fieldsExtnStart:],
		Wrap: func(p *arena.Pointer[rawField]) Field {
			return wrapField(t.Context(), ref[rawField]{ptr: *p})
		},
	}
}

// ReservedRanges returns the reserved ranges declared in this type.
//
// This does not include reserved field names; see [Type.ReservedNames].
func (t Type) ReservedRanges() seq.Indexer[TagRange] {
	return seq.Slice[TagRange, rawRange]{
		Slice: t.raw.ranges[:t.raw.rangesExtnStart],
		Wrap: func(r *rawRange) TagRange {
			return TagRange{t.withContext, r}
		},
	}
}

// ExtensionRanges returns the extension ranges declared in this type.
func (t Type) ExtensionRanges() seq.Indexer[TagRange] {
	return seq.Slice[TagRange, rawRange]{
		Slice: t.raw.ranges[t.raw.rangesExtnStart:],
		Wrap: func(r *rawRange) TagRange {
			return TagRange{t.withContext, r}
		},
	}
}

// ReservedNames returns the reserved named declared in this type.
func (t Type) ReservedNames() seq.Indexer[ReservedName] {
	return seq.Slice[ReservedName, rawReservedName]{
		Slice: t.raw.reservedNames,
		Wrap: func(r *rawReservedName) ReservedName {
			return ReservedName{t.withContext, r}
		},
	}
}

// Options returns the options applied to this type.
func (t Type) Oneofs() seq.Indexer[Oneof] {
	return seq.Slice[Oneof, arena.Pointer[rawOneof]]{
		Slice: t.raw.oneofs,
		Wrap: func(p *arena.Pointer[rawOneof]) Oneof {
			return wrapOneof(t.Context(), *p)
		},
	}
}

// Options returns the options applied to this type.
func (t Type) Options() seq.Indexer[Option] {
	return seq.Slice[Option, arena.Pointer[rawOption]]{
		Slice: t.raw.options,
		Wrap: func(p *arena.Pointer[rawOption]) Option {
			return wrapOption(t.Context(), *p)
		},
	}
}

func wrapType(c *Context, r ref[rawType]) Type {
	if r.ptr.Nil() || c == nil {
		return Type{}
	}

	var ctx *Context
	switch {
	case r.file == -1:
		ctx = primitiveCtx
	case r.file > 0:
		ctx = c.file.imports[r.file-1].Context()
	default:
		ctx = c.File().Context()
	}

	return Type{
		withContext: internal.NewWith(ctx),
		raw:         ctx.arenas.types.Deref(r.ptr),
	}
}
