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

import (
	"fmt"
	"iter"
)

const (
	typePath typeKind = iota + 1
	typeModified
	typeGeneric
)

const (
	ModifierUnknown Modifier = iota + 1
	ModifierOptional
	ModifierRepeated
	ModifierRequired

	// Syntactically, we treat "group" as a modifier, because it
	// essentially parses the same way.
	//
	// Group [Field]s are the only fields that have no syntactically-specified
	// name.
	ModifierGroup
)

type typeKind int8

// Modifier is a modifier for a type, such as required, optional, or repeated.
type Modifier int8

// ModifierByName looks up a modifier kind by name.
//
// If name is not a known modifier, returns [ModifierUnknown].
func ModifierByName(name string) Modifier {
	switch name {
	case "optional":
		return ModifierOptional
	case "repeated":
		return ModifierRepeated
	case "required":
		return ModifierRequired
	case "group":
		return ModifierGroup
	default:
		return ModifierUnknown
	}
}

// String implements [strings.Stringer] for Modifier.
func (m Modifier) String() string {
	switch m {
	case ModifierUnknown:
		return "unknown"
	case ModifierOptional:
		return "optional"
	case ModifierRepeated:
		return "repeated"
	case ModifierRequired:
		return "required"
	case ModifierGroup:
		return "group"
	default:
		return fmt.Sprintf("modifier%d", int(m))
	}
}

// Type is the type of a field or service method.
//
// In the Protocompile AST, we regard many things as types for the sake of diagnostics.
// For example, "optional string" is a type, but so is the invalid type
// "optional repeated string".
//
// This is implemented by a limited set of types: [Path], [Modified], and [Generic].
type Type interface {
	Spanner

	kind() typeKind
}

func (Path) kind() typeKind     { return typePath }
func (Modified) kind() typeKind { return typeModified }
func (Generic) kind() typeKind  { return typeGeneric }

// Modified is a type with a [Modifier] prefix.
//
// Unlike in ordinary Protobuf, the Protocompile AST permits arbitrary nesting
// of modifiers.
type Modified struct {
	withContext

	raw *rawModified
}

// Modifier extracts the modifier out of this type.
//
// Returns [ModifierUnknown] if [Modified.ModifierToken] does not contain
// a known modifier.
func (m Modified) Modifier() Modifier {
	return ModifierByName(m.Keyword().Text())
}

// Keyword returns the token representing this type's modifier.
func (m Modified) Keyword() Token {
	return m.raw.modifier.With(m)
}

// Type returns the type that is being modified.
func (m Modified) Type() Type {
	return m.raw.inner.With(m)
}

// Span implements [Spanner] for Modified.
func (m Modified) Span() Span {
	return JoinSpans(m.Keyword(), m.Type())
}

// Generic is a type with generic arguments.
//
// Protobuf does not have generics... mostly. It has the map<K, V> production,
// which looks like something that generalizes, but doesn't. It is useful to parse
// when users mistakenly think this generalizes or provide the incorrect number
// of arguments.
//
// You will usually want to immediately call [Generic.Map] to codify the assumption
// that all generic types understood by your code are maps.
//
// Generic implements [Commas[Type]] for accessing its arguments.
type Generic struct {
	withContext

	raw *rawGeneric
}

// Path returns the path of the "type constructor". For example, for
// my.Map<K, V>, this would return the path my.Map.
func (g Generic) Path() Path {
	return g.raw.path.With(g)
}

// AngleBrackets returns the token tree corresponding to the <...> part of the type.
func (g Generic) AngleBrackets() Token {
	return g.raw.angles.With(g)
}

// Len implements [Slice] for Generic.
func (g Generic) Len() int {
	return len(g.raw.args)
}

// At implements [Slice] for Generic.
func (g Generic) At(n int) Type {
	return g.raw.args[n].ty.With(g)
}

// Comma implements [Commas] for Generic.
func (g Generic) Comma(n int) Token {
	return g.raw.args[n].comma.With(g)
}

// Iter implements [Slice] for Generic.
func (g Generic) Iter() iter.Seq2[int, Type] {
	return func(yield func(int, Type) bool) {
		for i, ty := range g.raw.args {
			if !yield(i, ty.ty.With(g)) {
				break
			}
		}
	}
}

// Span implements [Spanner] for Generic.
func (g Generic) Span() Span {
	return JoinSpans(g.Path(), g.AngleBrackets())
}

// rawType is the raw representation of a type.
//
// The vast, vast majority of types are paths. To avoid needing to waste
// space for such types, we use the following encoding for rawType.
//
// First, note that the two halves of a rawPath (see path.go) cannot be
// one non-synthetic and one synthetic. Thus, if the first "token" of the
// rawPath is synthetic and the second is not, the first is ^typeKind and
// the second is an index into a table in a Context. Otherwise, it's a path
// type. This logic is implemented in With().
type rawType rawPath

func (t rawType) With(c Contextual) Type {
	if t[0] < 0 && t[1] > 0 {
		c := c.Context()
		switch typeKind(^t[0]) {
		case typeModified:
			return Modified{withContext{c}, c.modifieds.At(int(t[1]))}
		case typeGeneric:
			return Generic{withContext{c}, c.generics.At(int(t[1]))}
		default:
			panic(fmt.Sprintf("protocompile/ast: invalid typeKind: %d", ^t[0]))
		}
	}
	return rawPath(t).With(c)
}

type rawModified struct {
	modifier rawToken
	inner    rawType
}

type rawGeneric struct {
	path   rawPath
	angles rawToken
	args   []struct {
		ty    rawType
		comma rawToken
	}
}
