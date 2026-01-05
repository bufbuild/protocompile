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

package ir

import "github.com/bufbuild/protocompile/internal/arena"

// Exported symbols for test use only. Placing such symbols in a _test.go
// file avoids them being exported "for real".

type (
	Imports = imports
	Symtab  = symtab
)

func NewFile(s *Session, path string) *File {
	return &File{
		session: s,
		path:    s.intern.Intern(path),
	}
}

func GetImports(f *File) *Imports {
	return &f.imports
}

func (s Symbol) RawData() arena.Untyped {
	if s.IsZero() {
		return arena.Nil()
	}
	return s.Raw().data
}
