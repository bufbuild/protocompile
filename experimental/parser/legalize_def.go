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

package parser

import (
	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/internal/taxa"
)

// Map of a def kind to the valid parents it can have.
//
// We use taxa.Set here because it already exists and is pretty cheap.
var validDefParents = [...]taxa.Set{
	ast.DefKindMessage:   taxa.NewSet(taxa.TopLevel, taxa.Message, taxa.Group),
	ast.DefKindEnum:      taxa.NewSet(taxa.TopLevel, taxa.Message, taxa.Group),
	ast.DefKindService:   taxa.NewSet(taxa.TopLevel),
	ast.DefKindExtend:    taxa.NewSet(taxa.TopLevel, taxa.Message, taxa.Group),
	ast.DefKindField:     taxa.NewSet(taxa.Message, taxa.Group, taxa.Extend, taxa.Oneof),
	ast.DefKindOneof:     taxa.NewSet(taxa.Message, taxa.Group),
	ast.DefKindGroup:     taxa.NewSet(taxa.Message, taxa.Group, taxa.Extend),
	ast.DefKindEnumValue: taxa.NewSet(taxa.Enum),
	ast.DefKindMethod:    taxa.NewSet(taxa.Service),
	ast.DefKindOption: taxa.NewSet(
		taxa.TopLevel, taxa.Message, taxa.Enum, taxa.Service,
		taxa.Oneof, taxa.Group, taxa.Method,
	),
}

func legalizeDef(p *parser, parent classified, def ast.DeclDef) {
	if def.IsCorrupt() {
		return
	}

	kind := def.Classify()
	if !validDefParents[kind].Has(parent.what) {
		p.Error(errBadNest{parent: parent, child: def})
	}

	switch kind {
	case ast.DefKindMessage, ast.DefKindEnum, ast.DefKindService, ast.DefKindOneof, ast.DefKindExtend:
		legalizeTypeDefLike(p, taxa.Classify(def), def)
	case ast.DefKindField, ast.DefKindEnumValue, ast.DefKindGroup:
		legalizeFieldLike(p, taxa.Classify(def), def)
	case ast.DefKindOption:
		legalizeOption(p, def)
	case ast.DefKindMethod:
		legalizeMethod(p, def)
	}
}

// legalizeMessageLike legalizes something that resembles a type definition:
// namely, messages, enums, oneofs, services, and extension blocks.
func legalizeTypeDefLike(p *parser, what taxa.Noun, def ast.DeclDef) {
}

// legalizeMessageLike legalizes something that resembles a field definition:
// namely, fields, groups, and enum values.
func legalizeFieldLike(p *parser, what taxa.Noun, def ast.DeclDef) {
	if options := def.Options(); !options.IsZero() {
		legalizeCompactOptions(p, options)
	}

	if what == taxa.Field {
		legalizeFieldType(p, def.Type())
	}
}

func legalizeOption(p *parser, def ast.DeclDef) {
	legalizeOptionEntry(p, def.AsOption().Option, def.Span())
}

func legalizeMethod(p *parser, def ast.DeclDef) {
}
