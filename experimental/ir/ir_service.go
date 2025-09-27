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

package ir

import (
	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/internal"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/internal/arena"
	"github.com/bufbuild/protocompile/internal/intern"
)

// Service is a Protobuf service definition.
type Service struct {
	withContext

	raw *rawService
}

// Method is a Protobuf service method.
type Method struct {
	withContext

	raw *rawMethod
}

type rawService struct {
	def       ast.DeclDef
	fqn, name intern.ID

	methods  []arena.Pointer[rawMethod]
	options  arena.Pointer[rawValue]
	features arena.Pointer[rawFeatureSet]
}

type rawMethod struct {
	def           ast.DeclDef
	fqn, name     intern.ID
	service       arena.Pointer[rawService]
	input, output ref[rawType]
	options       arena.Pointer[rawValue]
	features      arena.Pointer[rawFeatureSet]

	inputStream, outputStream bool
}

// AST returns the declaration for this service, if known.
func (s Service) AST() ast.DeclDef {
	if s.IsZero() {
		return ast.DeclDef{}
	}
	return s.raw.def
}

// Name returns this service's declared name, i.e. the last component of its
// full name.
func (s Service) Name() string {
	return s.FullName().Name()
}

// FullName returns this service's fully-qualified name.
func (s Service) FullName() FullName {
	if s.IsZero() {
		return ""
	}
	return FullName(s.Context().session.intern.Value(s.raw.fqn))
}

// InternedName returns the intern ID for [Service.FullName]().Name().
func (s Service) InternedName() intern.ID {
	if s.IsZero() {
		return 0
	}
	return s.raw.name
}

// InternedFullName returns the intern ID for [Service.FullName].
func (s Service) InternedFullName() intern.ID {
	if s.IsZero() {
		return 0
	}
	return s.raw.fqn
}

// Options returns the options applied to this service.
func (s Service) Options() MessageValue {
	if s.IsZero() {
		return MessageValue{}
	}
	return wrapValue(s.Context(), s.raw.options).AsMessage()
}

// FeatureSet returns the Editions features associated with this service.
func (s Service) FeatureSet() FeatureSet {
	if s.IsZero() || s.raw.features.Nil() {
		return FeatureSet{}
	}

	return FeatureSet{
		internal.NewWith(s.Context()),
		s.Context().arenas.features.Deref(s.raw.features),
	}
}

// Methods returns the methods of this service.
func (s Service) Methods() seq.Indexer[Method] {
	var methods []arena.Pointer[rawMethod]
	if !s.IsZero() {
		methods = s.raw.methods
	}

	return seq.NewFixedSlice(
		methods,
		func(_ int, p arena.Pointer[rawMethod]) Method {
			return Method{
				s.withContext,
				s.Context().arenas.methods.Deref(p),
			}
		},
	)
}

// AST returns the declaration for this method, if known.
func (m Method) AST() ast.DeclDef {
	if m.IsZero() {
		return ast.DeclDef{}
	}
	return m.raw.def
}

// Name returns this method's declared name, i.e. the last component of its
// full name.
func (m Method) Name() string {
	return m.FullName().Name()
}

// FullName returns this method's fully-qualified name.
func (m Method) FullName() FullName {
	if m.IsZero() {
		return ""
	}
	return FullName(m.Context().session.intern.Value(m.raw.fqn))
}

// InternedName returns the intern ID for [Method.FullName]().Name().
func (m Method) InternedName() intern.ID {
	if m.IsZero() {
		return 0
	}
	return m.raw.name
}

// InternedFullName returns the intern ID for [Method.FullName].
func (m Method) InternedFullName() intern.ID {
	if m.IsZero() {
		return 0
	}
	return m.raw.fqn
}

// Options returns the options applied to this method.
func (m Method) Options() MessageValue {
	if m.IsZero() {
		return MessageValue{}
	}
	return wrapValue(m.Context(), m.raw.options).AsMessage()
}

// FeatureSet returns the Editions features associated with this method.
func (m Method) FeatureSet() FeatureSet {
	if m.IsZero() || m.raw.features.Nil() {
		return FeatureSet{}
	}

	return FeatureSet{
		internal.NewWith(m.Context()),
		m.Context().arenas.features.Deref(m.raw.features),
	}
}

// Service returns the service this method is part of.
func (m Method) Service() Service {
	if m.IsZero() {
		return Service{}
	}
	return Service{
		m.withContext,
		m.Context().arenas.services.Deref(m.raw.service),
	}
}

// Input returns the input type for this method, and whether it is a streaming
// input.
func (m Method) Input() (ty Type, stream bool) {
	if m.IsZero() {
		return Type{}, false
	}

	return wrapType(m.Context(), m.raw.input), m.raw.inputStream
}

// Output returns the output type for this method, and whether it is a streaming
// output.
func (m Method) Output() (ty Type, stream bool) {
	if m.IsZero() {
		return Type{}, false
	}

	return wrapType(m.Context(), m.raw.output), m.raw.outputStream
}
