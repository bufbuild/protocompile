// Copyright 2020-2024 Buf Technologies, Inc.
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

package ast2

import (
	"fmt"
)

const (
	typePath typeKind = iota + 1
	typeModified
	typeGeneric
)

const (
	ModifierUnknown Modifier = iota + 1
	ModifierOptional
	ModifierRepeated
	ModifierRequired

	// Syntactically, we treat "group" as a modifier, because it
	// essentially parses the same way.
	//
	// Group [Field]s are the only fields that have no syntactically-specified
	// name.
	ModifierGroup

	// This is the "stream Foo.bar" syntax of RPC methods. It is also treated as
	// a modifier.
	ModifierStream
)

// Modifier is a modifier for a type, such as required, optional, or repeated.
type Modifier int8

// ModifierByName looks up a modifier kind by name.
//
// If name is not a known modifier, returns [ModifierUnknown].
func ModifierByName(name string) Modifier {
	switch name {
	case "optional":
		return ModifierOptional
	case "repeated":
		return ModifierRepeated
	case "required":
		return ModifierRequired
	case "group":
		return ModifierGroup
	case "stream":
		return ModifierStream
	default:
		return ModifierUnknown
	}
}

// String implements [strings.Stringer] for Modifier.
func (m Modifier) String() string {
	switch m {
	case ModifierUnknown:
		return "unknown"
	case ModifierOptional:
		return "optional"
	case ModifierRepeated:
		return "repeated"
	case ModifierRequired:
		return "required"
	case ModifierGroup:
		return "group"
	case ModifierStream:
		return "stream"
	default:
		return fmt.Sprintf("modifier%d", int(m))
	}
}

// Type is the type of a field or service method.
//
// In the Protocompile AST, we regard many things as types for the sake of diagnostics.
// For example, "optional string" is a type, but so is the invalid type
// "optional repeated string".
//
// This is implemented by a limited set of types: [TypePath], [TypeModified], and [TypeGeneric].
type Type interface {
	Spanner

	typeKind() typeKind
	rawType() rawType
}

func (TypePath) typeKind() typeKind     { return typePath }
func (TypeModified) typeKind() typeKind { return typeModified }
func (TypeGeneric) typeKind() typeKind  { return typeGeneric }

// TypePath is a type that is a simple path reference.
type TypePath struct {
	// The path that refers to this type.
	Path
}

var _ Type = TypePath{}

func (t TypePath) rawType() rawType {
	return rawType(t.Path.raw)
}

// TypeModified is a type with a [Modifier] prefix.
//
// Unlike in ordinary Protobuf, the Protocompile AST permits arbitrary nesting
// of modifiers.
type TypeModified struct {
	withContext

	idx int
	raw *rawModified
}

type rawModified struct {
	modifier rawToken
	inner    rawType
}

var _ Type = TypeModified{}

// Modifier extracts the modifier out of this type.
//
// Returns [ModifierUnknown] if [TypeModified.ModifierToken] does not contain
// a known modifier.
func (t TypeModified) Modifier() Modifier {
	return ModifierByName(t.Keyword().Text())
}

// Keyword returns the token representing this type's modifier.
func (t TypeModified) Keyword() Token {
	return t.raw.modifier.With(t)
}

// Type returns the type that is being modified.
func (t TypeModified) Type() Type {
	return t.raw.inner.With(t)
}

// Span implements [Spanner] for TypeModified.
func (t TypeModified) Span() Span {
	return JoinSpans(t.Keyword(), t.Type())
}

func (t TypeModified) rawType() rawType {
	return rawType{^rawToken(typeModified), rawToken(t.idx)}
}

// TypeGeneric is a type with generic arguments.
//
// Protobuf does not have generics... mostly. It has the map<K, V> production,
// which looks like something that generalizes, but doesn't. It is useful to parse
// when users mistakenly think this generalizes or provide the incorrect number
// of arguments.
//
// You will usually want to immediately call [TypeGeneric.Map] to codify the assumption
// that all generic types understood by your code are maps.
//
// TypeGeneric implements [Commas[Type]] for accessing its arguments.
type TypeGeneric struct {
	withContext

	idx int
	raw *rawGeneric
}

