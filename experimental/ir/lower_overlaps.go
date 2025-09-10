package ir

import (
	"github.com/bufbuild/protocompile/experimental/internal/taxa"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/internal/ext/iterx"
	"github.com/bufbuild/protocompile/internal/interval"
)

// checkNumberOverlaps checks that the field numbers of a message or enum
// type do not overlap.
func checkNumberOverlaps(f File, r *report.Report) {
	type entry struct {
		member   Member   // If this came from a field/enum value.
		reserved TagRange // If this came from a reserved/extensions decl.
	}

	for t := range seq.Values(f.Types()) {
		// We use 64-bit numbers here, specifically to avoid wrap-around problems
		// when users use Int32Max as a field number.
		numbers := new(interval.Map[int64, entry])

		what := taxa.FieldNumber
		if t.IsEnum() {
			what = taxa.EnumValue
		}

		ranges := iterx.Chain(
			seq.Values(t.ReservedRanges()),
			seq.Values(t.ExtensionRanges()),
		)

		for tagRange := range ranges {
			a, b := tagRange.Range()
			if a == 0 || b == 0 {
				continue // Diagnosed already.
			}

			e := numbers.Insert(int64(a), int64(b)+1, entry{reserved: tagRange})
			if e.Value == nil {
				continue
			}

			c, d := e.Value.reserved.Range()
			r.Errorf("overlapping %v ranges", what).Apply(
				report.Snippet(tagRange.AST()),
				report.Snippetf(e.Value.reserved.AST(), "overlaps with this one"),
				report.Helpf("they overlap in the range `%v to %v`", max(a, c), min(b, d)),
			)
		}

		for member := range seq.Values(t.Members()) {
			n := member.Number()
			if n == 0 {
				continue // Diagnosed already.
			}

			e := numbers.Insert(int64(n), int64(n)+1, entry{member: member})
			if e.Value == nil {
				continue
			}

			// TODO: allow_alias in enums.

			if !e.Value.reserved.IsZero() {
				r.Errorf("use of reserved %v `%v`", what, n).Apply(
					report.Snippetf(member.AST().Value(), "used here"),
					report.Snippetf(e.Value.reserved.AST(), "%v reserved here", what),
				)
			} else {
				r.Errorf("%v `%v` used more than once", what, n).Apply(
					report.Snippetf(member.AST().Value(), "used here"),
					report.Snippetf(e.Value.member.AST().Value(), "but also used here"),
				)
			}
		}
	}
}
