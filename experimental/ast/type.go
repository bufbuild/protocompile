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

package ast

import (
	"fmt"
)

const (
	typePath typeKind = iota + 1
	typeModified
	typeGeneric
)

const (
	TypePrefixUnknown TypePrefix = iota
	TypePrefixOptional
	TypePrefixRepeated
	TypePrefixRequired

	// This is the "stream Foo.bar" syntax of RPC methods. It is also treated as
	// a prefix.
	TypePrefixStream
)

// TypePrefix is a prefix for a type, such as required, optional, or repeated.
type TypePrefix int8

type typeKind int8

// TypePrefixByName looks up a prefix kind by name.
//
// If name is not a known prefix, returns [TypePrefixUnknown].
func TypePrefixByName(name string) TypePrefix {
	switch name {
	case "optional":
		return TypePrefixOptional
	case "repeated":
		return TypePrefixRepeated
	case "required":
		return TypePrefixRequired
	case "stream":
		return TypePrefixStream
	default:
		return TypePrefixUnknown
	}
}

// String implements [strings.Stringer] for Modifier.
func (m TypePrefix) String() string {
	switch m {
	case TypePrefixUnknown:
		return "unknown"
	case TypePrefixOptional:
		return "optional"
	case TypePrefixRepeated:
		return "repeated"
	case TypePrefixRequired:
		return "required"
	case TypePrefixStream:
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
// This is implemented by types in this package of the form Type*.
type Type interface {
	Spanner

	typeKind() typeKind
	typeIndex() int
}

// types is storage for every kind of Type in a Context.
type types struct {
	modifieds pointers[rawPrefixed]
	generics  pointers[rawGeneric]
}

func (TypePath) typeKind() typeKind     { return typePath }
func (TypePrefixed) typeKind() typeKind { return typeModified }
func (TypeGeneric) typeKind() typeKind  { return typeGeneric }

func (TypePath) typeIndex() int       { return -1 }
func (t TypePrefixed) typeIndex() int { return t.idx }
func (t TypeGeneric) typeIndex() int  { return t.idx }

// TypePath is a type that is a simple path reference.
type TypePath struct {
	// The path that refers to this type.
	Path
}

var _ Type = TypePath{}

// TypePrefixed is a type with a [TypePrefix].
//
// Unlike in ordinary Protobuf, the Protocompile AST permits arbitrary nesting
// of modifiers.
type TypePrefixed struct {
	withContext

	idx int
	raw *rawPrefixed
}

type rawPrefixed struct {
	prefix rawToken
	ty     rawType
}

// TypePrefixedArgs is the arguments for [Context.NewTypePrefixed].
type TypePrefixedArgs struct {
	Prefix Token
	Type   Type
}

var _ Type = TypePrefixed{}

// Prefix extracts the modifier out of this type.
//
// Returns [TypePrefixUnknown] if [TypePrefixed.PrefixToken] does not contain
// a known modifier.
func (t TypePrefixed) Prefix() TypePrefix {
	return TypePrefixByName(t.PrefixToken().Text())
}

// PrefixToken returns the token representing this type's prefix.
func (t TypePrefixed) PrefixToken() Token {
	return t.raw.prefix.With(t)
}

// Type returns the type that is being prefixed.
func (t TypePrefixed) Type() Type {
	return t.raw.ty.With(t)
}

// SetType sets the expression that is being prefixed.
//
// If passed nil, this clears the type.
func (t TypePrefixed) SetType(ty Type) {
	t.raw.ty = toRawType(ty)
}

// Span implements [Spanner] for TypePrefixed.
func (t TypePrefixed) Span() Span {
	return JoinSpans(t.PrefixToken(), t.Type())
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
	path rawPath
	args rawTypeList
}

// TypeGenericArgs is the arguments for [Context.NewTypeGeneric].
//
// Generic arguments should be added after construction with [TypeGeneric.AppendComma].
type TypeGenericArgs struct {
	Path          Path
	AngleBrackets Token
}

var _ Type = TypeGeneric{}

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
	if t.Path().AsBuiltin() != BuiltinMap || t.Args().Len() != 2 {
		return nil, nil
	}

	return t.Args().At(0), t.Args().At(1)
}

// Args returns the argument list for this generic type.
func (t TypeGeneric) Args() TypeList {
	return TypeList{
		t.withContext,
		&t.raw.args,
	}
}

// Span implements [Spanner] for TypeGeneric.
func (t TypeGeneric) Span() Span {
	return JoinSpans(t.Path(), t.Args())
}

// TypeList is a [Commas] over a list of types surrounded by some kind of brackets.
//
// Despite the name, TypeList does not implement [Type] because it is not a type.
type TypeList struct {
	withContext

	raw *rawTypeList
}

var (
	_ Commas[Type] = TypeList{}
	_ Spanner      = TypeList{}
)

type rawTypeList struct {
	brackets rawToken
	args     []struct {
		ty    rawType
		comma rawToken
	}
}

// Brackets returns the token tree for the brackets wrapping the argument list.
//
// May be nil, if the user forgot to include brackets.
func (d TypeList) Brackets() Token {
	return d.raw.brackets.With(d)
}

// Len implements [Slice] for MethodTypes.
func (d TypeList) Len() int {
	return len(d.raw.args)
}

// At implements [Slice] for MethodTypes.
func (d TypeList) At(n int) Type {
	return d.raw.args[n].ty.With(d)
}

// At implements [Iter] for MethodTypes.
func (d TypeList) Iter(yield func(int, Type) bool) {
	for i, arg := range d.raw.args {
		if !yield(i, arg.ty.With(d)) {
			break
		}
	}
}

// Append implements [Inserter] for TypeGeneric.
func (d TypeList) Append(ty Type) {
	d.InsertComma(d.Len(), ty, Token{})
}

// Insert implements [Inserter] for TypeGeneric.
func (d TypeList) Insert(n int, ty Type) {
	d.InsertComma(n, ty, Token{})
}

// Delete implements [Inserter] for TypeGeneric.
func (d TypeList) Delete(n int) {
	deleteSlice(&d.raw.args, n)
}

// Comma implements [Commas] for MethodTypes.
func (d TypeList) Comma(n int) Token {
	return d.raw.args[n].comma.With(d)
}

// AppendComma implements [Commas] for MethodTypes.
func (d TypeList) AppendComma(ty Type, comma Token) {
	d.InsertComma(d.Len(), ty, comma)
}

// InsertComma implements [Commas] for MethodTypes.
func (d TypeList) InsertComma(n int, ty Type, comma Token) {
	d.Context().panicIfNotOurs(ty, comma)

	insertSlice(&d.raw.args, n, struct {
		ty    rawType
		comma rawToken
	}{toRawType(ty), comma.raw})
}

// Span implements [Spanner] for MethodTypes.
func (d TypeList) Span() Span {
	if !d.Brackets().Nil() {
		return d.Brackets().Span()
	}

	var span Span
	for _, arg := range d.raw.args {
		span = JoinSpans(span, arg.ty.With(d), arg.comma.With(d))
	}
	return span
}

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

func toRawType(t Type) rawType {
	if t == nil {
		return rawType{}
	}
	if path, ok := t.(TypePath); ok {
		return rawType(path.Path.raw)
	}
	return rawType{^rawToken(t.typeKind()), rawToken(t.typeIndex() + 1)}
}

func (t rawType) With(c Contextual) Type {
	if t[0] == 0 && t[1] == 0 {
		return nil
	}

	if t[0] < 0 && t[1] != 0 {
		c := c.Context()
		idx := int(t[1]) - 1
		switch typeKind(^t[0]) {
		case typeModified:
			return TypePrefixed{withContext{c}, idx, c.types.modifieds.At(idx)}
		case typeGeneric:
			return TypeGeneric{withContext{c}, idx, c.types.generics.At(idx)}
		default:
			panic(fmt.Sprintf("protocompile/ast: invalid typeKind: %d", ^t[0]))
		}
	}
	return TypePath{rawPath(t).With(c)}
}
