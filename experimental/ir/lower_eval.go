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
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"

	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/ast/predeclared"
	"github.com/bufbuild/protocompile/experimental/internal"
	"github.com/bufbuild/protocompile/experimental/internal/taxa"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/experimental/token/keyword"
	"github.com/bufbuild/protocompile/internal/ext/mapsx"
	"google.golang.org/protobuf/encoding/protowire"
)

const fieldNumberBits = 29

// evaluateFieldNumbers evaluates all non-extension field numbers: that is,
// the numbers in reserved ranges and in non-extension field and enum value
// declarations.
func evaluateFieldNumbers(f File, r *report.Report) {
	// TODO: Evaluate reserved ranges.

	for ty := range seq.Values(f.AllTypes()) {
		tags := make(map[int32]Field, ty.Fields().Len())
		for field := range seq.Values(ty.Fields()) {
			n, ok := evaluateFieldNumber(field, r)
			field.raw.number = n
			if !ok {
				continue
			}

			// TODO: Need to check allow_alias here.
			if first, ok := mapsx.Add(tags, n, field); !ok {
				what := taxa.FieldNumber
				if ty.IsEnum() {
					what = taxa.EnumValue
				}
				r.Errorf("%ss must be unique", what).Apply(
					report.Snippetf(field.AST().Value(), "used again here"),
					report.Snippetf(first.AST().Value(), "first used here"),
				)
			}
		}

		// TODO: compare with extension/reserved ranges.
	}
}

// evaluateExtensionNumbers evaluates all extension field numbers: that is,
// the numbers on extension ranges and extension fields.
func evaluateExtensionNumbers(f File, r *report.Report) {
	// TODO: Evaluate extension ranges.

	for extn := range seq.Values(f.AllExtensions()) {
		n, _ := evaluateFieldNumber(extn, r)
		extn.raw.number = n

		// TODO: compare with extension ranges.
	}
}

func evaluateFieldNumber(field Field, r *report.Report) (int32, bool) {
	if field.AST().Value().IsZero() {
		return 0, false // Diagnosed for us elsewhere.
	}

	e := &evaluator{
		Context: field.Context(),
		Report:  r,
		scope:   field.FullName().Parent(),
	}

	// Don't bother allocating a whole Value for this.
	v, ok := e.evalBits(evalArgs{
		expr:   field.AST().Value(),
		uint29: field.IsMessageField(), // TODO: MessageSet.
	})

	return int32(v), ok
}

// evaluator is the context needed to evaluate an expression.
type evaluator struct {
	*Context
	*report.Report
	scope FullName
}

type evalArgs struct {
	expr ast.ExprAny // The expression to evaluate.

	// The field that this value maps to, if evaluating an option.
	// If not set, we assume that we're evaluating a field number of some kind.
	field ref[rawField]
	// A span for whatever caused the above field to be selected.
	annotation report.Spanner

	allowMax bool // Whether the max keyword is to be honored.
	uint29   bool // Whether this is a 29-bit field number.
}

func (ea evalArgs) Field(c *Context) Field {
	return wrapField(c, ea.field)
}

func (ea evalArgs) Type(c *Context) Type {
	return ea.Field(c).Element()
}

// mismatch constructs a type mismatch error.
func (ea evalArgs) mismatch(c *Context, got any) errTypeCheck {
	var want any
	if ty := ea.Type(c); !ty.IsZero() {
		want = ty
	} else if ea.uint29 {
		want = taxa.FieldNumber
	} else {
		want = PredeclaredType(predeclared.Int32)
	}

	return errTypeCheck{
		want:       want,
		got:        got,
		expr:       ea.expr,
		annotation: ea.annotation,
	}
}

