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

// Package syntax specifies all of the syntax pragmas (including editions)
// that Protocompile understands.
package syntax

import (
	"github.com/bufbuild/protocompile/internal/ext/iterx"
	"github.com/bufbuild/protocompile/internal/iter"
)

//go:generate go run github.com/bufbuild/protocompile/internal/enum syntax.yaml

// Editions returns an iterator over all the editions in this package.
func Editions() iter.Seq[Syntax] {
	return func(yield func(Syntax) bool) {
		for i := 0; i < totalEditions; i++ {
			if !yield(Syntax(i + int(Edition2023))) {
				break
			}
		}
	}
}

var totalEditions = iterx.Count(All(), func(s Syntax) bool { return s.IsEdition() })
