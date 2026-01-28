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
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/bufbuild/protocompile/experimental/ast/predeclared"
)

func TestPredicates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		v           predeclared.Name
		scalar, key bool
		fdpType     descriptorpb.FieldDescriptorProto_Type
	}{
		{v: predeclared.Unknown},

		{v: predeclared.Int32, scalar: true, key: true, fdpType: descriptorpb.FieldDescriptorProto_TYPE_INT32},
		{v: predeclared.Int64, scalar: true, key: true, fdpType: descriptorpb.FieldDescriptorProto_TYPE_INT64},
		{v: predeclared.UInt32, scalar: true, key: true, fdpType: descriptorpb.FieldDescriptorProto_TYPE_UINT32},
		{v: predeclared.UInt64, scalar: true, key: true, fdpType: descriptorpb.FieldDescriptorProto_TYPE_UINT64},
		{v: predeclared.SInt32, scalar: true, key: true, fdpType: descriptorpb.FieldDescriptorProto_TYPE_SINT32},
		{v: predeclared.SInt64, scalar: true, key: true, fdpType: descriptorpb.FieldDescriptorProto_TYPE_SINT64},

		{v: predeclared.Fixed32, scalar: true, key: true, fdpType: descriptorpb.FieldDescriptorProto_TYPE_FIXED32},
		{v: predeclared.Fixed64, scalar: true, key: true, fdpType: descriptorpb.FieldDescriptorProto_TYPE_FIXED64},
		{v: predeclared.SFixed32, scalar: true, key: true, fdpType: descriptorpb.FieldDescriptorProto_TYPE_SFIXED32},
		{v: predeclared.SFixed64, scalar: true, key: true, fdpType: descriptorpb.FieldDescriptorProto_TYPE_SFIXED64},

		{v: predeclared.Float, scalar: true, fdpType: descriptorpb.FieldDescriptorProto_TYPE_FLOAT},
		{v: predeclared.Double, scalar: true, fdpType: descriptorpb.FieldDescriptorProto_TYPE_DOUBLE},

		{v: predeclared.String, scalar: true, key: true, fdpType: descriptorpb.FieldDescriptorProto_TYPE_STRING},
		{v: predeclared.Bytes, scalar: true, fdpType: descriptorpb.FieldDescriptorProto_TYPE_BYTES},
		{v: predeclared.Bool, scalar: true, key: true, fdpType: descriptorpb.FieldDescriptorProto_TYPE_BOOL},

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
		assert.Equal(t, test.fdpType, test.v.FDPType())
	}
}
