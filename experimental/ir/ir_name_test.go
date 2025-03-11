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

package ir_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bufbuild/protocompile/experimental/ir"
)

func TestFullNameAbsolute(t *testing.T) {
	t.Parallel()
	tests := []struct {
		fqn, abs ir.FullName
	}{
		{fqn: "", abs: "."},
		{fqn: "foo", abs: ".foo"},
		{fqn: "foo.bar", abs: ".foo.bar"},
		{fqn: ".", abs: "."},
		{fqn: ".foo", abs: ".foo"},
		{fqn: ".foo.bar", abs: ".foo.bar"},
	}

	for _, tt := range tests {
		t.Run(string(tt.fqn), func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.fqn == tt.abs, tt.fqn.Absolute())
			assert.Equal(t, tt.abs, tt.fqn.ToAbsolute())
		})
	}
}

func TestFullNameParts(t *testing.T) {
	t.Parallel()
	tests := []struct {
		fqn, parent ir.FullName
		name        string
	}{
		{fqn: "", parent: "", name: ""},
		{fqn: "foo", parent: "", name: "foo"},
		{fqn: ".foo", parent: "", name: "foo"},
		{fqn: "foo.bar", parent: "foo", name: "bar"},
		{fqn: ".foo.bar", parent: ".foo", name: "bar"},
		{fqn: "foo.bar.baz", parent: "foo.bar", name: "baz"},
		{fqn: ".foo.bar.baz", parent: ".foo.bar", name: "baz"},
	}

	for _, tt := range tests {
		t.Run(string(tt.fqn), func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.name, tt.fqn.Name())
			assert.Equal(t, tt.parent, tt.fqn.Parent())
		})
	}
}

func TestFullNameAppend(t *testing.T) {
	t.Parallel()
	tests := []struct {
		fqn, got ir.FullName
		names    []string
	}{
		{
			fqn:   "",
			names: []string{},
			got:   "",
		},
		{
			fqn:   "",
			names: []string{"foo"},
			got:   "foo",
		},
		{
			fqn:   "",
			names: []string{"foo", "bar"},
			got:   "foo.bar",
		},

		{
			fqn:   "pkg",
			names: []string{},
			got:   "pkg",
		},
		{
			fqn:   "pkg",
			names: []string{"foo"},
			got:   "pkg.foo",
		},
		{
			fqn:   "pkg",
			names: []string{"foo", "bar"},
			got:   "pkg.foo.bar",
		},

		{
			fqn:   ".pkg",
			names: []string{},
			got:   ".pkg",
		},
		{
			fqn:   ".pkg",
			names: []string{"foo"},
			got:   ".pkg.foo",
		},
		{
			fqn:   ".pkg",
			names: []string{"foo", "bar"},
			got:   ".pkg.foo.bar",
		},
	}

	for _, tt := range tests {
		t.Run(string(tt.fqn), func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.got, tt.fqn.Append(tt.names...))
		})
	}
}
