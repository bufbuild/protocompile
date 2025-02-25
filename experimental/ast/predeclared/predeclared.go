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

// package predeclared provides all of the identifiers with a special meaning
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

	"github.com/bufbuild/protocompile/experimental/token/keyword"
	"github.com/bufbuild/protocompile/internal/iter"
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
func (v Name) String() string {
	if !v.InRange() {
		return fmt.Sprintf("Name(%d)", int(v))
	}
	return keyword.Keyword(v).String()
}

// GoString implements [fmt.GoStringer].
func (v Name) GoString() string {
	if !v.InRange() {
		return fmt.Sprintf("predeclared.Name(%d)", int(v))
	}
	return keyword.Keyword(v).GoString()
}

// InRange returns whether this name value is within the range of declared
// values.
func (v Name) InRange() bool {
	return v == Unknown || (v >= Int32 && v <= Max)
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
