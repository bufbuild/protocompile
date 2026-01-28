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

package fdp

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/experimental/token"
)

func TestLineComments(t *testing.T) {
	t.Parallel()

	s := &token.Stream{
		File: source.NewFile("test",
			`// Line 1
// Line 2`),
	}
	line1 := s.Push(9, token.Comment)
	newline := s.Push(1, token.Space)
	line2 := s.Push(9, token.Comment)

	tokens := paragraph([]token.Token{line1, newline, line2})
	assert.Equal(t, " Line 1\n Line 2", tokens.stringify())
}

func TestBlockComments(t *testing.T) {
	t.Parallel()

	s := &token.Stream{
		File: source.NewFile("test",
			`/*
* Line 1
* Line 2
*/`),
	}
	block := s.Push(21, token.Comment)

	tokens := paragraph([]token.Token{block})
	assert.Equal(t, "\n Line 1\n Line 2\n", tokens.stringify())
}
