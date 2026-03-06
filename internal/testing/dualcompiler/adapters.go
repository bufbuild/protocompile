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

package dualcompiler

import (
	"io"
	"io/fs"

	"github.com/bufbuild/protocompile"
	"github.com/bufbuild/protocompile/experimental/source"
)

// resolverOpener adapts a protocompile.Resolver to the source.Opener interface
// used by the experimental compiler.
type resolverOpener struct {
	resolver protocompile.Resolver
}

// ResolverToOpener converts a Resolver to an Opener.
// Note: This adapter only supports SearchResult.Source. Other result types
// (AST, Proto, ParseResult, Desc) will return an error.
func ResolverToOpener(resolver protocompile.Resolver) source.Opener {
	return &resolverOpener{resolver: resolver}
}

// Open implements source.Opener.
func (r *resolverOpener) Open(path string) (*source.File, error) {
	result, err := r.resolver.FindFileByPath(path)
	if err != nil {
		return nil, err
	}

	// Handle the Source result type (most common in tests)
	if result.Source != nil {
		data, err := io.ReadAll(result.Source)
		if err != nil {
			return nil, err
		}
		return source.NewFile(path, string(data)), nil
	}

	// For other result types, we need to convert them to source.
	// For now, we don't support these cases.
	//
	// For AST and Proto, return an error since these should be converted.
	// For Desc, return ErrNotExist to allow fallback to WKTs source files.
	// This is important because protocompile.WithStandardImports returns
	// Desc for WKTs, but the experimental compiler needs source files.
	if result.AST != nil {
		return nil, fs.ErrNotExist
	}
	if result.Proto != nil {
		return nil, fs.ErrNotExist
	}
	if result.Desc != nil {
		// Return not found so the Openers can try the next opener (WKTs)
		return nil, fs.ErrNotExist
	}

	// Note: We skip checking ParseResult as it can cause nil pointer issues
	// and we primarily support Source-based resolution for tests.

	// No result found
	return nil, fs.ErrNotExist
}
