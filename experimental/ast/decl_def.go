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
	"iter"

	"github.com/bufbuild/protocompile/experimental/id"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/experimental/token/keyword"
)

// DeclDef is a general Protobuf definition.
//
// This [Decl] represents the union of several similar AST nodes, to aid in permissive
// parsing and precise diagnostics.
//
// This node represents messages, enums, services, extend blocks, fields, enum values,
// oneofs, groups, service methods, and options. It also permits nonsensical syntax, such as a
// message with a tag number.
//
// Generally, you should not need to work with DeclDef directly; instead, use the As* methods
// to access the correct concrete syntax production a DeclDef represents.
//
// # Grammar
//
//	DeclDef := (Type Path | Type | Ident) followers* (`;` | DeclBody)?
//
//	followers := inputs | outputs | value | CompactOptions
//	inputs    := `(` (Type `,`?)* `)`
//	outputs   := `returns` (Type | inputs)?
//	value     := (`=` Expr) | ExprPath | ExprLiteral | ExprRange | ExprField
//
// Note that this type will only record the first appearance of any follower.
type DeclDef id.Node[DeclDef, *File, *rawDeclDef]

type rawDeclDef struct {
	ty   id.Dyn[TypeAny, TypeKind] // Not present for enum fields.
	name PathID

	signature *rawSignature

	equals token.ID
	value  id.Dyn[ExprAny, ExprKind]

	options id.ID[CompactOptions]
	body    id.ID[DeclBody]
	semi    token.ID

	corrupt bool
}

// DeclDefArgs is arguments for creating a [DeclDef] with [Context.NewDeclDef].
type DeclDefArgs struct {
	// If both Keyword and Type are set, Type will be prioritized.
	Keyword token.Token
	Type    TypeAny
	Name    Path

	// NOTE: the values for the type signature are not provided at
	// construction time, and should be added by mutating through
	// DeclDef.Signature.
	Returns token.Token

	Equals token.Token
	Value  ExprAny

	Options CompactOptions

	Body      DeclBody
	Semicolon token.Token
}

// AsAny type-erases this declaration value.
//
// See [DeclAny] for more information.
func (d DeclDef) AsAny() DeclAny {
	if d.IsZero() {
		return DeclAny{}
	}
	return id.WrapDyn(d.Context(), id.NewDyn(DeclKindDef, id.ID[DeclAny](d.ID())))
}

// Type returns the "prefix" type of this definition.
//
// This type may coexist with a [Signature] in this definition.
//
// May be zero, such as for enum values. For messages and other productions
// introduced by a special keyword, this will be a [TypePath] whose single
// identifier is that keyword.
//
// See [DeclDef.KeywordToken].
func (d DeclDef) Type() TypeAny {
	if d.IsZero() {
		return TypeAny{}
	}

	return id.WrapDyn(d.Context(), d.Raw().ty)
}

// SetType sets the "prefix" type of this definition.
func (d DeclDef) SetType(ty TypeAny) {
	d.Raw().ty = ty.ID()
}

// KeywordToken returns the introducing keyword for this definition, if
// there is one.
//
// See [DeclDef.Type] for details on where this keyword comes from.
func (d DeclDef) Keyword() keyword.Keyword {
	return d.KeywordToken().Keyword()
}

// KeywordToken returns the introducing keyword token for this definition, if
// there is one.
//
// See [DeclDef.Type] for details on where this keyword comes from.
func (d DeclDef) KeywordToken() token.Token {
	// Begin by removing all modifiers. Certain kinds of defs can have
	// modifiers, such as groups and types. Any def that can have a body
	// is permitted to have modifiers, because that is unambiguous with a field.
	mods := false
	ty := d.Type()
	for ty.Kind() == TypeKindPrefixed {
		mods = true
		ty = ty.AsPrefixed().Type()
	}

	path := ty.AsPath()
	if path.IsZero() {
		return token.Zero
	}

	ident := path.Path.AsIdent()
	switch ident.Keyword() {
	case keyword.Option:
		if !mods { // NOTE: Options with modifiers are treated as fields by protoc.
			return ident
		}
	case keyword.RPC:
		if !d.Signature().IsZero() {
			return ident
		}

	case keyword.Message, keyword.Enum, keyword.Service, keyword.Extend,
		keyword.Oneof, keyword.Group:
		if !d.Body().IsZero() {
			return ident
		}
	}
	return token.Zero
}

