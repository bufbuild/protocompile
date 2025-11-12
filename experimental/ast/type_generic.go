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

package ast

import (
	"slices"

	"github.com/bufbuild/protocompile/experimental/ast/predeclared"
	"github.com/bufbuild/protocompile/experimental/id"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/experimental/token"
)

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
// TypeGeneric implements [Commas[TypeAny]] for accessing its arguments.
//
// # Grammar
//
//	TypeGeneric := TypePath `<` (Type `,`?`)* `>`
type TypeGeneric id.Node[TypeGeneric, *File, *rawTypeGeneric]

type rawTypeGeneric struct {
	path PathID
	args rawTypeList
}

// TypeGenericArgs is the arguments for [Context.NewTypeGeneric].
//
// Generic arguments should be added after construction with [TypeGeneric.AppendComma].
type TypeGenericArgs struct {
	Path          Path
	AngleBrackets token.Token
}

// AsAny type-erases this type value.
//
// See [TypeAny] for more information.
func (t TypeGeneric) AsAny() TypeAny {
	if t.IsZero() {
		return TypeAny{}
	}

	return id.WrapDyn(t.Context(), id.NewDyn(TypeKindGeneric, id.ID[TypeAny](t.ID())))
}

// Path returns the path of the "type constructor". For example, for
// my.Map<K, V>, this would return the path my.Map.
func (t TypeGeneric) Path() Path {
	if t.IsZero() {
		return Path{}
	}

	return t.Raw().path.In(t.Context())
}

// AsMap extracts the key/value types out of this generic type, checking that it's actually a
// map<K, V>. This is intended for asserting the extremely common case of "the only generic
// type is map".
//
// Returns zeros if this is not a map, or it has the wrong number of generic arguments.
func (t TypeGeneric) AsMap() (key, value TypeAny) {
	if t.Path().AsPredeclared() != predeclared.Map || t.Args().Len() != 2 {
		return TypeAny{}, TypeAny{}
	}

	return t.Args().At(0), t.Args().At(1)
}

// Args returns the argument list for this generic type.
func (t TypeGeneric) Args() TypeList {
	if t.IsZero() {
		return TypeList{}
	}

	return TypeList{
		id.WrapContext(t.Context()),
		&t.Raw().args,
	}
}

// Span implements [source.Spanner].
func (t TypeGeneric) Span() source.Span {
	if t.IsZero() {
		return source.Span{}
	}

	return source.Join(t.Path(), t.Args())
}

// TypeList is a [Commas] over a list of types surrounded by some kind of brackets.
//
// Despite the name, TypeList does not implement [TypeAny] because it is not a type.
type TypeList struct {
	withContext
	raw *rawTypeList
}

var (
	_ Commas[TypeAny] = TypeList{}
	_ source.Spanner  = TypeList{}
)

type rawTypeList struct {
	brackets token.ID
	args     []withComma[id.Dyn[TypeAny, TypeKind]]
}

// Brackets returns the token tree for the brackets wrapping the argument list.
//
// May be zero, if the user forgot to include brackets.
func (d TypeList) Brackets() token.Token {
	if d.IsZero() {
		return token.Zero
	}

	return id.Wrap(d.Context().Stream(), d.raw.brackets)
}

// SetBrackets sets the token tree for the brackets wrapping the argument list.
func (d TypeList) SetBrackets(brackets token.Token) {
	d.Context().Nodes().panicIfNotOurs(brackets)
	d.raw.brackets = brackets.ID()
}

// Len implements [seq.Indexer].
func (d TypeList) Len() int {
	if d.IsZero() {
		return 0
	}

	return len(d.raw.args)
}

// At implements [seq.Indexer].
func (d TypeList) At(n int) TypeAny {
	return id.WrapDyn(d.Context(), d.raw.args[n].Value)
}

// SetAt implements [seq.Setter].
func (d TypeList) SetAt(n int, ty TypeAny) {
	d.Context().Nodes().panicIfNotOurs(ty)
	d.raw.args[n].Value = ty.ID()
}

// Insert implements [seq.Inserter].
func (d TypeList) Insert(n int, ty TypeAny) {
	d.InsertComma(n, ty, token.Zero)
}

// Delete implements [seq.Inserter].
func (d TypeList) Delete(n int) {
	d.raw.args = slices.Delete(d.raw.args, n, n+1)
}

// Comma implements [Commas].
func (d TypeList) Comma(n int) token.Token {
	return id.Wrap(d.Context().Stream(), d.raw.args[n].Comma)
}

// AppendComma implements [Commas].
func (d TypeList) AppendComma(value TypeAny, comma token.Token) {
	d.InsertComma(d.Len(), value, comma)
}

// InsertComma implements [Commas].
func (d TypeList) InsertComma(n int, ty TypeAny, comma token.Token) {
	d.Context().Nodes().panicIfNotOurs(ty, comma)

	d.raw.args = slices.Insert(d.raw.args, n, withComma[id.Dyn[TypeAny, TypeKind]]{ty.ID(), comma.ID()})
}

// Span implements [source.Spanner].
func (d TypeList) Span() source.Span {
	switch {
	case d.IsZero():
		return source.Span{}
	case !d.Brackets().IsZero():
		return d.Brackets().Span()
	case d.Len() == 0:
		return source.Span{}
	default:
		return source.Join(d.At(0), d.At(d.Len()-1))
	}
}
