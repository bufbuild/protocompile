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

package predeclared

// IsScalar returns whether a predeclared name corresponds to one of the
// primitive scalar types.
func (n Name) IsScalar() bool {
	return n >= Int32 && n <= Bytes
}

// IsInt returns whether this is an integer type.
func (n Name) IsInt() bool {
	return n >= Int32 && n <= SFixed64
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

// IsUnsigned returns whether this is a signed integer type.
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