// Prefixes returns an iterator over the modifiers on this def, expressed as
// [TypePrefixed] nodes.
func (d DeclDef) Prefixes() iter.Seq[TypePrefixed] {
	return func(yield func(TypePrefixed) bool) {
		ty := d.Type()
		for ty.Kind() == TypeKindPrefixed {
			prefixed := ty.AsPrefixed()
			if !yield(prefixed) {
				break
			}
			ty = prefixed.Type()
		}
	}
}

// Name returns this definition's declared name.
func (d DeclDef) Name() Path {
	if d.IsZero() {
		return Path{}
	}

	return d.Raw().name.In(d.Context())
}

// Stem returns a span that contains both this definition's type and name.
//
// For e.g. a message, this is the "message Foo" part.
func (d DeclDef) Stem() source.Span {
	return source.Join(d.Type(), d.Name())
}

// Signature returns this definition's type signature, if it has one.
//
// Note that this is distinct from the type returned by [DeclDef.Type], which
// is the "prefix" type for the definition (such as for a field). This is a
// signature for e.g. a method.
//
// Not all defs have a signature, so this function may return a zero Signature.q
// If you want to add one, use [DeclDef.WithSignature].
func (d DeclDef) Signature() Signature {
	if d.IsZero() || d.Raw().signature == nil {
		return Signature{}
	}

	return Signature{
		id.WrapContext(d.Context()),
		d.Raw().signature,
	}
}

// WithSignature is like Signature, but it adds an empty signature if it would
// return zero.
func (d DeclDef) WithSignature() Signature {
	if !d.IsZero() && d.Signature().IsZero() {
		d.Raw().signature = new(rawSignature)
	}
	return d.Signature()
}

// Equals returns this definitions = token, before the value.
// May be zero.
func (d DeclDef) Equals() token.Token {
	if d.IsZero() {
		return token.Zero
	}

	return id.Wrap(d.Context().Stream(), d.Raw().equals)
}

// Value returns this definition's value. For a field, this will be the
// tag number, while for an option, this will be the complex expression
// representing its value.
func (d DeclDef) Value() ExprAny {
	if d.IsZero() {
		return ExprAny{}
	}

	return id.WrapDyn(d.Context(), d.Raw().value)
}

// SetValue sets the value of this definition.
//
// See [DeclDef.Value].
func (d DeclDef) SetValue(expr ExprAny) {
	d.Raw().value = expr.ID()
}

// Options returns the compact options list for this definition.
func (d DeclDef) Options() CompactOptions {
	if d.IsZero() {
		return CompactOptions{}
	}

	return id.Wrap(d.Context(), d.Raw().options)
}

// SetOptions sets the compact options list for this definition.
//
// Setting it to a zero Options clears it.
func (d DeclDef) SetOptions(opts CompactOptions) {
	d.Raw().options = opts.ID()
}

// Body returns this definition's body, if it has one.
func (d DeclDef) Body() DeclBody {
	if d.IsZero() {
		return DeclBody{}
	}

	return id.Wrap(d.Context(), d.Raw().body)
}

// SetBody sets the body for this definition.
func (d DeclDef) SetBody(b DeclBody) {
	d.Raw().body = b.ID()
}

// Semicolon returns the ending semicolon token for this definition.
// May be zero.
func (d DeclDef) Semicolon() token.Token {
	if d.IsZero() {
		return token.Zero
	}

	return id.Wrap(d.Context().Stream(), d.Raw().semi)
}

// IsCorrupt reports whether or not some part of the parser decided that this
// definition is not interpretable as any specific kind of definition.
func (d DeclDef) IsCorrupt() bool {
	return !d.IsZero() && d.Raw().corrupt
}

// the compiler to ignore it. See [DeclDef.IsCorrupt].
func (d DeclDef) MarkCorrupt() {
	d.Raw().corrupt = true
}

