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

// Package taxa (plural of taxon, an element of a taxonomy) provides support for
// classifying Protobuf syntax productions for use in the parser and in
// diagnostics.
//
// The Subject enum is also used in the parser stack as a simple way to inform
// recursive descent calls of what their caller is, since the What enum
// represents "everything" the parser stack pushes around.
package taxa

import "fmt"

//go:generate go run github.com/bufbuild/protocompile/internal/enum noun.yaml

// In is a shorthand for the "in" preposition.
func (s Noun) In() Place {
	return Place{s, "in"}
}

// After is a shorthand for the "after" preposition.
func (s Noun) After() Place {
	return Place{s, "after"}
}

// Without is a shorthand for the "without" preposition.
func (s Noun) Without() Place {
	return Place{s, "without"}
}

// On is a shorthand for the "on" preposition.
func (s Noun) On() Place {
	return Place{s, "on"}
}

// AsSet returns a singleton set containing this What.
func (s Noun) AsSet() Set {
	return NewSet(s)
}

// Place is a location within the grammar that can be referred to within a
// diagnostic.
//
// It corresponds to a prepositional phrase in English, so it is actually
// somewhat more general than a place, and more accurately describes a general
// state of being.
type Place struct {
	subject     Noun
	preposition string
}

// Subject returns this place's subject.
func (p Place) Subject() Noun {
	return p.subject
}

// String implements [fmt.Stringer].
func (p Place) String() string {
	return p.preposition + " " + p.subject.String()
}

// GoString implements [fmt.GoStringer].
//
// This exists to get pretty output out of the assert package.
func (p Place) GoString() string {
	return fmt.Sprintf("{%#v, %#v}", p.subject, p.preposition)
}
