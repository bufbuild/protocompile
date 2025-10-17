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

import (
	"iter"
	"slices"
	"strings"

	"github.com/bufbuild/protocompile/internal/ext/stringsx"
	"github.com/bufbuild/protocompile/internal/ext/unsafex"
)

// FullName is a fully-qualified Protobuf name, which is a dot-separated list of
// identifiers, with an optional dot prefix.
//
// This is a helper type for common operations on such names. This is essentially
// protoreflect.FullName, without depending on protoreflect. Unlike protoreflect,
// we do not provide validation methods.
type FullName string

// Absolute returns whether this is an absolute name, i.e., has a leading dot.
func (n FullName) Absolute() bool {
	return n != "" && n[0] == '.'
}

// IsIdent returns whether this name is a single identifier.
func (n FullName) IsIdent() bool {
	return !strings.Contains(string(n), ".")
}

// ToAbsolute returns this name with a leading dot.
func (n FullName) ToAbsolute() FullName {
	if n.Absolute() {
		return n
	}
	return "." + n
}

// ToRelative returns this name without a leading dot.
func (n FullName) ToRelative() FullName {
	return FullName(strings.TrimPrefix(string(n), "."))
}

// First returns the first component of this name.
func (n FullName) First() string {
	n = n.ToRelative()
	name, _, _ := strings.Cut(string(n), ".")
	return name
}

// Components returns an iterator over the components of this name.
//
// If there are adjacent dots, e.g. foo..bar, this will yield an empty string
// within the name.
func (n FullName) Components() iter.Seq[string] {
	return func(yield func(string) bool) {
		n = n.ToRelative()
		for {
			name, rest, more := strings.Cut(string(n), ".")
			if !yield(name) || !more {
				return
			}
			n = FullName(rest)
		}
	}
}

// Name returns the last component of this name.
func (n FullName) Name() string {
	_, name, _ := stringsx.CutLast(string(n), ".")
	return name
}

// Parent returns the name of the parent entity for this name.
//
// If the name only has one component, returns the zero value. In particular,
// the parent of ".foo" is "".
func (n FullName) Parent() FullName {
	parent, _, _ := stringsx.CutLast(string(n), ".")
	return FullName(parent)
}

// Append returns a name with the given component(s) appended.
//
// If this is an empty name, the resulting name will not be absolute.
func (n FullName) Append(names ...string) FullName {
	if len(names) == 0 {
		return n
	}

	return FullName(unsafex.StringAlias(n.appendToBytes(nil, names...)))
}

// appendToBytes is like [FullName.Append], but it appends to the given slice.
func (n FullName) appendToBytes(b []byte, names ...string) []byte {
	if len(names) == 0 {
		return append(b, n...)
	}

	m := len(n) + len(names) - 1
	if n != "" {
		m++
	}
	for _, name := range names {
		m += len(name)
	}

	b = slices.Grow(b, m)
	b = append(b, n...)

	for _, name := range names {
		if len(b) > 0 {
			b = append(b, '.')
		}
		b = append(b, name...)
	}

	return b
}
