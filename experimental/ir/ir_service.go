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
	"github.com/bufbuild/protocompile/experimental/id"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/internal/intern"
)

// Service is a Protobuf service definition.
type Service id.Node[Service, *File, *rawService]

// Method is a Protobuf service method.
type Method id.Node[Method, *File, *rawMethod]

type rawService struct {
	def       id.ID[ast.DeclDef]
	fqn, name intern.ID

	methods  []id.ID[Method]
	options  id.ID[Value]
	features id.ID[FeatureSet]
}

type rawMethod struct {
	def           id.ID[ast.DeclDef]
	fqn, name     intern.ID
	service       id.ID[Service]
	input, output Ref[Type]
	options       id.ID[Value]
	features      id.ID[FeatureSet]

	inputStream, outputStream bool
}

// AST returns the declaration for this service, if known.
func (s Service) AST() ast.DeclDef {
	if s.IsZero() {
		return ast.DeclDef{}
	}
	return id.Wrap(s.Context().AST(), s.Raw().def)
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
	return FullName(s.Context().session.intern.Value(s.Raw().fqn))
}

// InternedName returns the intern ID for [Service.FullName]().Name().
func (s Service) InternedName() intern.ID {
	if s.IsZero() {
		return 0
	}
	return s.Raw().name
}

// InternedFullName returns the intern ID for [Service.FullName].
func (s Service) InternedFullName() intern.ID {
	if s.IsZero() {
		return 0
	}
	return s.Raw().fqn
}

// Options returns the options applied to this service.
func (s Service) Options() MessageValue {
	if s.IsZero() {
		return MessageValue{}
	}
	return id.Wrap(s.Context(), s.Raw().options).AsMessage()
}

// FeatureSet returns the Editions features associated with this service.
func (s Service) FeatureSet() FeatureSet {
	if s.IsZero() {
		return FeatureSet{}
	}
	return id.Wrap(s.Context(), s.Raw().features)
}

// Deprecated returns whether this service is deprecated, by returning the
// relevant option value for setting deprecation.
func (s Service) Deprecated() Value {
	if s.IsZero() {
		return Value{}
	}
	builtins := s.Context().builtins()
	d := s.Options().Field(builtins.ServiceDeprecated)
	if b, _ := d.AsBool(); b {
		return d
	}
	return Value{}
}

// Methods returns the methods of this service.
func (s Service) Methods() seq.Indexer[Method] {
	var methods []id.ID[Method]
	if !s.IsZero() {
		methods = s.Raw().methods
	}

	return seq.NewFixedSlice(
		methods,
		func(_ int, p id.ID[Method]) Method {
			return id.Wrap(s.Context(), p)
		},
	)
}

// AST returns the declaration for this method, if known.
func (m Method) AST() ast.DeclDef {
	if m.IsZero() {
		return ast.DeclDef{}
	}
	return id.Wrap(m.Context().AST(), m.Raw().def)
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
	return FullName(m.Context().session.intern.Value(m.Raw().fqn))
}

// InternedName returns the intern ID for [Method.FullName]().Name().
func (m Method) InternedName() intern.ID {
	if m.IsZero() {
		return 0
	}
	return m.Raw().name
}

// InternedFullName returns the intern ID for [Method.FullName].
func (m Method) InternedFullName() intern.ID {
	if m.IsZero() {
		return 0
	}
	return m.Raw().fqn
}

// Options returns the options applied to this method.
func (m Method) Options() MessageValue {
	if m.IsZero() {
		return MessageValue{}
	}
	return id.Wrap(m.Context(), m.Raw().options).AsMessage()
}

// FeatureSet returns the Editions features associated with this method.
func (m Method) FeatureSet() FeatureSet {
	if m.IsZero() {
		return FeatureSet{}
	}
	return id.Wrap(m.Context(), m.Raw().features)
}

// Deprecated returns whether this service is deprecated, by returning the
// relevant option value for setting deprecation.
func (m Method) Deprecated() Value {
	if m.IsZero() {
		return Value{}
	}
	builtins := m.Context().builtins()
	d := m.Options().Field(builtins.MethodDeprecated)
	if b, _ := d.AsBool(); b {
		return d
	}
	return Value{}
}

// Service returns the service this method is part of.
func (m Method) Service() Service {
	if m.IsZero() {
		return Service{}
	}
	return id.Wrap(m.Context(), m.Raw().service)
}

// Input returns the input type for this method, and whether it is a streaming
// input.
func (m Method) Input() (ty Type, stream bool) {
	if m.IsZero() {
		return Type{}, false
	}

	return GetRef(m.Context(), m.Raw().input), m.Raw().inputStream
}

// Output returns the output type for this method, and whether it is a streaming
// output.
func (m Method) Output() (ty Type, stream bool) {
	if m.IsZero() {
		return Type{}, false
	}

	return GetRef(m.Context(), m.Raw().output), m.Raw().outputStream
}
