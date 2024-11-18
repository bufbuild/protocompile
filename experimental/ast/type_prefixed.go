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
	"fmt"
)

// TypePrefixed is a type with a [TypePrefix].
//
// Unlike in ordinary Protobuf, the Protocompile AST permits arbitrary nesting
// of modifiers.
type TypePrefixed struct{ typeImpl[rawTypePrefixed] }

type rawTypePrefixed struct {
	prefix rawToken
	ty     rawType
}

// TypePrefixedArgs is the arguments for [Context.NewTypePrefixed].
type TypePrefixedArgs struct {
	Prefix Token
	Type   TypeAny
}

// Prefix extracts the modifier out of this type.
//
// Returns [TypePrefixUnknown] if [TypePrefixed.PrefixToken] does not contain
// a known modifier.
func (t TypePrefixed) Prefix() TypePrefix {
	return TypePrefixByName(t.PrefixToken().Text())
}

// PrefixToken returns the token representing this type's prefix.
func (t TypePrefixed) PrefixToken() Token {
	return t.raw.prefix.With(t)
}

// Type returns the type that is being prefixed.
func (t TypePrefixed) Type() TypeAny {
	return TypeAny{t.withContext, t.raw.ty}
}

// SetType sets the expression that is being prefixed.
//
// If passed nil, this clears the type.
func (t TypePrefixed) SetType(ty TypeAny) {
	t.raw.ty = ty.raw
}

// Span implements [Spanner].
func (t TypePrefixed) Span() Span {
	return JoinSpans(t.PrefixToken(), t.Type())
}

const (
	TypePrefixUnknown TypePrefix = iota
	TypePrefixOptional
	TypePrefixRepeated
	TypePrefixRequired

	// This is the "stream Foo.bar" syntax of RPC methods. It is also treated as
	// a prefix.
	TypePrefixStream
)

// TypePrefix is a prefix for a type, such as required, optional, or repeated.
type TypePrefix int8

// TypePrefixByName looks up a prefix kind by name.
//
// If name is not a known prefix, returns [TypePrefixUnknown].
func TypePrefixByName(name string) TypePrefix {
	switch name {
	case "optional":
		return TypePrefixOptional
	case "repeated":
		return TypePrefixRepeated
	case "required":
		return TypePrefixRequired
	case "stream":
		return TypePrefixStream
	default:
		return TypePrefixUnknown
	}
}

// String implements [strings.Stringer].
func (m TypePrefix) String() string {
	switch m {
	case TypePrefixUnknown:
		return "unknown"
	case TypePrefixOptional:
		return "optional"
	case TypePrefixRepeated:
		return "repeated"
	case TypePrefixRequired:
		return "required"
	case TypePrefixStream:
		return "stream"
	default:
		return fmt.Sprintf("modifier%d", int(m))
	}
}