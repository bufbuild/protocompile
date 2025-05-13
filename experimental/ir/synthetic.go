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
	"strings"

	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/internal/ext/iterx"
	"github.com/bufbuild/protocompile/internal/ext/mapsx"
	"github.com/bufbuild/protocompile/internal/intern"
)

// Set of all names that are defined in scope of some message; used for
// generating synthetic names.
type syntheticNames intern.Set

// generate generates a new synthetic name, according to the rules for synthetic
// oneofs.
//
// Specifically, given a message, we can construct a table of the declared
// single-identifier name of each declaration in the message. This includes the
// names of enum values within a nested enum, due to a C++-friendly language
// bug.
//
// Then, we canonicalize candidate by prepending an underscore to it if it
// doesn't already have one, and then prepending X until we get a name that
// isn't in use yet.
//
// For example, the candidate "foo" will have the sequence of potential
// synthetic names: "_foo" -> "X_foo" -> "XX_foo" -> ...; notably, "_foo" also
// has the same sequence of synthetic names.
func (sn *syntheticNames) generate(candidate string, message Type) string {
	if *sn == nil {
		// The elements within a message that contribute names scoped to that
		// message are:
		//
		// 1. Fields.
		// 2. Extensions (so, more fields).
		// 3. Oneofs.
		// 4. Nested types, including enums, synthetic map entry types and
		//    synthetic group types.
		// 5. Nested enums' values, due to a language bug.
		*sn = mapsx.CollectSet(iterx.Chain(
			seq.Map(message.Members(), Member.InternedName),
			seq.Map(message.Extensions(), Member.InternedName),
			seq.Map(message.Oneofs(), Oneof.InternedName),
			iterx.FlatMap(seq.Values(message.Nested()), func(ty Type) iter.Seq[intern.ID] {
				if !ty.IsEnum() {
					return iterx.Of(ty.InternedName())
				}

				return iterx.Chain(
					iterx.Of(ty.InternedName()),
					// We need to include the enum values' names.
					seq.Map(ty.Members(), Member.InternedName),
				)
			}),
		))
	}

	return sn.generateIn(candidate, &message.Context().session.intern)
}

// generateIn is the part of [syntheticNames.generate] that actually constructs
// the string.
//
// it is outlined so that it can be tested separately.
func (sn *syntheticNames) generateIn(candidate string, table *intern.Table) string {
	// The _ prefix is unconditional, but only if candidate does not already
	// start with one.
	if !strings.HasPrefix(candidate, "_") {
		candidate = "_" + candidate
	}

	// Each time we fail, we add an X to the prefix.
	for !intern.Set(*sn).Add(table, candidate) {
		candidate = "X" + candidate
	}
	return candidate
}
