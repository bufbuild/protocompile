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
	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/id"
	"github.com/bufbuild/protocompile/experimental/internal/taxa"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/internal/arena"
	"github.com/bufbuild/protocompile/internal/ext/iterx"
)

// evaluateFieldNumbers evaluates all non-extension field numbers: that is,
// the numbers in reserved ranges and in non-extension field and enum value
// declarations.
func evaluateFieldNumbers(file *File, r *report.Report) {
	for ty := range seq.Values(file.AllTypes()) {
		if !ty.MapField().IsZero() {
			// Map entry types come with numbers pre-calculated.
			continue
		}

		scope := ty.FullName()

		var kind memberNumber
		switch {
		case ty.IsEnum():
			kind = enumNumber
		case ty.IsMessageSet():
			kind = messageSetNumber
		default:
			kind = fieldNumber
		}

		for member := range seq.Values(ty.Members()) {
			member.Raw().number, member.Raw().numberOk = evaluateMemberNumber(
				file, scope, member.AST().Value(), kind, false, r)
		}

		for tags := range seq.Values(ty.AllRanges()) {
			switch tags.AST().Kind() {
			case ast.ExprKindRange:
				a, b := tags.AST().AsRange().Bounds()

				start, startOk := evaluateMemberNumber(file, scope, a, kind, false, r)
				end, endOk := evaluateMemberNumber(file, scope, b, kind, true, r)

				if !startOk || !endOk {
					continue
				}

				if start > end {
					what := taxa.Reserved
					if tags.ForExtensions() {
						what = taxa.Extensions
					}

					r.Errorf("empty %s %v %v", what).Apply(
						report.Snippet(tags.AST()),
						report.Notef("range syntax requires that start <= end"),
					)
					continue
				}

				if start == end {
					r.Warnf("singleton range can be simplified").Apply(
						report.Snippet(tags.AST()),
						report.SuggestEdits(tags.AST(), "replace with a single number", report.Edit{
							Start: 0, End: tags.AST().Span().Len(),
							Replace: a.Span().Text(),
						}),
					)
				}

				tags.Raw().first = start
				tags.Raw().last = end
				tags.Raw().rangeOk = startOk && endOk

			default:
				n, ok := evaluateMemberNumber(file, scope, tags.AST(), kind, false, r)
				if !ok {
					continue
				}

				tags.Raw().first = n
				tags.Raw().last = n
				tags.Raw().rangeOk = ok
			}
		}
	}

	for extn := range seq.Values(file.AllExtensions()) {
		var kind memberNumber
		switch {
		case extn.Container().IsMessageSet():
			kind = messageSetNumber
		default:
			kind = fieldNumber
		}

		scope := extn.Context().Package()
		if ty := extn.Parent(); !ty.IsZero() {
			scope = ty.FullName()
		}

		extn.Raw().number, extn.Raw().numberOk = evaluateMemberNumber(
			file, scope, extn.AST().Value(), kind, false, r)
	}
}

func evaluateMemberNumber(file *File, scope FullName, number ast.ExprAny, kind memberNumber, allowMax bool, r *report.Report) (int32, bool) {
	if number.IsZero() {
		return 0, false // Diagnosed for us elsewhere.
	}

	e := &evaluator{
		File:   file,
		Report: r,
		scope:  scope,
	}

	// Don't bother allocating a whole Value for this.
	v, ok := e.evalBits(evalArgs{
		expr:         number,
		memberNumber: kind,
		allowMax:     allowMax,
	})

	return int32(v), ok
}

