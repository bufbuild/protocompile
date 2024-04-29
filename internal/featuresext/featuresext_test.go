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

package featuresext

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func TestFeaturesExt(t *testing.T) {
	t.Parallel()

	file, err := CppFeaturesDescriptor()
	require.NoError(t, err)
	assert.Equal(t, protoreflect.FullName("pb"), file.Package())
	assert.NotNil(t, file.Extensions().ByName("cpp"))

	file, err = JavaFeaturesDescriptor()
	require.NoError(t, err)
	assert.Equal(t, protoreflect.FullName("pb"), file.Package())
	assert.NotNil(t, file.Extensions().ByName("java"))
}