type rawGeneric struct {
	path   rawPath
	angles rawToken
	args   []struct {
		ty    rawType
		comma rawToken
	}
}

var (
	_ Type         = TypeGeneric{}
	_ Commas[Type] = TypeGeneric{}
)

// Path returns the path of the "type constructor". For example, for
// my.Map<K, V>, this would return the path my.Map.
func (t TypeGeneric) Path() Path {
	return t.raw.path.With(t)
}

// AsMap extracts the key/value types out of this generic type, checking that it's actually a
// map<K, V>. This is intended for asserting the extremely common case of "the only generic
// type is map".
//
// Returns nils if this is not a map, or it has the wrong number of generic arguments.
func (t TypeGeneric) AsMap() (key, value Type) {
	if t.Path().AsBuiltin() != BuiltinMap || t.Len() != 2 {
		return nil, nil
	}

	return t.At(0), t.At(1)
}

// AngleBrackets returns the token tree corresponding to the <...> part of the type.
func (t TypeGeneric) AngleBrackets() Token {
	return t.raw.angles.With(t)
}

// Len implements [Slice] for TypeGeneric.
func (t TypeGeneric) Len() int {
	return len(t.raw.args)
}

// At implements [Slice] for TypeGeneric.
func (t TypeGeneric) At(n int) Type {
	return t.raw.args[n].ty.With(t)
}

// Iter implements [Slice] for TypeGeneric.
func (t TypeGeneric) Iter(yield func(int, Type) bool) {
	for i, ty := range t.raw.args {
		if !yield(i, ty.ty.With(t)) {
			break

		}
	}
}

// Append implements [Inserter] for TypeGeneric.
func (t TypeGeneric) Append(ty Type) {
	t.InsertComma(t.Len(), ty, Token{})
}

// Insert implements [Inserter] for TypeGeneric.
func (t TypeGeneric) Insert(n int, ty Type) {
	t.InsertComma(n, ty, Token{})
}

// Delete implements [Inserter] for TypeGeneric.
func (t TypeGeneric) Delete(n int) {
	deleteSlice(&t.raw.args, n)
}

// Comma implements [Commas] for TypeGeneric.
func (t TypeGeneric) Comma(n int) Token {
	return t.raw.args[n].comma.With(t)
}

// AppendComma implements [Commas] for TypeGeneric.
func (t TypeGeneric) AppendComma(ty Type, comma Token) {
	t.InsertComma(t.Len(), ty, comma)
}

// InsertComma implements [Commas] for TypeGeneric.
func (t TypeGeneric) InsertComma(n int, ty Type, comma Token) {
	t.Context().panicIfNotOurs(ty, comma)

	insertSlice(&t.raw.args, n, struct {
		ty    rawType
		comma rawToken
	}{ty.rawType(), comma.id})
}

// Span implements [Spanner] for TypeGeneric.
func (t TypeGeneric) Span() Span {
	return JoinSpans(t.Path(), t.AngleBrackets())
}

func (t TypeGeneric) rawType() rawType {
	return rawType{^rawToken(typeModified), rawToken(t.idx)}
}

type typeKind int8

// rawType is the raw representation of a type.
//
// The vast, vast majority of types are paths. To avoid needing to waste
// space for such types, we use the following encoding for rawType.
//
// First, note that if the first half of a rawPath is negative, the other
// must be zero. Thus, if the first "token" of the rawPath is negative and
// the second is not, the first is ^typeKind and the second is an index
// into a table in a Context. Otherwise, it's a path type. This logic is
// implemented in With().
type rawType rawPath

func (t rawType) With(c Contextual) Type {
	if t[0] == 0 && t[1] == 0 {
		return nil
	}

	if t[0] < 0 && t[1] > 0 {
		c := c.Context()
		idx := int(t[1]) // NOTE: no -1 here, nil is represented by 0, 0 above.
		switch typeKind(^t[0]) {
		case typeModified:
			return TypeModified{withContext{c}, idx, c.modifieds.At(idx)}
		case typeGeneric:
			return TypeGeneric{withContext{c}, idx, c.generics.At(idx)}
		default:
			panic(fmt.Sprintf("protocompile/ast: invalid typeKind: %d", ^t[0]))
		}
	}
	return TypePath{rawPath(t).With(c)}
}
