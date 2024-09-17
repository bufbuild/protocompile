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

const (
	declFile declKind = iota + 1
	declPragma
	declPackage
	declImport
	declMessage
	declEnum
	declExtends
	declService
	declBody
	declField
	declMethod
)

// Decl is a Protobuf declaration.
//
// This includes things like imports, messages, fields, and options.
type Decl interface {
	Spanner

	// with should be called on a nil value of this type (not
	// a nil interface) and return the corresponding value of this type
	// extracted from the given context and index.
	//
	// Not to be called directly; see rawDecl[T].With().
	with(ctx *Context, idx int) Decl

	// kind returns what kind of decl this is.
	kind() declKind
}

func (File) kind() declKind    { return declFile }
func (Pragma) kind() declKind  { return declPragma }
func (Package) kind() declKind { return declPackage }
func (Import) kind() declKind  { return declImport }
func (Message) kind() declKind { return declMessage }
func (Enum) kind() declKind    { return declEnum }
func (Extends) kind() declKind { return declExtends }
func (Service) kind() declKind { return declService }
func (Body) kind() declKind    { return declBody }
func (Field) kind() declKind   { return declField }
func (Method) kind() declKind  { return declMethod }

// declID is a reference to a declaration inside some Context.
type rawDecl[_ Decl] uint32

// Wrap wraps this declID with a context to present to the user.
func (d rawDecl[T]) With(c Contextual) T {
	ctx := c.Context()

	var decl T
	return decl.with(ctx, int(uint32(d))).(T)
}

type declKind int8

// reify returns the corresponding nil Decl for the given kind,
// such that k.reify().kind() == k.
func (k declKind) reify() Decl {
	switch k {
	case declPragma:
		return Pragma{}
	case declPackage:
		return Package{}
	case declImport:
		return Import{}
	case declMessage:
		return Message{}
	case declEnum:
		return Enum{}
	case declExtends:
		return Extends{}
	case declService:
		return Service{}
	case declBody:
		return Body{}
	case declField:
		return Field{}
	case declMethod:
		return Method{}
	default:
		return nil
	}
}
