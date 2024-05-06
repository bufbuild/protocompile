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

package protocompile

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/types/descriptorpb"
)

func TestIsEditionSupported(t *testing.T) {
	t.Parallel()

	var min, max descriptorpb.Edition
	min = math.MaxInt32

	for editionNum := range descriptorpb.Edition_name {
		edition := descriptorpb.Edition(editionNum)
		if IsEditionSupported(edition) {
			if edition < min {
				min = edition
			}
			if edition > max {
				max = edition
			}
		}
	}

	assert.Equal(t, descriptorpb.Edition_EDITION_PROTO2, min)
	assert.Equal(t, descriptorpb.Edition_EDITION_2023, max)
}