// eval evaluates an expression into a value.
//
// Returns a zero value if type-checking fails.
//
//nolint:unused // Will be used by options lowering.
func (e *evaluator) eval(args evalArgs) Value {
	bits, ok := e.evalBits(args)
	if !ok {
		return Value{}
	}
	return Value{
		internal.NewWith(e.Context),
		e.Context.arenas.values.New(rawValue{
			expr:  args.expr,
			field: args.field,
			bits:  bits,
		}),
	}
}

// evalBits evaluates an expression, returning raw bits for the computed value.
//
// [evaluator.eval] is preferred; this is a separate function for the benefit
// of field number evaluation.
func (e *evaluator) evalBits(args evalArgs) (rawValueBits, bool) {
	switch args.expr.Kind() {
	case ast.ExprKindInvalid, ast.ExprKindError:
		return 0, false

	case ast.ExprKindLiteral:
		return e.evalLiteral(args, args.expr.AsLiteral(), false)

	case ast.ExprKindPath:
		return e.evalPath(args, args.expr.AsPath().Path)

	case ast.ExprKindPrefixed:
		expr := args.expr.AsPrefixed()

		inner := expr.Expr()
		switch expr.Prefix() {
		case keyword.Minus:
			// Special handling to ensure that negative literals work correctly.
			if inner.AsLiteral().Kind() == token.Number {
				return e.evalLiteral(args, inner.AsLiteral(), true)
			}

			// Special cases for certain literals.
			if inner.AsPath().AsKeyword() == keyword.Inf {
				v, ok := e.evalPath(args, inner.AsPath().Path)
				v |= 0x8000_0000_0000_0000 // Set the floating-point sign bit.
				return v, ok
			}

			// All other expressions cannot have a leading -.
			err := args.mismatch(e.Context, taxa.Classify(inner))
			err.want = taxa.Number
			return 0, false
		default:
			panic("unreachable")
		}

	case ast.ExprKindArray:
		sorry("array exprs")
	case ast.ExprKindDict:
		sorry("message exprs")

	case ast.ExprKindField:
		break // Legalized in the parser.
	case ast.ExprKindRange:
		e.Error(args.mismatch(e.Context, taxa.Range))

	default:
		panic("unexpected ast.ExprKind")
	}

	return 0, false
}

// evalLiteral evaluates a literal expression.
func (e evaluator) evalLiteral(args evalArgs, expr ast.ExprLiteral, neg bool) (rawValueBits, bool) {
	scalar := predeclared.Int32
	if ty := args.Type(e.Context); !ty.IsZero() {
		scalar = ty.Predeclared()
	}

	switch expr.Kind() {
	case token.Number:
		if n, ok := expr.AsInt(); ok {
			if !scalar.IsNumber() {
				e.Error(args.mismatch(e.Context, taxa.Int))
				return 0, false
			}

			if args.uint29 {
				return e.checkIntBounds(args, false, fieldNumberBits, neg, n)
			}
			return e.checkIntBounds(args, scalar.IsSigned(), scalar.Bits(), neg, n)
		}

		if n := expr.AsBigInt(); n != nil {
			if !scalar.IsNumber() {
				e.Error(args.mismatch(e.Context, taxa.Int))
				return 0, false
			}

			if args.uint29 {
				return e.checkIntBounds(args, false, fieldNumberBits, neg, n)
			}
			return e.checkIntBounds(args, scalar.IsSigned(), scalar.Bits(), neg, n)
		} else if n, ok := expr.AsFloat(); ok {
			if !scalar.IsFloat() {
				e.Error(args.mismatch(e.Context, taxa.Float))
				return 0, false
			}

			if neg {
				n = -n
			}

			// 32-bit floats are stored as 64-bit floats; this conversion is
			// lossless.
			return rawValueBits(math.Float64bits(n)), true
		}

	case token.String:
		sorry("string literals")
	}

	return 0, false
}

