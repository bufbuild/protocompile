// Copyright 2020-2024 Buf Technologies, Inc.
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
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bufbuild/protocompile/experimental/internal/taxa"
	"github.com/bufbuild/protocompile/internal/iters"
)

func TestAllStringify(t *testing.T) {
	t.Parallel()

	taxa.All()(func(s taxa.Subject) bool {
		assert.NotEqual(t, "", s.String())
		assert.NotEqual(t, "", s.GoString())

		return true
	})
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
		[]taxa.Subject{taxa.EOF, taxa.Decl, taxa.Message, taxa.Array, taxa.Comment},
		iters.Collect(set.All()),
	)
}

func TestJoin(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "", taxa.NewSet().Join("and"))
	assert.Equal(t, "`message`", taxa.NewSet(taxa.KeywordMessage).Join("and"))
	assert.Equal(t, "`message` and `enum`", taxa.NewSet(taxa.KeywordMessage, taxa.KeywordEnum).Join("and"))
	assert.Equal(t, "`message`, `enum`, and `service`",
		taxa.NewSet(taxa.KeywordMessage, taxa.KeywordEnum, taxa.KeywordService).Join("and"))
	assert.Equal(t, "`syntax`, `message`, `enum`, and `service`",
		taxa.NewSet(taxa.KeywordMessage, taxa.KeywordEnum, taxa.KeywordService, taxa.KeywordSyntax).Join("and"))
}
