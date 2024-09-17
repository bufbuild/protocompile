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

import "fmt"

const (
	BuiltinUnknown Builtin = iota
	BuiltinInt32
	BuiltinInt64
	BuiltinUInt32
	BuiltinUInt64
	BuiltinSInt32
	BuiltinSInt64

	BuiltinFloat
	BuiltinDouble

	BuiltinFixed32
	BuiltinFixed64
	BuiltinSFixed32
	BuiltinSFixed64

	BuiltinBool
	BuiltinString
	BuiltinBytes

	// This corresponds to the builtin "type" map<K, V>.
	BuiltinMap

	// This corresponds to the builtin "constant" max, used in range expressions.
	BuiltinMax

	builtinCount

	BuiltinFloat32 = BuiltinFloat
	BuiltinFloat64 = BuiltinDouble
)

var (
	builtinByName = map[string]Builtin{
		"int32":  BuiltinInt32,
		"int64":  BuiltinInt64,
		"uint32": BuiltinUInt32,
		"uint64": BuiltinUInt64,
		"sint32": BuiltinSInt32,
		"sint64": BuiltinSInt64,

		"float":  BuiltinFloat,
		"double": BuiltinDouble,

		"fixed32":  BuiltinFixed32,
		"fixed64":  BuiltinFixed64,
		"sfixed32": BuiltinSFixed32,
		"sfixed64": BuiltinSFixed64,

		"bool":   BuiltinBool,
		"string": BuiltinString,
		"bytes":  BuiltinBytes,

		"map": BuiltinMap,
		"max": BuiltinMax,
	}

	builtinNames = func() []string {
		names := make([]string, builtinCount)
		names[0] = "unknown"

		for name, idx := range builtinByName {
			names[idx] = name
		}
		return names
	}()
)

// Builtin is one of the built-in Protobuf types.
type Builtin int8

// BuiltinByName looks up a builtin type by name.
//
// If name does not name a builtin, returns [BuiltinUnknown].
func BuiltinByName(name string) Builtin {
	// The zero value is BuiltinUnknown.
	return builtinByName[name]
}

// String implements [strings.Stringer] for Builtin.
func (b Builtin) String() string {
	if int(b) < len(builtinNames) {
		return builtinNames[int(b)]
	}
	return fmt.Sprintf("builtin%d", int(b))
}

// IsPrimitive returns if this builtin name refers to one of the primitive types.
func (b Builtin) IsPrimitive() bool {
	switch b {
	case BuiltinInt32, BuiltinInt64,
		BuiltinUInt32, BuiltinUInt64,
		BuiltinSInt32, BuiltinSInt64,
		BuiltinFloat, BuiltinDouble,
		BuiltinFixed32, BuiltinFixed64,
		BuiltinSFixed32, BuiltinSFixed64,
		BuiltinBool,
		BuiltinString, BuiltinBytes:
		return true
	default:
		return false
	}
}
