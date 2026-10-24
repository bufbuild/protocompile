// Copyright 2020-2026 Buf Technologies, Inc.
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
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/internal/arena"
)

var _ id.Context = (*File)(nil)
var _ source.Spanner = DeclAny{}
var _ source.Spanner = Field{}
var _ source.Spanner = Literal{}
var _ source.Spanner = Block{}

// File is the root of a protoscope AST.
type File struct {
	path   string
	stream *token.Stream
	nodes  Nodes

	decls id.DynSeq[DeclAny, DeclKind, *File]
}

// New creates a new File with the given path and token stream.
func New(path string, stream *token.Stream) *File {
	f := &File{
		path:   path,
		stream: stream,
	}
	f.nodes.file = f
	return f
}

// FromID implements id.Context.
func (f *File) FromID(id uint64, want any) any {
	switch want.(type) {
	case **rawField:
		return f.nodes.fields.Deref(arena.Pointer[rawField](id))
	case **rawLiteral:
		return f.nodes.literals.Deref(arena.Pointer[rawLiteral](id))
	case **rawBlock:
		return f.nodes.blocks.Deref(arena.Pointer[rawBlock](id))
	default:
		return nil
	}
}

// Path returns the path to this file.
func (f *File) Path() string {
	return f.path
}

// Stream returns the token stream for this file.
func (f *File) Stream() *token.Stream {
	return f.stream
}

// Nodes returns a [Nodes] that can be used to construct new nodes in this file.
func (f *File) Nodes() *Nodes {
	return &f.nodes
}

// Decls returns the top-level declarations in this file.
func (f *File) Decls() seq.Inserter[DeclAny] {
	return f.decls.Inserter(f)
}

// DeclAny represents any protoscope declaration.
type DeclAny id.DynNode[DeclAny, DeclKind, *File]

// AsField converts a DeclAny into a Field, if that is the declaration it contains.
func (d DeclAny) AsField() Field {
	if d.Kind() != DeclKindField {
		return Field{}
	}
	return id.Wrap(d.Context(), id.ID[Field](d.ID().Value()))
}

// AsLiteral converts a DeclAny into a Literal, if that is the declaration it contains.
func (d DeclAny) AsLiteral() Literal {
	if d.Kind() != DeclKindLiteral {
		return Literal{}
	}
	return id.Wrap(d.Context(), id.ID[Literal](d.ID().Value()))
}

// AsBlock converts a DeclAny into a Block, if that is the declaration it contains.
func (d DeclAny) AsBlock() Block {
	if d.Kind() != DeclKindBlock {
		return Block{}
	}
	return id.Wrap(d.Context(), id.ID[Block](d.ID().Value()))
}

func (d DeclAny) Span() source.Span {
	switch d.Kind() {
	case DeclKindField:
		return d.AsField().Span()
	case DeclKindLiteral:
		return d.AsLiteral().Span()
	case DeclKindBlock:
		return d.AsBlock().Span()
	default:
		return source.Span{}
	}
}

type DeclKind byte

const (
	DeclKindUnknown DeclKind = iota
	DeclKindField
	DeclKindLiteral
	DeclKindBlock
)

func (k DeclKind) DecodeDynID(lo, _ int32) DeclKind {
	return DeclKind(lo)
}

func (k DeclKind) EncodeDynID(value int32) (lo, hi int32, ok bool) {
	return int32(k), value, true
}

// Nodes provides storage for the various AST node types, and can be used
// to construct new ones.
type Nodes struct {
	file *File

	fields   arena.Arena[rawField]
	literals arena.Arena[rawLiteral]
	blocks   arena.Arena[rawBlock]
}

// NewField creates a new Field node.
func (n *Nodes) NewField(args FieldArgs) Field {
	idField := n.fields.NewCompressed(rawField{
		tag:      args.Tag.ID(),
		wireType: args.WireType.ID(),
		value:    args.Value.ID(),
	})
	return id.Wrap(n.file, id.ID[Field](idField))
}

// NewLiteral creates a new Literal node.
func (n *Nodes) NewLiteral(t token.Token) Literal {
	idLiteral := n.literals.NewCompressed(rawLiteral{
		token: t.ID(),
	})
	return id.Wrap(n.file, id.ID[Literal](idLiteral))
}

// NewBlock creates a new Block node.
func (n *Nodes) NewBlock(t token.Token) Block {
	idBlock := n.blocks.NewCompressed(rawBlock{
		token: t.ID(),
	})
	return id.Wrap(n.file, id.ID[Block](idBlock))
}

// Field represents a tag expression: `Tag:WireType Value` or `Tag: Value`.
type Field id.Node[Field, *File, *rawField]

type rawField struct {
	tag      token.ID
	wireType token.ID // Optional, may be zero
	value    id.Dyn[DeclAny, DeclKind]
}

type FieldArgs struct {
	Tag      token.Token
	WireType token.Token // Optional
	Value    DeclAny
}

func (f Field) Tag() token.Token {
	return id.Wrap(f.Context().Stream(), f.Raw().tag)
}

func (f Field) WireType() token.Token {
	return id.Wrap(f.Context().Stream(), f.Raw().wireType)
}

func (f Field) Value() DeclAny {
	return id.WrapDyn(f.Context(), f.Raw().value)
}

func (f Field) AsAny() DeclAny {
	return id.WrapDyn(f.Context(), id.NewDyn(DeclKindField, id.ID[DeclAny](f.ID())))
}

func (f Field) Span() source.Span {
	return source.Join(f.Tag(), f.WireType(), f.Value())
}

// Literal represents a single literal value (string, number, boolean, or hex).
type Literal id.Node[Literal, *File, *rawLiteral]

type rawLiteral struct {
	token token.ID
}

func (l Literal) Token() token.Token {
	return id.Wrap(l.Context().Stream(), l.Raw().token)
}

func (l Literal) AsAny() DeclAny {
	return id.WrapDyn(l.Context(), id.NewDyn(DeclKindLiteral, id.ID[DeclAny](l.ID())))
}

func (l Literal) Span() source.Span {
	return l.Token().Span()
}

// Block represents a sequence of declarations enclosed in brackets or braces.
// For example: `[...]` (length-prefixed) or `!{...}` (group).
type Block id.Node[Block, *File, *rawBlock]

type rawBlock struct {
	token token.ID
	decls id.DynSeq[DeclAny, DeclKind, *File]
}

func (b Block) Token() token.Token {
	return id.Wrap(b.Context().Stream(), b.Raw().token)
}

func (b Block) Decls() seq.Inserter[DeclAny] {
	return b.Raw().decls.Inserter(b.Context())
}

func (b Block) AsAny() DeclAny {
	return id.WrapDyn(b.Context(), id.NewDyn(DeclKindBlock, id.ID[DeclAny](b.ID())))
}

func (b Block) Span() source.Span {
	return b.Token().Span()
}
