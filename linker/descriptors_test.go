// Copyright 2020-2023 Buf Technologies, Inc.
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

func TestNoOpDescriptors(t *testing.T) {
	t.Parallel()
	require.NotNil(t, noOpFile)
	require.NotNil(t, noOpMessage)
	require.NotNil(t, noOpOneof)
	require.NotNil(t, noOpField)
	require.NotNil(t, noOpEnum)
	require.NotNil(t, noOpEnumValue)
	require.NotNil(t, noOpExtension)
	require.NotNil(t, noOpService)
	require.NotNil(t, noOpMethod)
}
