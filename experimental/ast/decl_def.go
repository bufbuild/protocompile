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

import "github.com/bufbuild/protocompile/internal/arena"

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
type DeclDef struct {
	withContext

	ptr arena.Pointer[rawDeclDef]
	raw *rawDeclDef
}

type rawDeclDef struct {
	ty   rawType // Not present for enum fields.
	name rawPath

	signature *rawSignature

	equals rawToken
	value  rawExpr

	options arena.Pointer[rawCompactOptions]
	body    arena.Pointer[rawDeclBody]
	semi    rawToken
}

// DeclDefArgs is arguments for creating a [DeclDef] with [Context.NewDeclDef].
type DeclDefArgs struct {
	// If both Keyword and Type are set, Type will be prioritized.
	Keyword Token
	Type    Type
	Name    Path

	// NOTE: the values for the type signature are not provided at
	// construction time, and should be added by mutating through
	// DeclDef.Signature.
	Returns Token

	Equals Token
	Value  Expr

	Options CompactOptions

	Body      DeclBody
	Semicolon Token
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
func (d DeclDef) Type() Type {
	return d.raw.ty.With(d)
}

// SetType sets the "prefix" type of this definition.
func (d DeclDef) SetType(ty Type) {
	d.raw.ty = toRawType(ty)
}

// Keyword returns the introducing keyword for this definition, if
// there is one.
//
// See [DeclDef.Type] for details on where this keyword comes from.
func (d DeclDef) Keyword() Token {
	path, ok := d.Type().(TypePath)
	if !ok {
		return Token{}
	}
	ident := path.Path.AsIdent()
	switch ident.Text() {
	case "message", "enum", "service", "extend", "oneof", "group", "rpc", "option":
		return ident
	default:
		return Token{}
	}
}

// Name returns this definition's declared name.
func (d DeclDef) Name() Path {
	return d.raw.name.With(d)
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
func (d DeclDef) Equals() Token {
	return d.raw.equals.With(d)
}

// Value returns this definition's value. For a field, this will be the
// tag number, while for an option, this will be the complex expression
// representing its value.
func (d DeclDef) Value() Expr {
	return d.raw.value.With(d)
}

// SetValue sets the value of this definition.
//
// See [DeclDef.Value].
func (d DeclDef) SetValue(expr Expr) {
	d.raw.value = toRawExpr(expr)
}

// Options returns the compact options list for this definition.
func (d DeclDef) Options() CompactOptions {
	return wrapOptions(d, d.raw.options)
}

// SetOptions sets the compact options list for this definition.
//
// Setting it to a nil Options clears it.
func (d DeclDef) SetOptions(opts CompactOptions) {
	d.raw.options = opts.ptr
}

// Body returns this definition's body, if it has one.
func (d DeclDef) Body() DeclBody {
	return wrapDeclBody(d, d.raw.body)
}

// SetBody sets the body for this definition.
func (d DeclDef) SetBody(b DeclBody) {
	d.raw.body = b.ptr
}

// Semicolon returns the ending semicolon token for this definition.
// May be nil.
func (d DeclDef) Semicolon() Token {
	return d.raw.semi.With(d)
}

// Classify looks at all the fields in this definition and decides what kind of
// definition it's supposed to represent.
//
// For nonsensical definitions, this returns nil, although it is not guaranteed
// to return nil for *all* nonsensical definitions.
func (d DeclDef) Classify() Def {
	kw := d.Keyword()
	nameID := d.Name().AsIdent()

	eq := d.Equals()
	value := d.Value()
	noValue := eq.Nil() && value == nil

	switch text := kw.Text(); text {
	case "message", "enum", "service", "extend", "oneof":
		if (!nameID.Nil() || text == "extend") && noValue &&
			d.Signature().Nil() && d.Options().Nil() && !d.Body().Nil() {
			switch text {
			case "message":
				return DefMessage{
					Keyword: kw,
					Name:    nameID,
					Body:    d.Body(),
					Decl:    d,
				}
			case "enum":
				return DefEnum{
					Keyword: kw,
					Name:    nameID,
					Body:    d.Body(),
					Decl:    d,
				}
			case "service":
				return DefService{
					Keyword: kw,
					Name:    nameID,
					Body:    d.Body(),
					Decl:    d,
				}
			case "oneof":
				return DefOneof{
					Keyword: kw,
					Name:    nameID,
					Body:    d.Body(),
					Decl:    d,
				}
			case "extend":
				return DefExtend{
					Keyword:  kw,
					Extendee: d.Name(),
					Body:     d.Body(),
					Decl:     d,
				}
			}
		}
	case "group":
		if !nameID.Nil() && d.Signature().Nil() && value != nil {
			return DefGroup{
				Keyword: kw,
				Name:    nameID,
				Equals:  eq,
				Tag:     value,
				Options: d.Options(),
				Body:    d.Body(),
				Decl:    d,
			}
		}
	case "option":
		if value != nil && d.Signature().Nil() && d.Options().Nil() && d.Body().Nil() {
			return DefOption{
				Keyword: kw,
				Option: Option{
					Path:   d.Name(),
					Equals: eq,
					Value:  value,
				},
				Semicolon: d.Semicolon(),
				Decl:      d,
			}
		}
	case "rpc":
		if !nameID.Nil() && noValue && !d.Signature().Nil() && d.Options().Nil() {
			return DefMethod{
				Keyword:   kw,
				Name:      nameID,
				Signature: d.Signature(),
				Body:      d.Body(),
				Decl:      d,
			}
		}
	}

	// At this point, having complex path, a signature or a body is invalid.
	if nameID.Nil() || !d.Signature().Nil() || !d.Body().Nil() {
		return nil
	}

	if d.Type() == nil {
		return DefEnumValue{
			Name:      nameID,
			Equals:    eq,
			Tag:       value,
			Options:   d.Options(),
			Semicolon: d.Semicolon(),
			Decl:      d,
		}
	}

	return DefField{
		Type:      d.Type(),
		Name:      nameID,
		Equals:    eq,
		Tag:       value,
		Options:   d.Options(),
		Semicolon: d.Semicolon(),
		Decl:      d,
	}
}

// Span implements [Spanner].
func (d DeclDef) Span() Span {
	if d.Nil() {
		return Span{}
	}
	return JoinSpans(
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

func (d DeclDef) declRaw() (declKind, arena.Untyped) {
	return declDef, arena.Untyped(d.ptr)
}

func wrapDeclDef(c Contextual, ptr arena.Pointer[rawDeclDef]) DeclDef {
	ctx := c.Context()
	if ctx == nil || ptr.Nil() {
		return DeclDef{}
	}

	return DeclDef{
		withContext{ctx},
		ptr,
		ctx.decls.defs.Deref(ptr),
	}
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
	returns       rawToken
}

// Returns returns (lol) the "returns" token that separates the input and output
// type lists.
func (s Signature) Returns() Token {
	return s.raw.returns.With(s)
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

// Span implemented [Spanner].
func (s Signature) Span() Span {
	return JoinSpans(s.Inputs(), s.Returns(), s.Outputs())
}

// Def is the return type of [DeclDef.Classify].
//
// This interface is implemented by all the Def* types in this package, and
// can be type-asserted to any of them, usually in a type switch.
//
// A [DeclDef] can't be mutated through a Def; instead, you will need to mutate
// the general structure instead.
type Def interface {
	Spanner

	isDef()
}

// DefMessage is a [DeclDef] projected into a message definition.
//
// See [DeclDef.Classify].
type DefMessage struct {
	Keyword Token
	Name    Token
	Body    DeclBody

	Decl DeclDef
}

func (DefMessage) isDef()              {}
func (d DefMessage) Span() Span        { return d.Decl.Span() }
func (d DefMessage) Context() *Context { return d.Decl.Context() }

// DefEnum is a [DeclDef] projected into an enum definition.
//
// See [DeclDef.Classify].
type DefEnum struct {
	Keyword Token
	Name    Token
	Body    DeclBody

	Decl DeclDef
}

func (DefEnum) isDef()              {}
func (d DefEnum) Span() Span        { return d.Decl.Span() }
func (d DefEnum) Context() *Context { return d.Decl.Context() }

// DefService is a [DeclDef] projected into a service definition.
//
// See [DeclDef.Classify].
type DefService struct {
	Keyword Token
	Name    Token
	Body    DeclBody

	Decl DeclDef
}

func (DefService) isDef()              {}
func (d DefService) Span() Span        { return d.Decl.Span() }
func (d DefService) Context() *Context { return d.Decl.Context() }

// DefExtend is a [DeclDef] projected into an extension definition.
//
// See [DeclDef.Classify].
type DefExtend struct {
	Keyword  Token
	Extendee Path
	Body     DeclBody

	Decl DeclDef
}

func (DefExtend) isDef()              {}
func (d DefExtend) Span() Span        { return d.Decl.Span() }
func (d DefExtend) Context() *Context { return d.Decl.Context() }

// DefField is a [DeclDef] projected into a field definition.
//
// See [DeclDef.Classify].
type DefField struct {
	Type      Type
	Name      Token
	Equals    Token
	Tag       Expr
	Options   CompactOptions
	Semicolon Token

	Decl DeclDef
}

func (DefField) isDef()              {}
func (d DefField) Span() Span        { return d.Decl.Span() }
func (d DefField) Context() *Context { return d.Decl.Context() }

// DefEnumValue is a [DeclDef] projected into an enum value definition.
//
// See [DeclDef.Classify].
type DefEnumValue struct {
	Name      Token
	Equals    Token
	Tag       Expr
	Options   CompactOptions
	Semicolon Token

	Decl DeclDef
}

func (DefEnumValue) isDef()              {}
func (d DefEnumValue) Span() Span        { return d.Decl.Span() }
func (d DefEnumValue) Context() *Context { return d.Decl.Context() }

// DefEnumValue is a [DeclDef] projected into a oneof definition.
//
// See [DeclDef.Classify].
type DefOneof struct {
	Keyword Token
	Name    Token
	Body    DeclBody

	Decl DeclDef
}

func (DefOneof) isDef()              {}
func (d DefOneof) Span() Span        { return d.Decl.Span() }
func (d DefOneof) Context() *Context { return d.Decl.Context() }

// DefGroup is a [DeclDef] projected into a group definition.
//
// See [DeclDef.Classify].
type DefGroup struct {
	Keyword Token
	Name    Token
	Equals  Token
	Tag     Expr
	Options CompactOptions
	Body    DeclBody

	Decl DeclDef
}

func (DefGroup) isDef()              {}
func (d DefGroup) Span() Span        { return d.Decl.Span() }
func (d DefGroup) Context() *Context { return d.Decl.Context() }

// DefMethod is a [DeclDef] projected into a method definition.
//
// See [DeclDef.Classify].
type DefMethod struct {
	Keyword   Token
	Name      Token
	Signature Signature
	Body      DeclBody

	Decl DeclDef
}

func (DefMethod) isDef()              {}
func (d DefMethod) Span() Span        { return d.Decl.Span() }
func (d DefMethod) Context() *Context { return d.Decl.Context() }

// DefOption is a [DeclDef] projected into a method definition.
//
// Yes, an option is technically not defining anything, just setting a value.
// However, it's syntactically analogous to a definition!
//
// See [DeclDef.Classify].
type DefOption struct {
	Option

	Keyword   Token
	Semicolon Token

	Decl DeclDef
}

func (DefOption) isDef()              {}
func (d DefOption) Span() Span        { return d.Decl.Span() }
func (d DefOption) Context() *Context { return d.Decl.Context() }
