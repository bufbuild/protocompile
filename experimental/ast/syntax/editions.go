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

// IsValid returns whether this syntax is valid.
func (s Syntax) IsValid() bool {
	switch s {
	case Proto2, Proto3, Edition2023, Edition2024:
		return true
	default:
		return false
	}
}
