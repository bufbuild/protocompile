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

package unicodex

import "github.com/bufbuild/protocompile/internal/ext/bitsx"

// ToASCIILower converts an ASCII byte to lowercase.
func ToASCIILower(b byte) byte {
	diff := byte('a' - 'A')
	diff &= byte(bitsx.Mask(b >= 'A') & bitsx.Mask(b <= 'Z'))
	return b + diff
}

// ToASCIIUpper converts an ASCII byte to uppercase.
func ToASCIIUpper(b byte) byte {
	diff := byte('a' - 'A')
	diff &= byte(bitsx.Mask(b >= 'a') & bitsx.Mask(b <= 'z'))
	return b - diff
}
