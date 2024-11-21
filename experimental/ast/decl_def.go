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
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/internal/arena"
)

const (
	DefKindMessage DefKind = iota + 1
	DefKindEnum
	DefKindService
	DefKindExtend
	DefKindField
	DefKindOneof
	DefKindGroup
	DefKindEnumValue
	DefKindMethod
	DefKindOption
)

// DefKind is the kind of definition a [DeclDef] contains.
//
// See [DeclDef.Classify].
type DefKind int8

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
type DeclDef struct{ declImpl[rawDeclDef] }

type rawDeclDef struct {
	ty   rawType // Not present for enum fields.
	name rawPath

	signature *rawSignature

	equals token.ID
	value  rawExpr

	options arena.Pointer[rawCompactOptions]
	body    arena.Pointer[rawDeclBody]
	semi    token.ID
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

// Type returns the "prefix" type of this definition.
//
// This type may coexist with a [Signature] in this definition.
//
// May be nil, such as for enum values. For messages and other productions
// introduced by a special keyword, this will be a [TypePath] whose single
// identifier is that keyword.
//
// See [DeclDef.Keyword].
func (d DeclDef) Type() TypeAny {
	return d.raw.ty.With(d.Context())
}

// SetType sets the "prefix" type of this definition.
func (d DeclDef) SetType(ty TypeAny) {
	d.raw.ty = ty.raw
}

// Keyword returns the introducing keyword for this definition, if
// there is one.
//
// See [DeclDef.Type] for details on where this keyword comes from.
func (d DeclDef) Keyword() token.Token {
	path := d.Type().AsPath()
	if path.Nil() {
		return token.Nil
	}

	ident := path.Path.AsIdent()
	switch ident.Text() {
	case "message", "enum", "service", "extend", "oneof", "group", "rpc", "option":
		return ident
	default:
		return token.Nil
	}
}

// Name returns this definition's declared name.
func (d DeclDef) Name() Path {
	return d.raw.name.With(d.Context())
}

// Signature returns this definition's type signature, if it has one.
//
// Note that this is distinct from the type returned by [DeclDef.Type], which
// is the "prefix" type for the definition (such as for a field). This is a
// signature for e.g. a method.
//
// Not all defs have a signature, so this function may return a nil Signature.
// If you want to add one, use [DeclDef.WithSignature].
func (d DeclDef) Signature() Signature {
	if d.raw.signature == nil {
		return Signature{}
	}

	return Signature{
		d.withContext,
		d.raw.signature,
	}
}

// WithSignature is like Signature, but it adds an empty signature if it would
// return nil.
func (d DeclDef) WithSignature() Signature {
	if d.Signature().Nil() {
		d.raw.signature = new(rawSignature)
	}
	return d.Signature()
}

// Equals returns this definitions = token, before the value.
// May be nil.
func (d DeclDef) Equals() token.Token {
	return d.raw.equals.In(d.Context())
}

// Value returns this definition's value. For a field, this will be the
// tag number, while for an option, this will be the complex expression
// representing its value.
func (d DeclDef) Value() ExprAny {
	return d.raw.value.With(d.Context())
}

// SetValue sets the value of this definition.
//
// See [DeclDef.Value].
func (d DeclDef) SetValue(expr ExprAny) {
	d.raw.value = expr.raw
}

// Options returns the compact options list for this definition.
func (d DeclDef) Options() CompactOptions {
	return wrapOptions(d.Context(), d.raw.options)
}

// SetOptions sets the compact options list for this definition.
//
// Setting it to a nil Options clears it.
func (d DeclDef) SetOptions(opts CompactOptions) {
	d.raw.options = d.Context().Nodes().options.Compress(opts.raw)
}

// Body returns this definition's body, if it has one.
func (d DeclDef) Body() DeclBody {
	return wrapDeclBody(d.Context(), d.raw.body)
}

// SetBody sets the body for this definition.
func (d DeclDef) SetBody(b DeclBody) {
	d.raw.body = d.Context().Nodes().decls.bodies.Compress(b.raw)
}

// Semicolon returns the ending semicolon token for this definition.
// May be nil.
func (d DeclDef) Semicolon() token.Token {
	return d.raw.semi.In(d.Context())
}

// AsMessage extracts the fields from this definition relevant to interpreting
// it as a message.
//
// The return value's fields may be nil if they are not present (in particular,
// Name will be nil if d.Name() is not an identifier).
//
// See [DeclDef.Classify].
func (d DeclDef) AsMessage() DefMessage {
	return DefMessage{
		Keyword: d.Keyword(),
		Name:    d.Name().AsIdent(),
		Body:    d.Body(),
		Decl:    d,
	}
}

// AsEnum extracts the fields from this definition relevant to interpreting
// it as an enum.
//
// The return value's fields may be nil if they are not present (in particular,
// Name will be nil if d.Name() is not an identifier).
//
// See [DeclDef.Classify].
func (d DeclDef) AsEnum() DefEnum {
	return DefEnum{
		Keyword: d.Keyword(),
		Name:    d.Name().AsIdent(),
		Body:    d.Body(),
		Decl:    d,
	}
}

// AsService extracts the fields from this definition relevant to interpreting
// it as a service.
//
// The return value's fields may be nil if they are not present (in particular,
// Name will be nil if d.Name() is not an identifier).
//
// See [DeclDef.Classify].
func (d DeclDef) AsService() DefService {
	return DefService{
		Keyword: d.Keyword(),
		Name:    d.Name().AsIdent(),
		Body:    d.Body(),
		Decl:    d,
	}
}

// AsExtend extracts the fields from this definition relevant to interpreting
// it as a service.
//
// The return value's fields may be nil if they are not present.
//
// See [DeclDef.Classify].
func (d DeclDef) AsExtend() DefExtend {
	return DefExtend{
		Keyword:  d.Keyword(),
		Extendee: d.Name(),
		Body:     d.Body(),
		Decl:     d,
	}
}

// AsField extracts the fields from this definition relevant to interpreting
// it as a message field.
//
// The return value's fields may be nil if they are not present (in particular,
// Name will be nil if d.Name() is not an identifier).
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
// The return value's fields may be nil if they are not present (in particular,
// Name will be nil if d.Name() is not an identifier).
//
// See [DeclDef.Classify].
func (d DeclDef) AsOneof() DefOneof {
	return DefOneof{
		Keyword: d.Keyword(),
		Name:    d.Name().AsIdent(),
		Body:    d.Body(),
		Decl:    d,
	}
}

// AsGroup extracts the fields from this definition relevant to interpreting
// it as a group.
//
// The return value's fields may be nil if they are not present (in particular,
// Name will be nil if d.Name() is not an identifier).
//
// See [DeclDef.Classify].
func (d DeclDef) AsGroup() DefGroup {
	return DefGroup{
		Keyword: d.Keyword(),
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
// The return value's fields may be nil if they are not present (in particular,
// Name will be nil if d.Name() is not an identifier).
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
// The return value's fields may be nil if they are not present (in particular,
// Name will be nil if d.Name() is not an identifier).
//
// See [DeclDef.Classify].
func (d DeclDef) AsMethod() DefMethod {
	return DefMethod{
		Keyword:   d.Keyword(),
		Name:      d.Name().AsIdent(),
		Signature: d.Signature(),
		Body:      d.Body(),
		Decl:      d,
	}
}

// AsMethod extracts the fields from this definition relevant to interpreting
// it as an option.
//
// The return value's fields may be nil if they are not present.
//
// See [DeclDef.Classify].
func (d DeclDef) AsOption() DefOption {
	return DefOption{
		Keyword: d.Keyword(),
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
// [DeclDef.Keyword]. If there is no keyword or it isn't something that it
// recognizes, it is classified as either an enum value or a field, depending on
// whether this definition has a type.
//
// The correct way to use this function is as the input value for a switch. The
// cases of the switch should then use the As* methods, such as
// [DeclDef.AsMessage], to extract the relevant fields.
func (d DeclDef) Classify() DefKind {
	switch d.Keyword().Text() {
	case "message":
		if !d.Body().Nil() {
			return DefKindMessage
		}
	case "enum":
		if !d.Body().Nil() {
			return DefKindEnum
		}
	case "service":
		if !d.Body().Nil() {
			return DefKindService
		}
	case "extend":
		if !d.Body().Nil() {
			return DefKindExtend
		}
	case "oneof":
		if !d.Body().Nil() {
			return DefKindOneof
		}
	case "group":
		if !d.Body().Nil() {
			return DefKindGroup
		}
	case "rpc":
		if !d.Signature().Nil() {
			return DefKindMethod
		}
	case "option":
		return DefKindOption
	}

	if d.Type().Nil() {
		return DefKindEnumValue
	}

	return DefKindField
}

// Span implements [report.Spanner].
func (d DeclDef) Span() report.Span {
	if d.Nil() {
		return report.Span{}
	}
	return report.Join(
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

func wrapDeclDef(c Context, ptr arena.Pointer[rawDeclDef]) DeclDef {
	return DeclDef{wrapDecl(c, ptr)}
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
	return s.raw.returns.In(s.Context())
}

// Inputs returns the input argument list for this signature.
func (s Signature) Inputs() TypeList {
	return TypeList{
		s.withContext,
		&s.raw.input,
	}
}

// Outputs returns the output argument list for this signature.
func (s Signature) Outputs() TypeList {
	return TypeList{
		s.withContext,
		&s.raw.output,
	}
}

// Span implemented [report.Spanner].
func (s Signature) Span() report.Span {
	return report.Join(s.Inputs(), s.Returns(), s.Outputs())
}

// Def is the return type of [DeclDef.Classify].
//
// This interface is implemented by all the Def* types in this package, and
// can be type-asserted to any of them, usually in a type switch.
//
// A [DeclDef] can't be mutated through a Def; instead, you will need to mutate
// the general structure instead.
type Def interface {
	report.Spanner

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
func (d DefMessage) Span() report.Span { return d.Decl.Span() }
func (d DefMessage) Context() Context  { return d.Decl.Context() }

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
func (d DefEnum) Span() report.Span { return d.Decl.Span() }
func (d DefEnum) Context() Context  { return d.Decl.Context() }

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
func (d DefService) Span() report.Span { return d.Decl.Span() }
func (d DefService) Context() Context  { return d.Decl.Context() }

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
func (d DefExtend) Span() report.Span { return d.Decl.Span() }
func (d DefExtend) Context() Context  { return d.Decl.Context() }

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
func (d DefField) Span() report.Span { return d.Decl.Span() }
func (d DefField) Context() Context  { return d.Decl.Context() }

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
func (d DefEnumValue) Span() report.Span { return d.Decl.Span() }
func (d DefEnumValue) Context() Context  { return d.Decl.Context() }

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
func (d DefOneof) Span() report.Span { return d.Decl.Span() }
func (d DefOneof) Context() Context  { return d.Decl.Context() }

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
func (d DefGroup) Span() report.Span { return d.Decl.Span() }
func (d DefGroup) Context() Context  { return d.Decl.Context() }

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
func (d DefMethod) Span() report.Span { return d.Decl.Span() }
func (d DefMethod) Context() Context  { return d.Decl.Context() }

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
func (d DefOption) Span() report.Span { return d.Decl.Span() }
func (d DefOption) Context() Context  { return d.Decl.Context() }