// AsMessage extracts the fields from this definition relevant to interpreting
// it as a message.
//
// The return value's fields may be zero if they are not present (in particular,
// Name will be zero if d.Name() is not an identifier).
//
// See [DeclDef.Classify].
func (d DeclDef) AsMessage() DefMessage {
	return DefMessage{
		Keyword: d.KeywordToken(),
		Name:    d.Name().AsIdent(),
		Body:    d.Body(),
		Decl:    d,
	}
}

// AsEnum extracts the fields from this definition relevant to interpreting
// it as an enum.
//
// The return value's fields may be zero if they are not present (in particular,
// Name will be zero if d.Name() is not an identifier).
//
// See [DeclDef.Classify].
func (d DeclDef) AsEnum() DefEnum {
	return DefEnum{
		Keyword: d.KeywordToken(),
		Name:    d.Name().AsIdent(),
		Body:    d.Body(),
		Decl:    d,
	}
}

// AsService extracts the fields from this definition relevant to interpreting
// it as a service.
//
// The return value's fields may be zero if they are not present (in particular,
// Name will be zero if d.Name() is not an identifier).
//
// See [DeclDef.Classify].
func (d DeclDef) AsService() DefService {
	return DefService{
		Keyword: d.KeywordToken(),
		Name:    d.Name().AsIdent(),
		Body:    d.Body(),
		Decl:    d,
	}
}

// AsExtend extracts the fields from this definition relevant to interpreting
// it as a service.
//
// The return value's fields may be zero if they are not present.
//
// See [DeclDef.Classify].
func (d DeclDef) AsExtend() DefExtend {
	return DefExtend{
		Keyword:  d.KeywordToken(),
		Extendee: d.Name(),
		Body:     d.Body(),
		Decl:     d,
	}
}

// AsField extracts the fields from this definition relevant to interpreting
// it as a message field.
//
// The return value's fields may be zero if they are not present (in particular,
// Name will be zero if d.Name() is not an identifier).
//
// See [DeclDef.Classify].
func (d DeclDef) AsField() DefField {
	return DefField{
		Type:      d.Type(),
		Name:      d.Name().AsIdent(),
		Equals:    d.Equals(),
		Tag:       d.Value(),
		Options:   d.Options(),
		Semicolon: d.Semicolon(),
		Decl:      d,
	}
}

// AsOneof extracts the fields from this definition relevant to interpreting
// it as a oneof.
//
// The return value's fields may be zero if they are not present (in particular,
// Name will be zero if d.Name() is not an identifier).
//
// See [DeclDef.Classify].
func (d DeclDef) AsOneof() DefOneof {
	return DefOneof{
		Keyword: d.KeywordToken(),
		Name:    d.Name().AsIdent(),
		Body:    d.Body(),
		Decl:    d,
	}
}

// AsGroup extracts the fields from this definition relevant to interpreting
// it as a group.
//
// The return value's fields may be zero if they are not present (in particular,
// Name will be zero if d.Name() is not an identifier).
//
// See [DeclDef.Classify].
func (d DeclDef) AsGroup() DefGroup {
	return DefGroup{
		Keyword: d.KeywordToken(),
		Name:    d.Name().AsIdent(),
		Equals:  d.Equals(),
		Tag:     d.Value(),
		Options: d.Options(),
		Decl:    d,
	}
}

// AsEnumValue extracts the fields from this definition relevant to interpreting
// it as an enum value.
//
// The return value's fields may be zero if they are not present (in particular,
// Name will be zero if d.Name() is not an identifier).
//
// See [DeclDef.Classify].
func (d DeclDef) AsEnumValue() DefEnumValue {
	return DefEnumValue{
		Name:      d.Name().AsIdent(),
		Equals:    d.Equals(),
		Tag:       d.Value(),
		Options:   d.Options(),
		Semicolon: d.Semicolon(),
		Decl:      d,
	}
}

