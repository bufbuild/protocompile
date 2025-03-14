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
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/experimental/token/keyword"
)

// TypePrefixed is a type with a [TypePrefix].
//
// Unlike in ordinary Protobuf, the Protocompile AST permits arbitrary nesting
// of modifiers.
//
// # Grammar
//
//	TypePrefixed := (`optional` | `repeated` | `required` | `stream`) Type
//
// Note that there are ambiguities when Type is an absolute [TypePath].
// The source "optional .foo" names the type "optional.foo" only when inside
// of a [TypeGeneric]'s brackets or a [Signature]'s method parameters.
//
// Also, the `stream` prefix may only occur inside of a [Signature].
type TypePrefixed struct{ typeImpl[rawTypePrefixed] }

type rawTypePrefixed struct {
	prefix token.ID
	ty     rawType
}

// TypePrefixedArgs is the arguments for [Context.NewTypePrefixed].
type TypePrefixedArgs struct {
	Prefix token.Token
	Type   TypeAny
}

// Prefix extracts the modifier out of this type.
//
// Returns [keyword.Unknown] if [TypePrefixed.PrefixToken] does not contain
// a known prefix.
func (t TypePrefixed) Prefix() keyword.Keyword {
	return t.PrefixToken().Keyword()
}

// PrefixToken returns the token representing this type's prefix.
func (t TypePrefixed) PrefixToken() token.Token {
	if t.IsZero() {
		return token.Zero
	}

	return t.raw.prefix.In(t.Context())
}

// Type returns the type that is being prefixed.
func (t TypePrefixed) Type() TypeAny {
	if t.IsZero() {
		return TypeAny{}
	}

	return newTypeAny(t.Context(), t.raw.ty)
}

// SetType sets the expression that is being prefixed.
//
// If passed zero, this clears the type.
func (t TypePrefixed) SetType(ty TypeAny) {
	t.raw.ty = ty.raw
}

// Span implements [report.Spanner].
func (t TypePrefixed) Span() report.Span {
	if t.IsZero() {
		return report.Span{}
	}

	return report.Join(t.PrefixToken(), t.Type())
}
