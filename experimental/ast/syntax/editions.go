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

package syntax

import (
	"fmt"
	"iter"

	"github.com/bufbuild/protocompile/internal/ext/iterx"
	"github.com/bufbuild/protocompile/internal/ext/slicesx"
)

const (
	EditionLegacyNumber = 900
	EditionProto2Number = 998
	EditionProto3Number = 999
	Edition2023Number   = 1000
	Edition2024Number   = 1001
	EditionMaxNumber    = 0x7fff_ffff
)

// IsEdition returns whether this represents an edition.
func (s Syntax) IsEdition() bool {
	return s != Proto2 && s != Proto3
}

// Greater returns whether this syntax/edition is later than a particular enum
// number from descriptor.proto.
func (s Syntax) LaterThanNumber(enumValue int32) bool {
	switch enumValue {
	case EditionLegacyNumber:
		return true
	case EditionProto2Number:
		return Proto2 <= s
	case EditionProto3Number:
		return Proto3 <= s
	case Edition2023Number:
		return Edition2023 <= s
	case Edition2024Number:
		return Edition2024 <= s
	default:
		return false
	}
}

// FromEnum converts a google.protobuf.Edition value into a [Syntax].
//
// Returns [Unknown] for numbers that do not correspond to a real syntax or
// edition.
func FromEnum(enumValue int32) Syntax {
	switch enumValue {
	case EditionProto2Number:
		return Proto2
	case EditionProto3Number:
		return Proto3
	case Edition2023Number:
		return Edition2023
	case Edition2024Number:
		return Edition2024
	default:
		return Unknown
	}
}

// Editions returns an iterator over all the editions in this package.
func Editions() iter.Seq[Syntax] {
	return func(yield func(Syntax) bool) {
		for i := range totalEditions {
			if !yield(Syntax(i + int(Edition2023))) {
				break
			}
		}
	}
}

// PrettyString returns a nice string for this syntax: this is either something
// like `proto2`, or Edition 2023.
func (s Syntax) PrettyString() string {
	v, ok := slicesx.Get(prettyStrings, s)
	if !ok {
		return "<unknown>"
	}
	return v
}

var totalEditions = iterx.Count(iterx.Filter(All(), Syntax.IsEdition))

var prettyStrings = func() []string {
	strings := []string{"<unknown>"}
	for value := range All() {
		if !value.IsEdition() {
			strings = append(strings, fmt.Sprintf("`%s`", value))
		} else {
			strings = append(strings, fmt.Sprintf("Edition %s", value))
		}
	}
	return strings
}()
