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

// Package predeclared provides all of the identifiers with a special meaning
// in Protobuf.
//
// These are a subset of the [keyword.Keyword] enum which are names that are
// special for name resolution. For example, the identifier string overrides the
// meaning of a path with a single identifier called string, (such as a
// reference to a message named string in the current package) and as such
// counts as a predeclared identifier.
package predeclared

import (
	"fmt"
	"iter"

	"github.com/bufbuild/protocompile/experimental/token/keyword"
)

// Name is one of the built-in Protobuf names. These represent particular
// paths whose meaning the language overrides to mean something other than
// a relative path with that name.
type Name keyword.Keyword

const (
	Unknown = Name(keyword.Unknown)

	Int32    = Name(keyword.Int32)
	Int64    = Name(keyword.Int64)
	UInt32   = Name(keyword.UInt32)
	UInt64   = Name(keyword.UInt64)
	SInt32   = Name(keyword.SInt32)
	SInt64   = Name(keyword.SInt64)
	Fixed32  = Name(keyword.Fixed32)
	Fixed64  = Name(keyword.Fixed64)
	SFixed32 = Name(keyword.SFixed32)
	SFixed64 = Name(keyword.SFixed64)
	Float    = Name(keyword.Float)
	Double   = Name(keyword.Double)
	Bool     = Name(keyword.Bool)
	String   = Name(keyword.String)
	Bytes    = Name(keyword.Bytes)
	Inf      = Name(keyword.Inf)
	NAN      = Name(keyword.NAN)
	True     = Name(keyword.True)
	False    = Name(keyword.False)
	Map      = Name(keyword.Map)
	Max      = Name(keyword.Max)

	Float32 = Float
	Float64 = Double
)

// FromKeyword performs a vast from a [keyword.Keyword], but also validates
// that it is in-range. If it isn't, returns [Unknown].
func FromKeyword(kw keyword.Keyword) Name {
	n := Name(kw)
	if n.InRange() {
		return n
	}
	return Unknown
}

// String implements [fmt.Stringer].
func (n Name) String() string {
	if !n.InRange() {
		return fmt.Sprintf("Name(%d)", int(n))
	}
	return keyword.Keyword(n).String()
}

// GoString implements [fmt.GoStringer].
func (n Name) GoString() string {
	if !n.InRange() {
		return fmt.Sprintf("predeclared.Name(%d)", int(n))
	}
	return keyword.Keyword(n).GoString()
}

// InRange returns whether this name value is within the range of declared
// values.
func (n Name) InRange() bool {
	return n == Unknown || (n >= Int32 && n <= Max)
}

// IsScalar returns whether a predeclared name corresponds to one of the
// primitive scalar types.
func (n Name) IsScalar() bool {
	return n >= Int32 && n <= Bytes
}

// IsInt returns whether this is an integer type.
func (n Name) IsInt() bool {
	return n >= Int32 && n <= SFixed64
}

// IsNumber returns whether this is a numeric type.
func (n Name) IsNumber() bool {
	return n >= Int32 && n <= Double
}

// IsPackable returns whether this is a type that can go in a packed repeated
// field.
func (n Name) IsPackable() bool {
	return n >= Int32 && n <= Bool
}

// IsVarint returns whether this is a varint-encoded type.
func (n Name) IsVarint() bool {
	return n >= Int32 && n <= SInt64 || n == Bool
}

// IsZigZag returns whether this is a ZigZag varint-encoded type.
func (n Name) IsZigZag() bool {
	return n == SInt32 || n == SInt64
}

// IsFixed returns whether this is a fixed-width type.
func (n Name) IsFixed() bool {
	return n >= Fixed32 && n <= Double
}

// IsUnsigned returns whether this is an unsigned integer type.
func (n Name) IsUnsigned() bool {
	switch n {
	case UInt32, UInt64, Fixed32, Fixed64:
		return true
	default:
		return false
	}
}

// IsSigned returns whether this is a signed integer type.
func (n Name) IsSigned() bool {
	return n.IsInt() && !n.IsUnsigned()
}

// IsFloat returns whether this is a floating-point type.
func (n Name) IsFloat() bool {
	return n == Float32 || n == Float64
}

// IsString returns whether this is a string type (string or bytes).
func (n Name) IsString() bool {
	return n == String || n == Bytes
}

// Bits returns the bit size of a name satisfying [Name.IsNumber].
//
// Return 0 for all other names.
func (n Name) Bits() int {
	switch n {
	case Int32, UInt32, SInt32, Fixed32, SFixed32, Float32:
		return 32
	case Int64, UInt64, SInt64, Fixed64, SFixed64, Float64:
		return 64
	default:
		return 0
	}
}

// IsMapKey returns whether this predeclared name represents one of the map key
// types.
func (n Name) IsMapKey() bool {
	return (n >= Int32 && n <= SFixed64) || n == Bool || n == String
}

// Lookup looks up a predefined identifier by name.
//
// If name does not name a predefined identifier, returns [Unknown].
func Lookup(s string) Name {
	return FromKeyword(keyword.Lookup(s))
}

// All returns an iterator over all distinct [Name] values.
func All() iter.Seq[Name] {
	return func(yield func(Name) bool) {
		if !yield(Unknown) {
			return
		}
		for i := Int32; i <= Max; i++ {
			if !yield(i) {
				return
			}
		}
	}
}
