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
	"github.com/bufbuild/protocompile/internal/ext/iterx"
	"github.com/bufbuild/protocompile/internal/iter"
)

// IsEdition returns whether this represents an edition.
func (s Syntax) IsEdition() bool {
	return s != Proto2 && s != Proto3
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

var totalEditions = iterx.Count(iterx.Filter(All(), Syntax.IsEdition))
