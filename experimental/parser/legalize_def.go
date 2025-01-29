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

func legalizeDef(p *parser, parent classified, def ast.DeclDef) {
	kind := def.Classify()

	if def.IsCorrupt() {
		return
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
