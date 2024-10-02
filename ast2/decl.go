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
	declRange
	declOption
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
	declKind() declKind
	// declIndex returns the index this declaration occupies in its owning
	// context. This is 0-indexed, and must be incremented
	declIndex() int
}

func (File) declKind() declKind    { return declFile }
func (Pragma) declKind() declKind  { return declPragma }
func (Package) declKind() declKind { return declPackage }
func (Import) declKind() declKind  { return declImport }
func (Message) declKind() declKind { return declMessage }
func (Enum) declKind() declKind    { return declEnum }
func (Extends) declKind() declKind { return declExtends }
func (Service) declKind() declKind { return declService }
func (Body) declKind() declKind    { return declBody }
func (Field) declKind() declKind   { return declField }
func (Method) declKind() declKind  { return declMethod }
func (Range) declKind() declKind   { return declRange }
func (Option) declKind() declKind  { return declOption }

type declKind int8

// decl is a typed reference to a declaration inside some Context.
//
// Note: decl indices are one-indexed, to allow for the zero value
// to represent nil.
type decl[T Decl] uint32

func declFor[T Decl](d T) decl[T] {
	if d.Context() == nil {
		return decl[T](0)
	}
	return decl[T](d.declIndex() + 1)
}

// Wrap wraps this declID with a context to present to the user.
func (d decl[T]) With(c Contextual) T {
	ctx := c.Context()

	var decl T
	if d == 0 {
		return decl
	}

	return decl.with(ctx, int(uint32(d)-1)).(T)
}

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
	case declRange:
		return Range{}
	case declOption:
		return Option{}
	default:
		return nil
	}
}
