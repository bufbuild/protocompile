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
	"github.com/bufbuild/protocompile/experimental/id"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/internal/arena"
)

// TagAny is any Tag* type in this package.
//
// Values of this type can be obtained by calling an AsAny method on a Tag*
// type, such as [TagText.AsAny]. It can be type-asserted back to any of
// the concrete Tag* types using its own As* methods.
//
// This type is used in lieu of a putative TagAny interface type to avoid heap
// allocations in functions that would return one of many different Tag*
// types.
//
// # Grammar
//
//	Tag      :=
type TagAny id.DynNode[TagAny, TagKind, *File]

// AsEnd converts a TagAny into a TagEnd, if that is the tag
// it contains.
//
// Otherwise, returns zero.
func (t TagAny) AsEnd() TagEnd {
	if t.Kind() != TagKindEnd {
		return TagEnd{}
	}

	return id.Wrap(t.Context(), id.ID[TagEnd](t.ID().Value()))
}

// AsText converts a TagAny into a TagText, if that is the tag
// it contains.
//
// Otherwise, returns zero.
func (t TagAny) AsText() TagText {
	if t.Kind() != TagKindText {
		return TagText{}
	}

	return id.Wrap(t.Context(), id.ID[TagText](t.ID().Value()))
}

// AsExpr converts a TagAny into a TagExpr, if that is the tag
// it contains.
//
// Otherwise, returns zero.
func (t TagAny) AsExpr() TagExpr {
	if t.Kind() != TagKindExpr {
		return TagExpr{}
	}

	return id.Wrap(t.Context(), id.ID[TagExpr](t.ID().Value()))
}

// AsEmit converts a TagAny into a TagEmit, if that is the tag
// it contains.
//
// Otherwise, returns zero.
func (t TagAny) AsEmit() TagEmit {
	if t.Kind() != TagKindEmit {
		return TagEmit{}
	}

	return id.Wrap(t.Context(), id.ID[TagEmit](t.ID().Value()))
}

// AsImport converts a TagAny into a TagImport, if that is the tag
// it contains.
//
// Otherwise, returns zero.
func (t TagAny) AsImport() TagImport {
	if t.Kind() != TagKindImport {
		return TagImport{}
	}

	return id.Wrap(t.Context(), id.ID[TagImport](t.ID().Value()))
}

// AsIf converts a TagAny into a TagIf, if that is the tag
// it contains.
//
// Otherwise, returns zero.
func (t TagAny) AsIf() TagIf {
	if t.Kind() != TagKindIf {
		return TagIf{}
	}

	return id.Wrap(t.Context(), id.ID[TagIf](t.ID().Value()))
}

// AsFor converts a TagAny into a TagFor, if that is the tag
// it contains.
//
// Otherwise, returns zero.
func (t TagAny) AsFor() TagFor {
	if t.Kind() != TagKindFor {
		return TagFor{}
	}

	return id.Wrap(t.Context(), id.ID[TagFor](t.ID().Value()))
}

// AsSwitch converts a TagAny into a TagSwitch, if that is the tag
// it contains.
//
// Otherwise, returns zero.
func (t TagAny) AsSwitch() TagSwitch {
	if t.Kind() != TagKindSwitch {
		return TagSwitch{}
	}

	return id.Wrap(t.Context(), id.ID[TagSwitch](t.ID().Value()))
}

// AsCase converts a TagAny into a TagCase, if that is the tag
// it contains.
//
// Otherwise, returns zero.
func (t TagAny) AsCase() TagCase {
	if t.Kind() != TagKindCase {
		return TagCase{}
	}

	return id.Wrap(t.Context(), id.ID[TagCase](t.ID().Value()))
}

// AsMacro converts a TagAny into a TagMacro, if that is the tag
// it contains.
//
// Otherwise, returns zero.
func (t TagAny) AsMacro() TagMacro {
	if t.Kind() != TagKindMacro {
		return TagMacro{}
	}

	return id.Wrap(t.Context(), id.ID[TagMacro](t.ID().Value()))
}

// Span implements [source.Spanner].
func (t TagAny) Span() source.Span {
	return source.Join(
		t.AsEnd(),
		t.AsEmit(),
		t.AsEmit(),
		t.AsImport(),
		t.AsIf(),
		t.AsFor(),
		t.AsSwitch(),
		t.AsCase(),
		t.AsMacro(),
	)
}

// TagEnd marks the end of a [Fragment].
// # Grammar
//
//	TagEnd := `[:` `end` `:]`
type TagEnd id.Node[TagEnd, *File, *rawTagEnd]

// TagEndArg sis arguments for [Nodes.NewTagEnd].
type TagEndArgs struct {
	Brackets token.Token
	Keyword  token.Token
}

type rawTagEnd struct {
	brackets token.ID
	keyword  token.ID
}

// AsAny type-erases this tag value.
//
// See [TagAny] for more information.
func (t TagEnd) AsAny() TagAny {
	if t.IsZero() {
		return TagAny{}
	}
	return id.WrapDyn(t.Context(), id.NewDyn(TagKindEnd, id.ID[TagAny](t.ID())))
}

// Brackets returns the bracket token that makes up this tag.
func (t TagEnd) Brackets() token.Token {
	if t.IsZero() {
		return token.Zero
	}
	return id.Wrap(t.Context().Stream(), t.Raw().brackets)
}

// Keyword returns the "end" keyword token.
func (t TagEnd) Keyword() token.Token {
	if t.IsZero() {
		return token.Zero
	}
	return id.Wrap(t.Context().Stream(), t.Raw().keyword)
}

// Span implements [source.Spanner].
func (t TagEnd) Span() source.Span {
	return t.Brackets().Span()
}

func (TagKind) DecodeDynID(lo, _ int32) TagKind {
	return TagKind(lo)
}

func (k TagKind) EncodeDynID(value int32) (int32, int32, bool) {
	return int32(k), value, true
}

type tags struct {
	ends     arena.Arena[rawTagEnd]
	texts    arena.Arena[rawTagText]
	exprs    arena.Arena[rawTagExpr]
	emits    arena.Arena[rawTagEmit]
	imports  arena.Arena[rawTagImport]
	ifs      arena.Arena[rawTagIf]
	fors     arena.Arena[rawTagFor]
	switches arena.Arena[rawTagSwitch]
	cases    arena.Arena[rawTagCase]
	macros   arena.Arena[rawTagMacro]
}
