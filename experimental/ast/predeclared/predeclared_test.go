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

package predeclared_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bufbuild/protocompile/experimental/ast/predeclared"
)

func TestPredicates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		v           predeclared.Name
		scalar, key bool
	}{
		{v: predeclared.Unknown},

		{v: predeclared.Int32, scalar: true, key: true},
		{v: predeclared.Int64, scalar: true, key: true},
		{v: predeclared.UInt32, scalar: true, key: true},
		{v: predeclared.UInt64, scalar: true, key: true},
		{v: predeclared.SInt32, scalar: true, key: true},
		{v: predeclared.SInt64, scalar: true, key: true},

		{v: predeclared.Fixed32, scalar: true, key: true},
		{v: predeclared.Fixed64, scalar: true, key: true},
		{v: predeclared.SFixed32, scalar: true, key: true},
		{v: predeclared.SFixed64, scalar: true, key: true},

		{v: predeclared.Float, scalar: true},
		{v: predeclared.Double, scalar: true},

		{v: predeclared.String, scalar: true, key: true},
		{v: predeclared.Bytes, scalar: true},
		{v: predeclared.Bool, scalar: true, key: true},

		{v: predeclared.Map},
		{v: predeclared.Max},
		{v: predeclared.True},
		{v: predeclared.False},
		{v: predeclared.Inf},
		{v: predeclared.NAN},
	}

	for _, test := range tests {
		assert.Equal(t, test.scalar, test.v.IsScalar())
		assert.Equal(t, test.key, test.v.IsMapKey())
	}
}
