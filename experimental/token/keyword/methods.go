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

package keyword

// OpenClose returns the open and close brackets for the four bracket
// keywords: [Parens], [Brackets], [Braces], and [Angles].
func (k Keyword) OpenClose() (string, string, bool) {
	switch k {
	case Parens:
		return "(", ")", true
	case Brackets:
		return "[", "]", true
	case Braces:
		return "{", "}", true
	case Angles:
		return "<", ">", true
	default:
		return "", "", false
	}
}

// IsModifier returns whether this keyword is any kind of modifier.
func (k Keyword) IsModifier() bool {
	return k.IsMethodTypeModifier() ||
		k.IsTypeModifier() ||
		k.IsImportModifier() ||
		k.IsMethodTypeModifier()
}

// IsFieldModifier returns whether this is a modifier for a field type.
func (k Keyword) IsFieldTypeModifier() bool {
	switch k {
	case Optional, Required, Repeated:
		return true
	default:
		return false
	}
}

// IsTypeModifier returns whether this is a modifier for a type declaration.
func (k Keyword) IsTypeModifier() bool {
	switch k {
	case Export, Local:
		return true
	default:
		return false
	}
}

// IsImportModifier returns whether this is a modifier for an import declaration.
func (k Keyword) IsImportModifier() bool {
	switch k {
	case Public, Weak, Option:
		return true
	default:
		return false
	}
}

// IsMethodTypeModifier returns whether this is a modifier for a method declaration.
func (k Keyword) IsMethodTypeModifier() bool {
	return k == Stream
}
