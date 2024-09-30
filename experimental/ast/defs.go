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

// DefEnum is a [DeclDef] projected into an enum definition.
//
// See [DeclDef.Classify].
type DefEnum struct {
	Keyword Token
	Name    Token
	Body    DeclBody

	Decl DeclDef
}

// DefService is a [DeclDef] projected into a service definition.
//
// See [DeclDef.Classify]
type DefService struct {
	Keyword Token
	Name    Token
	Body    DeclBody

	Decl DeclDef
}

// DefExtend is a [DeclDef] projected into an extension definition.
//
// See [DeclDef.Classify].
type DefExtend struct {
	Keyword  Token
	Extendee Path
	Body     DeclBody

	Decl DeclDef
}

// DefField is a [DeclDef] projected into a field definition.
//
// See [DeclDef.Classify].
type DefField struct {
	Type      Type
	Name      Token
	Equals    Token
	Tag       Expr
	Options   Options
	Semicolon Token

	Decl DeclDef
}

// DefEnumValue is a [DeclDef] projected into an enum value definition.
//
// See [DeclDef.Classify].
type DefEnumValue struct {
	Name      Token
	Equals    Token
	Tag       Expr
	Options   Options
	Semicolon Token

	Decl DeclDef
}

// DefEnumValue is a [DeclDef] projected into a oneof definition.
//
// See [DeclDef.Classify].
type DefOneof struct {
	Keyword Token
	Name    Token
	Body    DeclBody

	Decl DeclDef
}

// DefGroup is a [DeclDef] projected into a group definition.
//
// See [DeclDef.Classify].
type DefGroup struct {
	Keyword Token
	Name    Token
	Equals  Token
	Tag     Expr
	Options Options
	Body    DeclBody

	Decl DeclDef
}

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

func (DefMessage) isDef()   {}
func (DefEnum) isDef()      {}
func (DefService) isDef()   {}
func (DefExtend) isDef()    {}
func (DefField) isDef()     {}
func (DefEnumValue) isDef() {}
func (DefOneof) isDef()     {}
func (DefGroup) isDef()     {}
func (DefMethod) isDef()    {}
func (DefOption) isDef()    {}

func (d DefMessage) Span() Span   { return d.Decl.Span() }
func (d DefEnum) Span() Span      { return d.Decl.Span() }
func (d DefService) Span() Span   { return d.Decl.Span() }
func (d DefExtend) Span() Span    { return d.Decl.Span() }
func (d DefField) Span() Span     { return d.Decl.Span() }
func (d DefEnumValue) Span() Span { return d.Decl.Span() }
func (d DefOneof) Span() Span     { return d.Decl.Span() }
func (d DefGroup) Span() Span     { return d.Decl.Span() }
func (d DefMethod) Span() Span    { return d.Decl.Span() }
func (d DefOption) Span() Span    { return d.Decl.Span() }

func (d DefMessage) Context() *Context   { return d.Decl.Context() }
func (d DefEnum) Context() *Context      { return d.Decl.Context() }
func (d DefService) Context() *Context   { return d.Decl.Context() }
func (d DefExtend) Context() *Context    { return d.Decl.Context() }
func (d DefField) Context() *Context     { return d.Decl.Context() }
func (d DefEnumValue) Context() *Context { return d.Decl.Context() }
func (d DefOneof) Context() *Context     { return d.Decl.Context() }
func (d DefGroup) Context() *Context     { return d.Decl.Context() }
func (d DefMethod) Context() *Context    { return d.Decl.Context() }
func (d DefOption) Context() *Context    { return d.Decl.Context() }
