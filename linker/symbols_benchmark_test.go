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

package linker

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func BenchmarkSymbols(b *testing.B) {
	s := &Symbols{}
	_, err := s.importPackages(nil, "foo.bar.baz.fizz.buzz.frob.nitz", nil)
	require.NoError(b, err)
	for i := 0; i < b.N; i++ {
		pkg := s.getPackage("foo.bar.baz.fizz.buzz.frob.nitz", true)
		require.NotNil(b, pkg)
	}
}
