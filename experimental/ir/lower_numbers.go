package ir

import (
	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/internal/taxa"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/internal/arena"
)

// evaluateFieldNumbers evaluates all non-extension field numbers: that is,
// the numbers in reserved ranges and in non-extension field and enum value
// declarations.
func evaluateFieldNumbers(f File, r *report.Report) {
	for ty := range seq.Values(f.AllTypes()) {
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
			n, ok := evaluateMemberNumber(f.Context(), scope, member.AST().Value(), kind, false, r)
			if ok {
				member.raw.number = n
			}
		}

		for _, raw := range ty.raw.ranges {
			tags := ReservedRange{ty.withContext, ty.Context().arenas.ranges.Deref(raw)}

			switch tags.AST().Kind() {
			case ast.ExprKindRange:
				a, b := tags.AST().AsRange().Bounds()

				start, startOk := evaluateMemberNumber(f.Context(), scope, a, kind, false, r)
				end, endOk := evaluateMemberNumber(f.Context(), scope, b, kind, true, r)

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

				tags.raw.first = start
				tags.raw.last = end

			default:
				n, ok := evaluateMemberNumber(f.Context(), scope, tags.AST(), kind, false, r)
				if !ok {
					continue
				}

				tags.raw.first = n
				tags.raw.last = n
			}
		}
	}

	for extn := range seq.Values(f.AllExtensions()) {
		var kind memberNumber
		switch {
		case extn.Container().IsMessageSet():
			kind = messageSetNumber
		default:
			kind = fieldNumber
		}

		scope := extn.Context().File().Package()
		if ty := extn.Parent(); !ty.IsZero() {
			scope = ty.FullName()
		}

		extn.raw.number, _ = evaluateMemberNumber(f.Context(), scope, extn.AST().Value(), kind, false, r)
	}
}

func evaluateMemberNumber(c *Context, scope FullName, number ast.ExprAny, kind memberNumber, allowMax bool, r *report.Report) (int32, bool) {
	if number.IsZero() {
		return 0, false // Diagnosed for us elsewhere.
	}

	e := &evaluator{
		Context: c,
		Report:  r,
		scope:   scope,
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
func buildFieldNumberRanges(f File, r *report.Report) {
	// First, dump all of the ranges into the intersection set.
	for ty := range seq.Values(f.AllTypes()) {
		for tagRange := range seq.Values(ty.ReservedRanges()) {
			lo, hi := tagRange.Range()
			if lo == 0 || hi == 0 {
				continue // Diagnosed already.
			}
			ty.raw.rangesByNumber.Insert(lo, hi, rawTagRange{
				ptr: arena.Untyped(f.Context().arenas.ranges.Compress(tagRange.raw)),
			})
		}
		for tagRange := range seq.Values(ty.ExtensionRanges()) {
			lo, hi := tagRange.Range()
			if lo == 0 || hi == 0 {
				continue // Diagnosed already.
			}
			ty.raw.rangesByNumber.Insert(lo, hi, rawTagRange{
				ptr: arena.Untyped(f.Context().arenas.ranges.Compress(tagRange.raw)),
			})
		}

		// Members last, so that if an intersection contains only members, the
		// first value is a member.
		for member := range seq.Values(ty.Members()) {
			n := member.Number()
			if n == 0 {
				continue // Diagnosed already.
			}
			ty.raw.rangesByNumber.Insert(n, n, rawTagRange{
				isMember: true,
				ptr:      arena.Untyped(f.Context().arenas.members.Compress(member.raw)),
			})
		}

		// Now, iterate over every entry and diagnose the ones that have more
		// than one value.
		for entry := range ty.raw.rangesByNumber.Entries() {
			if len(entry.Values) < 2 {
				continue
			}

			first := TagRange{ty.withContext, entry.Values[0]}

			for _, tags := range entry.Values[1:] {
				tags := TagRange{ty.withContext, tags}
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
	for extn := range seq.Values(f.AllExtensions()) {
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
