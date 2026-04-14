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

package bitsx

import (
	"fmt"
	"math/bits"
)

// ByteSize formats a number as a human-readable number of bytes.
func ByteSize[T Int](v T) string {
	abs := v
	if v < 0 {
		abs = -v
	}

	n := bits.Len64(uint64(abs))
	if n >= 30 {
		return fmt.Sprintf("%.03f GB", float64(v)/float64(1024*1024*1024))
	}
	if n >= 20 {
		return fmt.Sprintf("%.03f MB", float64(v)/float64(1024*1024))
	}
	if n >= 10 {
		return fmt.Sprintf("%.03f KB", float64(v)/float64(1024))
	}
	return fmt.Sprintf("%v.000 B", v)
}