// AsMethod extracts the fields from this definition relevant to interpreting
// it as a service method.
//
// The return value's fields may be zero if they are not present (in particular,
// Name will be zero if d.Name() is not an identifier).
//
// See [DeclDef.Classify].
func (d DeclDef) AsMethod() DefMethod {
	return DefMethod{
		Keyword:   d.KeywordToken(),
		Name:      d.Name().AsIdent(),
		Signature: d.Signature(),
		Body:      d.Body(),
		Decl:      d,
	}
}

// AsMethod extracts the fields from this definition relevant to interpreting
// it as an option.
//
// The return value's fields may be zero if they are not present.
//
// See [DeclDef.Classify].
func (d DeclDef) AsOption() DefOption {
	return DefOption{
		Keyword: d.KeywordToken(),
		Option: Option{
			Path:   d.Name(),
			Equals: d.Equals(),
			Value:  d.Value(),
		},
		Semicolon: d.Semicolon(),
		Decl:      d,
	}
}

// Classify looks at all the fields in this definition and decides what kind of
// definition it's supposed to represent.
//
// To select which definition this probably is, this function looks at
// [DeclDef.KeywordToken]. If there is no keyword or it isn't something that it
// recognizes, it is classified as either an enum value or a field, depending on
// whether this definition has a type.
//
// The correct way to use this function is as the input value for a switch. The
// cases of the switch should then use the As* methods, such as
// [DeclDef.AsMessage], to extract the relevant fields.
func (d DeclDef) Classify() DefKind {
	if d.IsZero() || d.IsCorrupt() {
		return DefKindInvalid
	}

	switch d.Keyword() {
	case keyword.Message:
		return DefKindMessage
	case keyword.Enum:
		return DefKindEnum
	case keyword.Service:
		return DefKindService
	case keyword.Extend:
		return DefKindExtend
	case keyword.Oneof:
		return DefKindOneof
	case keyword.Group:
		return DefKindGroup
	case keyword.RPC:
		return DefKindMethod
	case keyword.Option:
		return DefKindOption
	}

	if d.Type().IsZero() {
		return DefKindEnumValue
	}

	return DefKindField
}

// Span implements [source.Spanner].
func (d DeclDef) Span() source.Span {
	if d.IsZero() {
		return source.Span{}
	}

	return source.Join(
		d.Type(),
		d.Name(),
		d.Signature(),
		d.Equals(),
		d.Value(),
		d.Options(),
		d.Body(),
		d.Semicolon(),
	)
}

// Signature is a type signature of the form (types) returns (types).
//
// Signatures may have multiple inputs and outputs.
type Signature struct {
	withContext
	raw *rawSignature
}

type rawSignature struct {
	input, output rawTypeList
	returns       token.ID
}

// Returns returns (lol) the "returns" token that separates the input and output
// type lists.
func (s Signature) Returns() token.Token {
	if s.IsZero() {
		return token.Zero
	}

	return id.Wrap(s.Context().Stream(), s.raw.returns)
}

// Inputs returns the input argument list for this signature.
func (s Signature) Inputs() TypeList {
	if s.IsZero() {
		return TypeList{}
	}

	return TypeList{
		id.WrapContext(s.Context()),
		&s.raw.input,
	}
}

// Outputs returns the output argument list for this signature.
func (s Signature) Outputs() TypeList {
	if s.IsZero() {
		return TypeList{}
	}

	return TypeList{
		id.WrapContext(s.Context()),
		&s.raw.output,
	}
}

// Span implemented [source.Spanner].
func (s Signature) Span() source.Span {
	if s.IsZero() {
		return source.Span{}
	}

	return source.Join(s.Inputs(), s.Returns(), s.Outputs())
}

// Def is the return type of [DeclDef.Classify].
//
// This interface is implemented by all the Def* types in this package, and
// can be type-asserted to any of them, usually in a type switch.
//
// A [DeclDef] can't be mutated through a Def; instead, you will need to mutate
// the general structure instead.
type Def interface {
	source.Spanner

	isDef()
}

// DefMessage is a [DeclDef] projected into a message definition.
//
// See [DeclDef.Classify].
type DefMessage struct {
	Keyword token.Token
	Name    token.Token
	Body    DeclBody

	Decl DeclDef
}

func (DefMessage) isDef()              {}
func (d DefMessage) Span() source.Span { return d.Decl.Span() }
func (d DefMessage) Context() *File    { return d.Decl.Context() }

