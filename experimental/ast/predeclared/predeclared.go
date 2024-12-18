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

//go:generate go run github.com/bufbuild/protocompile/internal/enum

//enum:import "strings"
//enum:stringfunc strings.ToLower
//enum:string
//enum:gostring

//enum:fromstring Lookup
//enum:doc fromstring "Lookup looks up a builtin type by name."
//enum:doc fromstring
//enum:doc fromstring "If name does not name a builtin, returns [Unknown]."

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

	// Aliases for the floating-point types with explicit bit-sizes.
	Float32 = Float  //enum:skip
	Float64 = Double //enum:skip
)
