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

package source_test

import (
	"io/fs"
	"os"
	"testing"

	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/internal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFS(t *testing.T) {
	t.Parallel()

	opener := source.FS{FS: os.DirFS(internal.CallerDir(0))}

	text, err := opener.Open("hello.txt")
	require.NoError(t, err)
	assert.Equal(t, "hello!\n", text)

	_, err = opener.Open("missing.txt")
	require.ErrorIs(t, err, fs.ErrNotExist)
}

func TestMap(t *testing.T) {
	t.Parallel()

	opener := source.Map{"hello.txt": "hello!\n"}

	text, err := opener.Open("hello.txt")
	require.NoError(t, err)
	assert.Equal(t, "hello!\n", text)

	_, err = opener.Open("missing.txt")
	require.ErrorIs(t, err, fs.ErrNotExist)
}

func TestOpeners(t *testing.T) {
	t.Parallel()

	opener := source.Openers{
		&source.Map{"overlaid.txt": "overlaid!\n"},
		&source.FS{FS: os.DirFS(internal.CallerDir(0))},
	}

	text, err := opener.Open("overlaid.txt")
	require.NoError(t, err)
	assert.Equal(t, "overlaid!\n", text)

	text, err = opener.Open("hello.txt")
	require.NoError(t, err)
	assert.Equal(t, "hello!\n", text)

	_, err = opener.Open("missing.txt")
	require.ErrorIs(t, err, fs.ErrNotExist)
}
