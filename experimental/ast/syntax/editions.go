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
	"iter"
	"strconv"
)

// LatestImplementedEdition is the most recent edition that the compiler
// implements.
const LatestImplementedEdition = Edition2023

// All returns an iterator over all known [Syntax] values.
func All() iter.Seq[Syntax] {
	return func(yield func(Syntax) bool) {
		_ = yield(Proto2) &&
			yield(Proto3) &&
			yield(Edition2023) &&
			yield(Edition2024)
	}
}

// Editions returns an iterator over all the editions in this package.
func Editions() iter.Seq[Syntax] {
	return func(yield func(Syntax) bool) {
		_ = yield(Edition2023) &&
			yield(Edition2024)
	}
}

// IsEdition returns whether this represents an edition.
func (s Syntax) IsEdition() bool {
	return s != Proto2 && s != Proto3
}

// IsSupported returns whether this syntax is fully supported.
func (s Syntax) IsSupported() bool {
	switch s {
	case Proto2, Proto3, Edition2023:
		return true
	default:
		return false
	}
}

// IsValid returns whether this syntax is valid (i.e., it can appear in a
// syntax/edition declaration).
func (s Syntax) IsValid() bool {
	switch s {
	case Proto2, Proto3, Edition2023, Edition2024:
		return true
	default:
		return false
	}
}

// IsKnown returns whether this syntax is a known value in google.protobuf.Edition.
func (s Syntax) IsKnown() bool {
	switch s {
	case Unknown, EditionLegacy,
		Proto2, Proto3, Edition2023, Edition2024,
		EditionTest1, EditionTest2, EditionTest99997, EditionTest99998, EditionTest99999,
		EditionMax:
		return true
	default:
		return false
	}
}

// IsConstraint returns whether this syntax can be used as a constraint in
// google.protobuf.FieldOptions.feature_support.
func (s Syntax) IsConstraint() bool {
	switch s {
	case Proto2, Proto3, Edition2023, Edition2024,
		EditionLegacy:
		return true
	default:
		return false
	}
}

// DescriptorName converts a syntax into the corresponding google.protobuf.Edition name.
//
// Returns a stringified digit if it is not a named edition value.
func (s Syntax) DescriptorName() string {
	name := descriptorNames[s]
	if name != "" {
		return name
	}
	return strconv.Itoa(int(s))
}

var descriptorNames = map[Syntax]string{
	Unknown:       "EDITION_UNKNOWN",
	EditionLegacy: "EDITION_LEGACY",

	Proto2:      "EDITION_PROTO2",
	Proto3:      "EDITION_PROTO3",
	Edition2023: "EDITION_2023",
	Edition2024: "EDITION_2024",

	EditionTest1:     "EDITION_1_TEST_ONLY",
	EditionTest2:     "EDITION_2_TEST_ONLY",
	EditionTest99997: "EDITION_99997_TEST_ONLY",
	EditionTest99998: "EDITION_99998_TEST_ONLY",
	EditionTest99999: "EDITION_99999_TEST_ONLY",

	EditionMax: "EDITION_MAX",
}
