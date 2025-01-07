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
// These are not keywords, but are rather special names injected into scope in
// places where any user-defined path is allowed. For example, the identifier
// string overrides the meaning of a path with a single identifier called string,
// (such as a reference to a message named string in the current package) and as
// such counts as a predeclared identifier.
package predeclared

import "fmt"

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

	Bool   // Booleans.
	String // Textual strings (ostensibly UTF-8).
	Bytes  // Arbitrary byte blobs.

	Map // The special "type" map<K, V>, the only generic type in Protobuf.
	Max // The special "constant" max, used in range expressions.

	// True and false constants for bool.
	True
	False

	// Special floating-point constants for infinity and NaN.
	Inf
	Nan

	count // Total number of valid Name values, used in the names constant below.

	// Aliases for the floating-point types with explicit bit-sizes.
	Float32 = Float
	Float64 = Double
)

// Name is one of the built-in Protobuf names. These represent particular
// paths whose meaning the language overrides to mean something other than
// a relative path with that name.
type Name byte

// Lookup looks up a builtin type by name.
//
// If name does not name a builtin, returns [Unknown].
func Lookup(name string) Name {
	// The zero value is Unknown, which map indexing will helpfully
	// return for us here.
	return byName[name]
}

// String implements [strings.Stringer].
func (n Name) String() string {
	if int(n) < len(names) {
		return names[int(n)]
	}
	return fmt.Sprintf("builtin%d", int(n))
}

// IsScalarType returns if this builtin name refers to one of the built-in
// scalar types (an integer or float, or one of bool, string, or bytes).
func (n Name) IsScalarType() bool {
	switch n {
	case
		Int32, Int64,
		UInt32, UInt64,
		SInt32, SInt64,

		Fixed32, Fixed64,
		SFixed32, SFixed64,

		Float, Double,

		Bool, String, Bytes:
		return true

	default:
		return false
	}
}

var (
	byName = map[string]Name{
		"int32":  Int32,
		"int64":  Int64,
		"uint32": UInt32,
		"uint64": UInt64,
		"sint32": SInt32,
		"sint64": SInt64,

		"fixed32":  Fixed32,
		"fixed64":  Fixed64,
		"sfixed32": SFixed32,
		"sfixed64": SFixed64,

		"float":  Float,
		"double": Double,

		"bool":   Bool,
		"string": String,
		"bytes":  Bytes,

		"map": Map,
		"max": Max,

		"true":  True,
		"false": False,
		"inf":   Inf,
		"nan":   Nan,
	}

	names = func() []string {
		names := make([]string, count)
		names[0] = "unknown"

		for name, idx := range byName {
			names[idx] = name
		}
		return names
	}()
)
