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

// Code generated by github.com/bufbuild/protocompile/internal/enum split.yaml. DO NOT EDIT.

package dom

import (
	"fmt"
	"iter"
)

// SplitKind identifies the kind of whitespace that should follow the text of a chunk.
type SplitKind int8

const (
	SplitKindUnknown SplitKind = iota // Unknown whitespace.
	SplitKindSoft                     // SplitKindSoft represents a soft split, which means that if a split happens, then the chunk may be converted to a hard split. If a split does not happen, then the text of the chunk will be followed by a space based on spaceWhenUnsplit at the time of output.
	SplitKindHard                     // SplitKindHard represents a hard split, which means the text for the chunk will be followed by a newline at the time of output.
	SplitKindDouble                   // SplitKindDouble represents a double hard split, which means the text for the chunk will be followed by two newlines at the time of output.
	SplitKindNever                    // SplitKindNever means that the chunks should never be split, and the text for the chunk will be followed by a space based on spaceWhenUnsplit at the time of output.
)

// String implements [fmt.Stringer].
func (v SplitKind) String() string {
	if int(v) < 0 || int(v) > len(_table_SplitKind_String) {
		return fmt.Sprintf("SplitKind(%v)", int(v))
	}
	return _table_SplitKind_String[int(v)]
}

// GoString implements [fmt.GoStringer].
func (v SplitKind) GoString() string {
	if int(v) < 0 || int(v) > len(_table_SplitKind_GoString) {
		return fmt.Sprintf("domSplitKind(%v)", int(v))
	}
	return _table_SplitKind_GoString[int(v)]
}

var _table_SplitKind_String = [...]string{
	SplitKindUnknown: "SplitKindUnknown",
	SplitKindSoft:    "SplitKindSoft",
	SplitKindHard:    "SplitKindHard",
	SplitKindDouble:  "SplitKindDouble",
	SplitKindNever:   "SplitKindNever",
}

var _table_SplitKind_GoString = [...]string{
	SplitKindUnknown: "SplitKindUnknown",
	SplitKindSoft:    "SplitKindSoft",
	SplitKindHard:    "SplitKindHard",
	SplitKindDouble:  "SplitKindDouble",
	SplitKindNever:   "SplitKindNever",
}
var _ iter.Seq[int] // Mark iter as used.