// buildFieldNumberRanges builds the field number range table for all types.
//
// This also checks for and diagnoses overlaps.
func buildFieldNumberRanges(file *File, r *report.Report) {
	// overlapLimit sets the maximum number of overlapping ranges we tolerate
	// before we stop processing overlapping ranges.
	//
	// Inserting to an [interval.Intersect] is actually O(k log n), where k is
	// the number of intersecting intervals. No valid Protobuf file can trigger
	// this behavior, but pathological files can, such as the fragment
	//
	//	reserved 1, 1 to 2, 1 to 3, 1 to 4, ...
	//
	// which forces k = n. This can be mitigated by not processing more ranges
	// after we see N overlaps This ensures k = O(N). For example, in the
	// example above, we would only have ranges up to 1 to N before stopping,
	// and so we get at worst performance O(N^2 log n) = O(log n), because N
	// is a constant. Note processing members still proceeds, and is still
	// O(n log n) work (because k <= 1 in this case).
	//
	// This is that N. In this case, we set missingRanges so that extension
	// range checks are skipped, to avoid generating incorrect diagnostics (the
	// file will already be rejected in this case, so not providing precise
	// diagnostics in is fine).
	const overlapLimit = 50

	// First, dump all of the ranges into the intersection set.
	for ty := range seq.Values(file.AllTypes()) {
		var totalOverlaps int
		for tagRange := range iterx.Chain(
			seq.Values(ty.ReservedRanges()),
			seq.Values(ty.ExtensionRanges()),
		) {
			lo, hi := tagRange.Range()
			if !tagRange.Raw().rangeOk {
				continue // Diagnosed already.
			}
			disjoint := ty.Raw().rangesByNumber.Insert(lo, hi, rawTagRange{
				ptr: arena.Untyped(file.arenas.ranges.Compress(tagRange.Raw())),
			})

			// Avoid quadratic behavior. See overlapLimit's comment above.
			if !disjoint {
				totalOverlaps++
				if totalOverlaps > overlapLimit {
					ty.Raw().missingRanges = true
					break
				}
			}
		}

		// Members last, so that if an intersection contains only members, the
		// first value is a member.
		for member := range seq.Values(ty.Members()) {
			n := member.Number()
			if !member.Raw().numberOk {
				continue // Diagnosed already.
			}
			ty.Raw().rangesByNumber.Insert(n, n, rawTagRange{
				isMember: true,
				ptr:      arena.Untyped(file.arenas.members.Compress(member.Raw())),
			})
		}

		// Now, iterate over every entry and diagnose the ones that have more
		// than one value.
		for entry := range ty.Raw().rangesByNumber.Entries() {
			if len(entry.Value) < 2 {
				continue
			}

			first := TagRange{id.WrapContext(ty.Context()), entry.Value[0]}
			if ty.AllowsAlias() && first.raw.isMember {
				// If all of the members of the intersections are members,
				// we don't diagnose.
				continue
			}

			for _, tags := range entry.Value[1:] {
				tags := TagRange{id.WrapContext(ty.Context()), tags}
				if a, b := first.AsMember(), tags.AsMember(); ty.AllowsAlias() && !a.IsZero() && !b.IsZero() {
					continue
				}

				r.Error(errOverlap{
					ty:     ty,
					first:  first,
					second: tags,
				})
			}
		}
	}

	// Check that every extension has a corresponding extension range.
extensions:
	for extn := range seq.Values(file.AllExtensions()) {
		n := extn.Number()
		if n == 0 {
			continue // Diagnosed already.
		}

		ty := extn.Container()
		if ty.IsZero() || !ty.IsMessage() {
			continue
		}

		var first TagRange
		for tags := range ty.Ranges(n) {
			if first.IsZero() {
				first = tags
			}

			if tags.AsReserved().ForExtensions() {
				continue extensions
			}
		}

		var d *report.Diagnostic
		if !first.IsZero() {
			d = r.Error(errOverlap{
				ty:     ty,
				first:  first,
				second: extn.AsTagRange(),
			})
		} else {
			// Don't diagnose if we're missing some ranges, because we might
			// produce false positives. This can only happen for types that have
			// already generated diagnostics, so it's ok to skip diagnosing.
			if ty.Raw().missingRanges {
				continue
			}

			d = r.Errorf("extension with unreserved number `%v`", n).Apply(
				report.Snippet(extn.AST().Value()),
			)
		}
		d.Apply(report.Helpf(
			"the parent message `%s` must have reserved this number with an %s, e.g. `extensions %v;`",
			ty.FullName(), taxa.Extensions, extn.Number(),
		))
	}

	// NOTE: Can't do extension number overlap checking yet, because we need
	// a global view of all files to do that.
}

type errOverlap struct {
	ty            Type
	first, second TagRange
}

func (e errOverlap) Diagnose(d *report.Diagnostic) {
	what := taxa.FieldNumber
	if e.ty.IsEnum() {
		what = taxa.EnumValue
	}

again:
	if second := e.second.AsMember(); !second.IsZero() {
		if first := e.first.AsMember(); !first.IsZero() {
			d.Apply(
				report.Message("%v `%v` used more than once", what, second.Number()),
				report.Snippetf(second.AST().Value(), "used here"),
				report.Snippetf(first.AST().Value(), "previously used here"),
			)
		} else {
			first := e.first.AsReserved()
			d.Apply(
				report.Message("use of reserved %v `%v`", what, second.Number()),
				report.Snippetf(second.AST().Value(), "used here"),
				report.Snippetf(first.AST(), "%v reserved here", what),
			)
		}
	} else {
		if first := e.first.AsMember(); !first.IsZero() {
			e.second, e.first = e.first, e.second
			goto again
		}

		second := e.second.AsReserved()
		first := e.first.AsReserved()

		lo1, hil := first.Range()
		lo2, hi2 := second.Range()
		d.Apply(
			report.Message("overlapping %v ranges", what),
			report.Snippetf(second.AST(), "this range"),
			report.Snippetf(first.AST(), "overlaps with this one"),
		)

		lo1 = max(lo1, lo2)
		hil = min(hil, hi2)
		if lo1 == hil {
			d.Apply(report.Helpf("they overlap at `%v`", lo1))
		} else {
			d.Apply(report.Helpf("they overlap in the range `%v to %v`", lo1, hil))
		}

		// TODO: Generate a suggestion to split the range, if both ranges are
		// of the same type.
	}
}
