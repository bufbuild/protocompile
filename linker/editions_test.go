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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/bufbuild/protocompile/internal"
)

func TestFieldDescriptors(t *testing.T) {
	t.Parallel()
	assert.NotNil(t, fieldPresenceField, "field_presence")
	assert.NotNil(t, repeatedFieldEncodingField, "repeated_field_encoding")
	assert.NotNil(t, messageEncodingField, "message_encoding")
	assert.NotNil(t, enumTypeField, "enum_type")
	assert.NotNil(t, jsonFormatField, "json_format")
}

func TestGetEditionDefaults(t *testing.T) {
	t.Parallel()
	// Make sure all supported editions have defaults.
	for _, edition := range internal.SupportedEditions {
		features := getEditionDefaults(edition)
		require.NotNil(t, features)
	}
	// Spot check some things
	features := getEditionDefaults(descriptorpb.Edition_EDITION_PROTO2)
	require.NotNil(t, features)
	assert.Equal(t, descriptorpb.FeatureSet_CLOSED, features.GetEnumType())
	assert.Equal(t, descriptorpb.FeatureSet_EXPLICIT, features.GetFieldPresence())
	assert.Equal(t, descriptorpb.FeatureSet_EXPANDED, features.GetRepeatedFieldEncoding())

	features = getEditionDefaults(descriptorpb.Edition_EDITION_PROTO3)
	require.NotNil(t, features)
	assert.Equal(t, descriptorpb.FeatureSet_OPEN, features.GetEnumType())
	assert.Equal(t, descriptorpb.FeatureSet_IMPLICIT, features.GetFieldPresence())
	assert.Equal(t, descriptorpb.FeatureSet_PACKED, features.GetRepeatedFieldEncoding())

	features = getEditionDefaults(descriptorpb.Edition_EDITION_2023)
	require.NotNil(t, features)
	assert.Equal(t, descriptorpb.FeatureSet_OPEN, features.GetEnumType())
	assert.Equal(t, descriptorpb.FeatureSet_EXPLICIT, features.GetFieldPresence())
	assert.Equal(t, descriptorpb.FeatureSet_PACKED, features.GetRepeatedFieldEncoding())
}
