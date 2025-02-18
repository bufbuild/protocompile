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

// Code generated by github.com/bufbuild/protocompile/internal/enum predeclared.yaml. DO NOT EDIT.

package predeclared

import (
	"fmt"
	"github.com/bufbuild/protocompile/internal/iter"
)

// Name is one of the built-in Protobuf names. These represent particular
// paths whose meaning the language overrides to mean something other than
// a relative path with that name.
type Name byte

const (
	Unknown Name = iota

	// Varint types: 32/64-bit signed, unsigned, and Zig-Zag.
	Int32
	Int64
	UInt32
	UInt64
	SInt32
	SInt64

	// Fixed integer types: 32/64-bit unsigned and signed.
	Fixed32
	Fixed64
	SFixed32
	SFixed64

	// Floating-point types: 32/64-bit, using C-style names.
	Float
	Double

	// Booleans.
	Bool

	// Textual strings (ostensibly UTF-8).
	String

	// Arbitrary byte blobs.
	Bytes

	// The special "type" map<K, V>, the only generic type in Protobuf.
	Map

	// The special "constant" max, used in range expressions.
	Max

	// True and false constants for bool.
	True
	False

	// Special floating-point constants for infinity and NaN.
	Inf
	NAN

	// Aliases for the floating-point types with explicit bit-sizes.
	Float32 = Float
	Float64 = Double
)

// String implements [fmt.Stringer].
func (v Name) String() string {
	if int(v) < 0 || int(v) > len(_table_Name_String) {
		return fmt.Sprintf("Name(%v)", int(v))
	}
	return _table_Name_String[int(v)]
}

// GoString implements [fmt.GoStringer].
func (v Name) GoString() string {
	if int(v) < 0 || int(v) > len(_table_Name_GoString) {
		return fmt.Sprintf("predeclaredName(%v)", int(v))
	}
	return _table_Name_GoString[int(v)]
}

// Lookup looks up a predefined identifier by name.
//
// If name does not name a predefined identifier, returns [Unknown].
func Lookup(s string) Name {
	return _table_Name_Lookup[s]
}

// All returns an iterator over all distinct [Name] values.
func All() iter.Seq[Name] {
	return func(yield func(Name) bool) {
		for i := 0; i < 22; i++ {
			if !yield(Name(i)) {
				return
			}
		}
	}
}

var _table_Name_String = [...]string{
	Unknown:  "unknown",
	Int32:    "int32",
	Int64:    "int64",
	UInt32:   "uint32",
	UInt64:   "uint64",
	SInt32:   "sint32",
	SInt64:   "sint64",
	Fixed32:  "fixed32",
	Fixed64:  "fixed64",
	SFixed32: "sfixed32",
	SFixed64: "sfixed64",
	Float:    "float",
	Double:   "double",
	Bool:     "bool",
	String:   "string",
	Bytes:    "bytes",
	Map:      "map",
	Max:      "max",
	True:     "true",
	False:    "false",
	Inf:      "inf",
	NAN:      "nan",
}

var _table_Name_GoString = [...]string{
	Unknown:  "Unknown",
	Int32:    "Int32",
	Int64:    "Int64",
	UInt32:   "UInt32",
	UInt64:   "UInt64",
	SInt32:   "SInt32",
	SInt64:   "SInt64",
	Fixed32:  "Fixed32",
	Fixed64:  "Fixed64",
	SFixed32: "SFixed32",
	SFixed64: "SFixed64",
	Float:    "Float",
	Double:   "Double",
	Bool:     "Bool",
	String:   "String",
	Bytes:    "Bytes",
	Map:      "Map",
	Max:      "Max",
	True:     "True",
	False:    "False",
	Inf:      "Inf",
	NAN:      "NAN",
}

var _table_Name_Lookup = map[string]Name{
	"unknown":  Unknown,
	"int32":    Int32,
	"int64":    Int64,
	"uint32":   UInt32,
	"uint64":   UInt64,
	"sint32":   SInt32,
	"sint64":   SInt64,
	"fixed32":  Fixed32,
	"fixed64":  Fixed64,
	"sfixed32": SFixed32,
	"sfixed64": SFixed64,
	"float":    Float,
	"double":   Double,
	"bool":     Bool,
	"string":   String,
	"bytes":    Bytes,
	"map":      Map,
	"max":      Max,
	"true":     True,
	"false":    False,
	"inf":      Inf,
	"nan":      NAN,
}
var _ iter.Seq[int] // Mark iter as used.
