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

package incremental_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bufbuild/protocompile/experimental/incremental"
)

func TestBuildURL(t *testing.T) {
	t.Parallel()

	type B = incremental.URLBuilder

	tests := []struct {
		Name, URL string
		Args      B
	}{
		{
			Name: "empty",
			URL:  "",
		},
		{
			Name: "opaque",
			URL:  "foo:bar/baz,bang",
			Args: B{
				Scheme: "foo",
				Opaque: "bar/baz,bang",
			},
		},
		{
			Name: "path",
			URL:  "foo://bar/baz%2Cbang",
			Args: B{
				Scheme: "foo",
				Path:   "bar/baz,bang",
			},
		},
		{
			Name: "queries",
			URL:  "foo://bar/baz%2Cbang?foo=2%2C2&baz=foo%2Fbar",
			Args: B{
				Scheme: "foo",
				Path:   "bar/baz,bang",
				Queries: [][2]string{
					{"foo", "2,2"},
					{"baz", "foo/bar"},
				},
			},
		},
		{
			Name: "queries_opaque",
			URL:  "foo:bar/baz,bang?foo=2%2C2&baz=foo%2Fbar",
			Args: B{
				Scheme: "foo",
				Opaque: "bar/baz,bang",
				Queries: [][2]string{
					{"foo", "2,2"},
					{"baz", "foo/bar"},
				},
			},
		},
		{
			Name: "fragment",
			URL:  "foo://bar/baz%2Cbang?a=b#bar+baz",
			Args: B{
				Scheme:   "foo",
				Path:     "bar/baz,bang",
				Queries:  [][2]string{{"a", "b"}},
				Fragment: "bar+baz",
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.Name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, test.URL, test.Args.Build())
		})
	}
}
