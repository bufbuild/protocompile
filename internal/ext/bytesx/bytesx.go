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

package bytesx

import "github.com/bufbuild/protocompile/internal/ext/unicodex"

// MakeASCIILower uppercases every ASCII letter in buf in-place. Works for any
// UTF-8 string.
func MakeASCIILower(buf []byte) {
	for i := range buf {
		buf[i] = unicodex.ToASCIILower(buf[i])
	}
}

// MakeASCIIUpper uppercases every ASCII letter in buf in-place. Works for any
// UTF-8 string.
func MakeASCIIUpper(buf []byte) {
	for i := range buf {
		buf[i] = unicodex.ToASCIIUpper(buf[i])
	}
}
