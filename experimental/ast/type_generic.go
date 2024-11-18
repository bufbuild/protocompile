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
	"slices"

	"github.com/bufbuild/protocompile/experimental/ast/predeclared"
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
type TypeGeneric struct{ typeImpl[rawTypeGeneric] }

type rawTypeGeneric struct {
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
func (t TypeGeneric) AsMap() (key, value TypeAny) {
	if t.Path().AsPredeclared() != predeclared.Map || t.Args().Len() != 2 {
		return TypeAny{}, TypeAny{}
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

// Span implements [Spanner].
func (t TypeGeneric) Span() Span {
	return JoinSpans(t.Path(), t.Args())
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
	_ Spanner         = TypeList{}
)

type rawTypeList struct {
	brackets rawToken
	args     []withComma[rawType]
}

// Brackets returns the token tree for the brackets wrapping the argument list.
//
// May be nil, if the user forgot to include brackets.
func (d TypeList) Brackets() Token {
	return d.raw.brackets.With(d)
}

// Len implements [Slice].
func (d TypeList) Len() int {
	return len(d.raw.args)
}

// At implements [Slice].
func (d TypeList) At(n int) TypeAny {
	return TypeAny{d.withContext, d.raw.args[n].Value}
}

// At implements [Iter].
func (d TypeList) Iter(yield func(int, TypeAny) bool) {
	for i, arg := range d.raw.args {
		if !yield(i, TypeAny{d.withContext, arg.Value}) {
			break
		}
	}
}

// Append implements [Inserter].
func (d TypeList) Append(ty TypeAny) {
	d.InsertComma(d.Len(), ty, Token{})
}

// Insert implements [Inserter].
func (d TypeList) Insert(n int, ty TypeAny) {
	d.InsertComma(n, ty, Token{})
}

// Delete implements [Inserter].
func (d TypeList) Delete(n int) {
	d.raw.args = slices.Delete(d.raw.args, n, n+1)
}

// Comma implements [Commas].
func (d TypeList) Comma(n int) Token {
	return d.raw.args[n].Comma.With(d)
}

// AppendComma implements [Commas].
func (d TypeList) AppendComma(ty TypeAny, comma Token) {
	d.InsertComma(d.Len(), ty, comma)
}

// InsertComma implements [Commas].
func (d TypeList) InsertComma(n int, ty TypeAny, comma Token) {
	d.Context().panicIfNotOurs(ty, comma)

	d.raw.args = slices.Insert(d.raw.args, n, withComma[rawType]{ty.raw, comma.raw})
}

// Span implements [Spanner].
func (d TypeList) Span() Span {
	if !d.Brackets().Nil() {
		return d.Brackets().Span()
	}

	var span Span
	for _, arg := range d.raw.args {
		span = JoinSpans(span, TypeAny{d.withContext, arg.Value}, arg.Comma.With(d))
	}
	return span
}