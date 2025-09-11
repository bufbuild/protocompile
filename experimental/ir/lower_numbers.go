package ir

import (
	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/internal/taxa"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/internal/arena"
	"github.com/bufbuild/protocompile/internal/ext/iterx"
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
			member.raw.number, _ = evaluateMemberNumber(f.Context(), scope, member.AST().Value(), kind, false, r)
		}

		for _, raw := range ty.raw.ranges {
			tags := ReservedRange{ty.withContext, ty.Context().arenas.ranges.Deref(raw)}

			switch tags.AST().Kind() {
			case ast.ExprKindRange:
				a, startOk := evaluateMemberNumber(f.Context(), scope, tags.AST(), kind, false, r)
				b, endOk := evaluateMemberNumber(f.Context(), scope, tags.AST(), kind, true, r)

				if !startOk || !endOk {
					continue
				}

				if a <= b {
					what := taxa.Reserved
					if tags.ForExtensions() {
						what = taxa.Extensions
					}

					r.Errorf("empty %s", what).Apply(
						report.Snippet(tags.AST()),
						report.Notef("`a to b` requires that a < b"),
					)
					continue
				}

				tags.raw.first = a
				tags.raw.last = b

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
	for ty := range seq.Values(f.Types()) {
		what := taxa.FieldNumber
		if ty.IsEnum() {
			what = taxa.EnumValue
		}

		ranges := iterx.Chain(
			seq.Values(ty.ReservedRanges()),
			seq.Values(ty.ExtensionRanges()),
		)

		// Do the ranges first, so we can check whether ranges overlap or not.
		for tagRange := range ranges {
			a, b := tagRange.Range()
			if a == 0 || b == 0 {
				continue // Diagnosed already.
			}

			e := ty.raw.rangesByNumber.Insert(int64(a), int64(b)+1, rawTagRange{
				ptr: arena.Untyped(f.Context().arenas.ranges.Compress(tagRange.raw)),
			})
			if e.Value == nil {
				continue
			}

			// Only ranges inserted so far.
			tags := TagRange{ty.withContext, *e.Value}.AsReserved()

			c, d := tags.Range()
			r.Errorf("overlapping %v ranges", what).Apply(
				report.Snippet(tagRange.AST()),
				report.Snippetf(tags.AST(), "overlaps with this one"),
				report.Helpf("they overlap in the range `%v to %v`", max(a, c), min(b, d)),
			)
		}

		for member := range seq.Values(ty.Members()) {
			n := member.Number()
			if n == 0 {
				continue // Diagnosed already.
			}

			e := ty.raw.rangesByNumber.Insert(int64(n), int64(n)+1, rawTagRange{
				isMember: true,
				ptr:      arena.Untyped(f.Context().arenas.members.Compress(member.raw)),
			})
			if e.Value == nil {
				continue
			}
			tags := TagRange{ty.withContext, *e.Value}

			if reserved := tags.AsReserved(); reserved.IsZero() {
				r.Errorf("use of reserved %v `%v`", what, n).Apply(
					report.Snippetf(member.AST().Value(), "used here"),
					report.Snippetf(reserved.AST(), "%v reserved here", what),
				)
			} else {
				if ty.AllowsAlias() {
					continue
				}

				prev := tags.AsMember()
				r.Errorf("%v `%v` used more than once", what, n).Apply(
					report.Snippetf(member.AST().Value(), "used here"),
					report.Snippetf(prev.AST().Value(), "but also used here"),
				)
			}
		}
	}

	for extn := range seq.Values(f.AllExtensions()) {
		n := extn.Number()
		if n == 0 {
			continue // Diagnosed already.
		}

		ty := extn.Container()
		tags := ty.TagRange(n)
		if tags.IsZero() {
			r.Errorf("extension with unreserved number `%v`", n).Apply(
				report.Snippet(extn.AST().Value()),
				report.Helpf("the parent message must have reserved this number with an %s", taxa.Extensions),
			)
			continue
		}

		if member := tags.AsMember(); !member.IsZero() {
			r.Errorf("%v `%v` used more than once", taxa.FieldNumber, n).Apply(
				report.Snippetf(extn.AST().Value(), "used here"),
				report.Snippetf(member.AST().Value(), "but also used here"),
			)
			continue
		}

		reserved := tags.AsReserved()
		if !reserved.ForExtensions() {
			r.Errorf("use of reserved %v `%v`", taxa.FieldNumber, n).Apply(
				report.Snippetf(extn.AST().Value(), "used here"),
				report.Snippetf(reserved.AST(), "%v reserved here", taxa.FieldNumber),
				report.Helpf("the parent message must have reserved this number with an %s", taxa.Extensions),
			)
		}

		// By process of elimination, we are in a valid extension range.
	}

	// NOTE: Can't do extension number overlap checking yet, because we need
	// a global view of all files to do that.
}