// checkIntBounds checks that an integer is within the bounds of a possibly
// signed value with the given number of bits. Failure results in a saturated
// result.
//
// If neg is set, this means that the expression had a - out in front of it.
//
// If bits == fieldNumberBits, the field number bounds check is used instead, which disallows
// 0 and values in the implementation-reserved range.
func (e *evaluator) checkIntBounds(args evalArgs, signed bool, bits int, neg bool, got any) (rawValueBits, bool) {
	err := func() *report.Diagnostic {
		return e.Error(errLiteralRange{
			errTypeCheck: args.mismatch(e.Context, nil),
			got:          got,
			signed:       signed,
			bits:         bits,
		})
	}

	var tooLarge bool
	var v uint64
	switch n := got.(type) {
	case uint64:
		v = n
	case *big.Int:
		// We assume that a big.Int is always larger than a uint64.
		tooLarge = true
	}

	if signed {
		hi := (int64(1) << (bits - 1)) - 1
		lo := ^hi // Ensure that lo is sign-extended to 64 bits.

		if neg {
			v = -v
		}
		v := int64(v)

		if (neg && tooLarge) || v < lo {
			err()
			return rawValueBits(lo), false
		}
		if (!neg && tooLarge) || v > hi {
			err()
			return rawValueBits(hi), false
		}
	} else {
		if neg {
			err()
			return 0, false
		}

		hi := (uint64(1) << bits) - 1
		if v > hi {
			err()
			return rawValueBits(hi), false
		}
	}

	if bits == fieldNumberBits {
		n := protowire.Number(v)
		if n == 0 {
			err()
			return 0, false
		}

		// Check that this is not one of the special reserved numbers.
		if n >= protowire.FirstReservedNumber && n <= protowire.LastReservedNumber {
			err()
			return rawValueBits(v), false
		}
	}

	return rawValueBits(v), true
}

// evalPath evaluates a path expression.
func (e evaluator) evalPath(args evalArgs, expr ast.Path) (rawValueBits, bool) {
	if ty := args.Type(e.Context); ty.IsEnum() {
		// We can just plumb the text of the expression directly here, since
		// if it's anything that isn't an identifier, this lookup will fail.
		value := ty.FieldByName(expr.Span().Text())

		// TODO: This depends on field numbers being resolved before options,
		// but some options need to be resolved first.
		if !value.IsZero() {
			return rawValueBits(value.Number()), true
		}
	}

	scalar := predeclared.Int32
	if ty := args.Type(e.Context); !ty.IsZero() {
		scalar = ty.Predeclared()
	}

	// If we see a name that matches one of the predeclared names, resolve
	// to it, just like it would for type lookup.
	switch name := expr.AsPredeclared(); name {
	case predeclared.Max:
		if !scalar.IsNumber() {
			e.Error(args.mismatch(e.Context, taxa.PredeclaredMax))
			return 0, false
		}

		ok := args.allowMax
		if !ok {
			e.Errorf("%s outside of %s", taxa.PredeclaredMax, taxa.Range).Apply(
				report.Snippet(expr),
				report.Notef(
					"the special %s expression is only allowed in a %s",
					taxa.PredeclaredMax, taxa.Range),
			)
		}

		if args.uint29 {
			return rawValueBits(protowire.MaxValidNumber), ok
		}

		if scalar.IsFloat() {
			return rawValueBits(math.Float64bits(math.Inf(0))), ok
		}

		n := uint64(1) << scalar.Bits()
		if scalar.IsSigned() {
			n >>= 1
		}
		n--
		return rawValueBits(n), ok

	case predeclared.True, predeclared.False:
		if scalar != predeclared.Bool {
			e.Error(args.mismatch(e.Context, PredeclaredType(predeclared.Bool)))
			return 0, false
		}

		switch name {
		case predeclared.False:
			return 0, true
		case predeclared.True:
			return 1, true
		}

	case predeclared.Inf, predeclared.NAN:
		if !scalar.IsFloat() {
			e.Error(args.mismatch(e.Context, taxa.Float))
			return 0, false
		}

		switch name {
		case predeclared.Inf:
			return rawValueBits(math.Float64bits(math.Inf(0))), true
		case predeclared.NAN:
			return rawValueBits(math.Float64bits(math.NaN())), true
		}
	}

	// Perform symbol lookup in the current scope. This isn't what protoc
	// does, but it allows us to produce better diagnostics.
	sym := symbolRef{
		Context: e.Context,
		Report:  e.Report,

		scope: e.scope,
		name:  FullName(expr.Canonicalized()),

		allowScalars: true,
	}.resolve()

	if !sym.IsZero() {
		e.Error(args.mismatch(e.Context, sym))
	}
	return 0, false
}

