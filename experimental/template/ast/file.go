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

package ast

import (
	"github.com/bufbuild/protocompile/experimental/id"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/internal/arena"
	"github.com/bufbuild/protocompile/internal/ext/unsafex"
)

// File is the top-level AST node for a Protobuf file.
//
// A file is a list of declarations (in other words, it is a [DeclBody]). The
// File type provides convenience functions for extracting salient elements,
// such as the [DeclSyntax] and the [DeclPackage].
//
// # Grammar
//
//	File := DeclAny*
type File struct {
	_      unsafex.NoCopy
	stream *token.Stream
	path   string

	tags      tags
	fragments arena.Arena[rawFragment]
}

type withContext = id.HasContext[*File]

// New creates a fresh context for a file.
//
// path is the semantic import path of this file, which may not be the same as
// file.Path, which is used for diagnostics.
func New(path string, file *source.File) *File {
	f := &File{
		stream: &token.Stream{File: file},
		path:   path,
	}
	_ = f.Nodes().NewFragment(FragmentArgs{}) // This is the fragment for the whole file.

	return f
}

// Path returns the semantic import path of this file.
func (f *File) Path() string {
	if f == nil {
		return ""
	}
	return f.path
}

// Stream returns the underlying token stream.
func (f *File) Stream() *token.Stream {
	if f == nil {
		return nil
	}
	return f.stream
}

// Nodes returns the node arena for this file, which can be used to allocate
// new AST nodes.
func (f *File) Nodes() *Nodes {
	return (*Nodes)(f)
}

// Root returns the root fragment for the file.
func (f *File) Root() Fragment {
	return id.Wrap(f, id.ID[Fragment](1))
}

// FromID implements [id.Context].
func (f *File) FromID(id uint64, want any) any {
	switch want.(type) {
	case **rawTagEnd:
		return f.tags.ends.Deref(arena.Pointer[rawTagEnd](id))
	case **rawTagText:
		return f.tags.texts.Deref(arena.Pointer[rawTagText](id))
	case **rawTagExpr:
		return f.tags.exprs.Deref(arena.Pointer[rawTagExpr](id))
	case **rawTagEmit:
		return f.tags.emits.Deref(arena.Pointer[rawTagEmit](id))
	case **rawTagImport:
		return f.tags.imports.Deref(arena.Pointer[rawTagImport](id))
	case **rawTagIf:
		return f.tags.ifs.Deref(arena.Pointer[rawTagIf](id))
	case **rawTagFor:
		return f.tags.fors.Deref(arena.Pointer[rawTagFor](id))
	case **rawTagSwitch:
		return f.tags.switches.Deref(arena.Pointer[rawTagSwitch](id))
	case **rawTagCase:
		return f.tags.cases.Deref(arena.Pointer[rawTagCase](id))
	case **rawTagMacro:
		return f.tags.macros.Deref(arena.Pointer[rawTagMacro](id))

	case **rawFragment:
		return f.fragments.Deref(arena.Pointer[rawFragment](id))

	default:
		return f.stream.FromID(id, want)
	}
}