// DefEnum is a [DeclDef] projected into an enum definition.
//
// See [DeclDef.Classify].
type DefEnum struct {
	Keyword token.Token
	Name    token.Token
	Body    DeclBody

	Decl DeclDef
}

func (DefEnum) isDef()              {}
func (d DefEnum) Span() source.Span { return d.Decl.Span() }
func (d DefEnum) Context() *File    { return d.Decl.Context() }

// DefService is a [DeclDef] projected into a service definition.
//
// See [DeclDef.Classify].
type DefService struct {
	Keyword token.Token
	Name    token.Token
	Body    DeclBody

	Decl DeclDef
}

func (DefService) isDef()              {}
func (d DefService) Span() source.Span { return d.Decl.Span() }
func (d DefService) Context() *File    { return d.Decl.Context() }

// DefExtend is a [DeclDef] projected into an extension definition.
//
// See [DeclDef.Classify].
type DefExtend struct {
	Keyword  token.Token
	Extendee Path
	Body     DeclBody

	Decl DeclDef
}

func (DefExtend) isDef()              {}
func (d DefExtend) Span() source.Span { return d.Decl.Span() }
func (d DefExtend) Context() *File    { return d.Decl.Context() }

// DefField is a [DeclDef] projected into a field definition.
//
// See [DeclDef.Classify].
type DefField struct {
	Type      TypeAny
	Name      token.Token
	Equals    token.Token
	Tag       ExprAny
	Options   CompactOptions
	Semicolon token.Token

	Decl DeclDef
}

func (DefField) isDef()              {}
func (d DefField) Span() source.Span { return d.Decl.Span() }
func (d DefField) Context() *File    { return d.Decl.Context() }

// DefEnumValue is a [DeclDef] projected into an enum value definition.
//
// See [DeclDef.Classify].
type DefEnumValue struct {
	Name      token.Token
	Equals    token.Token
	Tag       ExprAny
	Options   CompactOptions
	Semicolon token.Token

	Decl DeclDef
}

func (DefEnumValue) isDef()              {}
func (d DefEnumValue) Span() source.Span { return d.Decl.Span() }
func (d DefEnumValue) Context() *File    { return d.Decl.Context() }

// DefEnumValue is a [DeclDef] projected into a oneof definition.
//
// See [DeclDef.Classify].
type DefOneof struct {
	Keyword token.Token
	Name    token.Token
	Body    DeclBody

	Decl DeclDef
}

func (DefOneof) isDef()              {}
func (d DefOneof) Span() source.Span { return d.Decl.Span() }
func (d DefOneof) Context() *File    { return d.Decl.Context() }

// DefGroup is a [DeclDef] projected into a group definition.
//
// See [DeclDef.Classify].
type DefGroup struct {
	Keyword token.Token
	Name    token.Token
	Equals  token.Token
	Tag     ExprAny
	Options CompactOptions
	Body    DeclBody

	Decl DeclDef
}

func (DefGroup) isDef()              {}
func (d DefGroup) Span() source.Span { return d.Decl.Span() }
func (d DefGroup) Context() *File    { return d.Decl.Context() }

// DefMethod is a [DeclDef] projected into a method definition.
//
// See [DeclDef.Classify].
type DefMethod struct {
	Keyword   token.Token
	Name      token.Token
	Signature Signature
	Body      DeclBody

	Decl DeclDef
}

func (DefMethod) isDef()              {}
func (d DefMethod) Span() source.Span { return d.Decl.Span() }
func (d DefMethod) Context() *File    { return d.Decl.Context() }

// DefOption is a [DeclDef] projected into a method definition.
//
// Yes, an option is technically not defining anything, just setting a value.
// However, it's syntactically analogous to a definition!
//
// See [DeclDef.Classify].
type DefOption struct {
	Option

	Keyword   token.Token
	Semicolon token.Token

	Decl DeclDef
}

func (DefOption) isDef()              {}
func (d DefOption) Span() source.Span { return d.Decl.Span() }
func (d DefOption) Context() *File    { return d.Decl.Context() }
