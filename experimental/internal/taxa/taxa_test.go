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

package taxa_test

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bufbuild/protocompile/experimental/internal/taxa"
	"github.com/bufbuild/protocompile/experimental/token/keyword"
)

func TestMax(t *testing.T) {
	t.Parallel()

	var maxKw keyword.Keyword

	for kw := range keyword.All() {
		maxKw = max(maxKw, kw)
	}

	require.Less(t, int(taxa.Noun(maxKw)), int(taxa.Unrecognized))
}

func TestSet(t *testing.T) {
	t.Parallel()

	set := taxa.NewSet(taxa.Array, taxa.Decl, taxa.Comment, taxa.EOF)
	assert.True(t, set.Has(taxa.Array))
	assert.True(t, set.Has(taxa.Comment))
	assert.False(t, set.Has(taxa.Message))

	set = set.With(taxa.Message)
	assert.True(t, set.Has(taxa.Array))
	assert.True(t, set.Has(taxa.Comment))
	assert.True(t, set.Has(taxa.Message))

	assert.Equal(t,
		[]taxa.Noun{taxa.EOF, taxa.Decl, taxa.Message, taxa.Array, taxa.Comment},
		slices.Collect(set.All()),
	)

	set = taxa.NewSet(taxa.Noun(keyword.Message), taxa.Message)
	assert.Equal(t, taxa.NewSet(taxa.Noun(keyword.Message)), set.Keywords())
	assert.Equal(t, taxa.NewSet(taxa.Message), set.NonKeywords())
}

func TestJoin(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "", taxa.NewSet().Join("and"))
	assert.Equal(t, "`message`", taxa.NewSet(taxa.Noun(keyword.Message)).Join("and"))
	assert.Equal(t, "`message` and `enum`", taxa.NewSet(taxa.Noun(keyword.Message), taxa.Noun(keyword.Enum)).Join("and"))
	assert.Equal(t, "`message`, `enum`, and `service`",
		taxa.NewSet(taxa.Noun(keyword.Message), taxa.Noun(keyword.Enum), taxa.Noun(keyword.Service)).Join("and"))
	assert.Equal(t, "`syntax`, `message`, `enum`, and `service`",
		taxa.NewSet(taxa.Noun(keyword.Message), taxa.Noun(keyword.Enum), taxa.Noun(keyword.Service), taxa.Noun(keyword.Syntax)).Join("and"))
}
