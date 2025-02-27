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

package intern

import (
	"strings"

	"github.com/bufbuild/protocompile/internal/ext/unsafex"
)

const (
	maxInlined = 32 / 6
)

var (
	// NOTE: We do not use exactly the same alphabet as LLVM: we swap _ and .,
	// so that . is encoded as 0b111111, aka 077.
	char6ToByte = []byte("0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ_.")
	byteToChar6 = func() []byte {
		out := make([]byte, 256)
		for i := range out {
			out[i] = 0xff
		}
		for j, b := range char6ToByte {
			out[int(b)] = byte(j)
		}
		return out
	}()
)

// encodeChar6 attempts to encoding data using the char6 encoding. Returns
// whether encoding was successful, and an encoded value.
func encodeChar6(data string) (ID, bool) {
	if data == "" {
		return 0, true
	}
	if len(data) > maxInlined || strings.HasSuffix(data, ".") {
		return 0, false
	}

	// The main encoding loop is outlined to promote inlining of the two
	// above checks into Table.Intern.
	return encodeOutlined(data)
}

func encodeOutlined(data string) (ID, bool) {
	// Start by filling value with all ones. Once we shift in all of the
	// encoded bytes from data, we will have two desired properties:
	//
	// 1. The sign bit will be set.
	//
	// 2. If there are less than five bytes, the trailing sextets will all
	//    be 077, aka '.'. Because we do not allow trailing periods, we can
	//    use this to determine the length of the original string.
	//
	//    Thus, "foo" is encoded as if it was the string "foo..".
	value := ID(-1)
	for i := len(data) - 1; i >= 0; i-- {
		sextet := byteToChar6[data[i]]
		if sextet == 0xff {
			return 0, false
		}
		value <<= 6
		value |= ID(sextet)
	}

	return value, true
}

// decodeChar6 decodes id assuming it contains a char6-encoded string.
func decodeChar6(id ID) string {
	// The main decoding loop is outlined to promote inlining of decodeChar6,
	// and thus heap-promotion of the returned string.
	data, len := decodeOutlined(id) //nolint:predeclared,revive // For `len`.
	return unsafex.StringAlias(data[:len])
}

//nolint:predeclared,revive // For `len`.
func decodeOutlined(id ID) (data [maxInlined]byte, len int) {
	for i := range data {
		data[i] = char6ToByte[int(id&077)]
		id >>= 6
	}

	// Figure out the length by removing a maximal suffix of
	// '.' bytes. Note that an all-ones value will decode to "", but encode
	// will never return that value.
	len = maxInlined
	for ; len > 0; len-- {
		if data[len-1] != '.' {
			break
		}
	}

	return data, len
}