// errTypeCheck is a type-checking failure.
type errTypeCheck struct {
	want, got any

	expr       report.Spanner
	annotation report.Spanner
}

// Diagnose implements [report.Diagnose].
func (e errTypeCheck) Diagnose(d *report.Diagnostic) {
	strings := func(v any) (name, what string) {
		type symbol interface {
			FullName() FullName
			noun() taxa.Noun
		}

		if sym, ok := v.(symbol); ok {
			name = "`" + string(sym.FullName()) + "`"
			return name, sym.noun().String() + " " + name
		}

		name = fmt.Sprint(v)
		return name, name
	}

	wantName, wantWhat := strings(e.want)
	gotName, gotWhat := strings(e.got)

	d.Apply(
		report.Message("mismatched types"),
		report.Snippetf(e.expr, "expected %s, found %s", wantName, gotName),
		report.Notef("expected: %s\n   found: %s", wantWhat, gotWhat),
	)
	if e.annotation != nil {
		d.Apply(report.Snippetf(e.annotation, "expected due to this"))
	}
}

// errLiteralRange is like [errTypeCheck], but is specifically about integer
// ranges.
type errLiteralRange struct {
	errTypeCheck
	got    any
	signed bool
	bits   int
}

func (e errLiteralRange) Diagnose(d *report.Diagnostic) {
	name := e.want
	if sym, ok := e.want.(interface{ FullName() FullName }); ok {
		name = "`" + string(sym.FullName()) + "`"
	}

	var lo, hi uint64
	var sign string
	if e.signed {
		sign = "-"
		lo = uint64(1) << (e.bits - 1)
		hi = lo - 1
	} else {
		hi = (uint64(1) << e.bits) - 1
	}

	var base int
	var prefix string
	text := e.expr.Span().Text()
	text = text[strings.IndexAny(text, "0123456789xXoObB"):]

	switch {
	case strings.HasPrefix(text, "0x"), strings.HasPrefix(text, "0X"):
		base = 16
		prefix = text[:2]
	case text != "0" && strings.HasPrefix(text, "0"):
		base = 8
		prefix = "0"
	case strings.HasPrefix(text, "0o"), strings.HasPrefix(text, "0O"):
		base = 8
		prefix = text[:2]
	case strings.HasPrefix(text, "0b"), strings.HasPrefix(text, "0B"):
		base = 2
		prefix = text[:2]
	default:
		base = 10
	}

	itoa := func(v uint64) string {
		return prefix + strconv.FormatUint(v, base)
	}

	if e.bits == fieldNumberBits {
		d.Apply(
			report.Message("%s out of range", taxa.FieldNumber),
			report.Snippet(e.expr),
			report.Notef("the range for %ss is `%v to %v`,\n"+
				"minus `%v to %v`, which is reserved for internal use",
				taxa.FieldNumber,
				itoa(1),
				itoa(hi),
				itoa(uint64(protowire.FirstReservedNumber)),
				itoa(uint64(protowire.LastReservedNumber))),
		)
	} else {
		d.Apply(
			report.Message("literal out of range for %s", name),
			report.Snippet(e.expr),
			report.Notef("the range for %s is `%v%v to %v`", name, sign,
				itoa(lo), itoa(hi)),
		)
	}

	if e.annotation != nil {
		d.Apply(report.Snippetf(e.annotation, "expected due to this"))
	}
}
