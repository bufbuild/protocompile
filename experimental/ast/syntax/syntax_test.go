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

package syntax_test

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bufbuild/protocompile/experimental/ast/syntax"
	"github.com/bufbuild/protocompile/internal/ext/iterx"
)

func TestEditions(t *testing.T) {
	t.Parallel()

	assert.Equal(t, []syntax.Syntax{syntax.Edition2023, syntax.Edition2024}, slices.Collect(syntax.Editions()))
	assert.Equal(t,
		[]syntax.Syntax{syntax.Edition2023, syntax.Edition2024},
		slices.Collect(syntax.Editions()),
	)
	// Verify all editions report as supported
	supported := slices.Collect(iterx.Filter(syntax.Editions(), syntax.Syntax.IsSupported))
	assert.Equal(t, []syntax.Syntax{syntax.Edition2023, syntax.Edition2024}, supported)
}
