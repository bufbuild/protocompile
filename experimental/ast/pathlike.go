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
	"fmt"

	"github.com/bufbuild/protocompile/experimental/token"
)

// pathLike is the raw representation of a type or expression.
//
// The vast, vast majority of types and expressions are paths. To avoid needing
// to waste space for such types, we use the following encoding for rawType.
//
// First, note that if the first half of a rawPath is negative, the other
// must be zero. Thus, if the first "token" of the rawPath is negative and
// the second is not, the first is ^kind and the second is an integer that is
// interpreted depending on kind, such as an arena pointer.
//
// This logic is implemented in the functions below.
type pathLike[Kind ~int8] struct {
	// Can't use Kind for the type because this must be wide
	// enough to accommodate token.ID in the event this
	// pathLike actually represents a path.
	StartOrKind int32
	EndOrValue  int32
}

// wrapInPathLike wraps a integer-like value in a pathLike.
//
// If either of kind or value are zero, both must be.
func wrapPathLike[Value ~int32 | ~uint32, Kind ~int8](kind Kind, value Value) pathLike[Kind] {
	if kind != 0 && value == 0 {
		panic(fmt.Sprintf("protocompile/ast: invalid pathLike representation: %v, %v", kind, value))
	}

	return pathLike[Kind]{
		StartOrKind: ^int32(kind),
		EndOrValue:  int32(value),
	}
}

// unwrapPathLike unwraps a pointer previously wrapped with wrapPathLike.
//
// Returns zero if this pathLike contains the wrong kind, or if it is a real
// path.
func unwrapPathLike[Value ~int32 | ~uint32, Kind ~int8](want Kind, p pathLike[Kind]) Value {
	if got, ok := p.kind(); !ok || got != want {
		return 0
	}

	return Value(p.EndOrValue)
}

// wrapPath wraps a path in a pathLike.
func wrapPath[Kind ~int8](path rawPath) pathLike[Kind] {
	return pathLike[Kind]{
		StartOrKind: int32(path.Start),
		EndOrValue:  int32(path.End),
	}
}

// kind returns the kind within this pathLike, if it is not a path.
func (p pathLike[Kind]) kind() (Kind, bool) {
	if p.StartOrKind < 0 && p.EndOrValue != 0 {
		return Kind(^p.StartOrKind), true
	}
	return 0, false
}

// path converts this pathLike into a Path if it is in fact a genuine path.
func (p pathLike[Kind]) path(c Context) (Path, bool) {
	if _, notPath := p.kind(); notPath {
		return Path{}, false
	}
	return rawPath{Start: token.ID(p.StartOrKind), End: token.ID(p.EndOrValue)}.With(c), true
}
