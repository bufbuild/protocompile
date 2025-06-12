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
	"github.com/bufbuild/protocompile/experimental/internal/taxa"
	"github.com/bufbuild/protocompile/experimental/ir/presence"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/experimental/token/keyword"
	"github.com/bufbuild/protocompile/internal/ext/iterx"
	"github.com/bufbuild/protocompile/internal/ext/mapsx"
	"github.com/bufbuild/protocompile/internal/ext/slicesx"
	"github.com/bufbuild/protocompile/internal/ext/stringsx"
)

const (
	fieldNumberBits = 29
	fieldNumberMax  = 1<<fieldNumberBits - 1
	firstReserved   = 19000
	lastReserved    = 19999

	messageSetNumberBits = 31
	messageSetNumberMax  = 1<<messageSetNumberBits - 1

	enumNumberBits = 32
	enumNumberMax  = math.MaxInt32

	// These are the NaN bits used by protoc. Go's math.NaN() does not specify
	// which NaN it returns, but it isn't this one.
	nanBits = 0x7ff8000000000000
)

// evaluateFieldNumbers evaluates all non-extension field numbers: that is,
// the numbers in reserved ranges and in non-extension field and enum value
// declarations.
func evaluateFieldNumbers(f File, r *report.Report) {
	// TODO: Evaluate reserved ranges.

	for ty := range seq.Values(f.AllTypes()) {
		tags := make(map[int32]Member, ty.Members().Len())
		for member := range seq.Values(ty.Members()) {
			n, ok := evaluateMemberNumber(member, r)
			member.raw.number = n
			if !ok {
				continue
			}

			// TODO: Need to check allow_alias here.
			if first, ok := mapsx.Add(tags, n, member); !ok {
				what := taxa.FieldNumber
				if ty.IsEnum() {
					what = taxa.EnumValue
				}
				r.Errorf("%ss must be unique", what).Apply(
					report.Snippetf(member.AST().Value(), "used again here"),
					report.Snippetf(first.AST().Value(), "first used here"),
				)
			}
		}
		ty.raw.haveNumbers = true

		// TODO: compare with extension/reserved ranges.
	}
}

// evaluateExtensionNumbers evaluates all extension field numbers: that is,
// the numbers on extension ranges and extension fields.
func evaluateExtensionNumbers(f File, r *report.Report) {
	// TODO: Evaluate extension ranges.

	for extn := range seq.Values(f.AllExtensions()) {
		n, _ := evaluateMemberNumber(extn, r)
		extn.raw.number = n

		// TODO: compare with extension ranges.
	}
}

func evaluateMemberNumber(member Member, r *report.Report) (int32, bool) {
	if member.AST().Value().IsZero() {
		return 0, false // Diagnosed for us elsewhere.
	}

	e := &evaluator{
		Context: member.Context(),
		Report:  r,
		scope:   member.FullName().Parent(),
	}

	var memberNumber memberNumber
	switch {
	case member.IsEnumValue():
		memberNumber = enumNumber
	case member.IsMessageField(), member.IsExtension():
		memberNumber = fieldNumber

		// TODO: MessageSet.
	}

	// Don't bother allocating a whole Value for this.
	v, ok := e.evalBits(evalArgs{
		expr:         member.AST().Value(),
		memberNumber: memberNumber,
	})

	return int32(v), ok
}

// evaluator is the context needed to evaluate an expression.
type evaluator struct {
	*Context
	*report.Report
	scope FullName
}

//nolint:govet // vet complains about 8 wasted padding bytes.
type evalArgs struct {
	expr ast.ExprAny // The expression to evaluate.

	// The location to write the parsed expression into. If zero, a new value
	// will be allocated.
	target     Value
	field      Member
	optionPath ast.Path

	rawField      ref[rawMember]
	isConcreteAny bool

	// A span for whatever caused the above field to be selected.
	annotation report.Spanner

	textFormat   bool         // Whether we're inside of a message literal.
	allowMax     bool         // Whether the max keyword is to be honored.
	memberNumber memberNumber // Specifies which member number type we're resolving.
}

// memberNumber is used to tag evalArgs with one of the special types associated
// with a member number.
type memberNumber byte

const (
	enumNumber       memberNumber = iota + 1 // int32
	fieldNumber                              // uint29
	messageSetNumber                         // uint31
)

// Type returns the type that evaluation is targeting.
func (ea evalArgs) Type() Type {
	if ea.isConcreteAny {
		msg := ea.target.Elements().At(0).AsMessage()
		return msg.Type()
	}
	return ea.field.Element()
}

// mismatch constructs a type mismatch error.
func (ea evalArgs) mismatch(got any) errTypeCheck {
	var want any
	if ty := ea.Type(); !ty.IsZero() {
		want = ty
	} else {
		switch ea.memberNumber {
		case enumNumber:
			want = PredeclaredType(predeclared.Int32)
		case fieldNumber:
			want = taxa.FieldNumber
		case messageSetNumber:
			want = taxa.MessageSetNumber
		}
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
// Returns a zero value if evaluation produced "no value", such as due to
// a type checking failure or an empty array.
func (e *evaluator) eval(args evalArgs) Value {
	defer e.AnnotateICE(report.Snippetf(args.expr, "while evaluating this"))

	if arr := args.expr.AsArray(); !arr.IsZero() && arr.Elements().Len() == 0 {
		// We don't create a value for empty arrays, but we still need to
		// perform type checking.
		if args.field.Presence() != presence.Repeated {
			e.Error(args.mismatch(taxa.Array))
		}

		return Value{}
	}

	first := args.target.IsZero()
	if first && args.rawField.ptr.Nil() {
		args.rawField = compressMember(e.Context, args.field)
	} else if !first {
		args.rawField = args.target.raw.field
	}

	switch args.expr.Kind() {
	case ast.ExprKindArray:
		if args.field.Presence() != presence.Repeated {
			e.Error(args.mismatch(taxa.Array))
		}

		expr := args.expr.AsArray()
		for elem := range seq.Values(expr.Elements()) {
			copied := args // Copy.
			copied.expr = elem

			v := e.eval(copied)
			if args.target.IsZero() {
				// Make sure to pick up a freshly allocated value, if this
				// was the first iteration.
				args.target = v
			}
		}
	case ast.ExprKindDict:
		args.target = e.evalMessage(args, args.expr.AsDict())
	default:
		bits, ok := e.evalBits(args)
		if !ok {
			return Value{}
		}

		if first {
			args.target = newZeroScalar(e.Context, args.rawField)
			args.target.raw.bits = bits
		} else {
			appendRaw(args.target, bits)
		}
	}

	if !args.target.IsZero() {
		if args.target.AST().IsZero() {
			args.target.raw.expr = args.expr
		}
		if args.target.OptionPath().IsZero() {
			args.target.raw.optionPath = args.optionPath
		}
	}

	return args.target
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

			// Special cases for "signed identifiers".
			if inner.Kind() == ast.ExprKindPath &&
				args.Type().Predeclared().IsFloat() {
				v, ok := e.evalPath(args, inner.AsPath().Path)
				v |= 0x8000_0000_0000_0000 // Set the floating-point sign bit.
				return v, ok
			}

			// All other expressions cannot have a leading -.
			err := args.mismatch(taxa.Classify(inner))
			err.want = taxa.Number
			return 0, false
		default:
			panic("unreachable")
		}

	case ast.ExprKindArray:
		e.Error(args.mismatch(taxa.Array))
	case ast.ExprKindDict:
		e.Error(args.mismatch(taxa.Dict))
	case ast.ExprKindRange:
		if args.memberNumber == 0 {
			break // Legalized in the parser.
		}
		e.Error(args.mismatch(taxa.Range))

	case ast.ExprKindField:
		break // Legalized in the parser.

	default:
		panic("unexpected ast.ExprKind")
	}

	return 0, false
}

// evalKey evaluates a key in a message literal.
func (e *evaluator) evalKey(args evalArgs, expr ast.ExprAny) Member {
	// There are a number of potentially incorrect ways of specifying
	// a field here, which we want to diagnose.
	//
	// 1. A field might be named by number. In this case, we rely on
	// 	  field numbers having already been evaluated and we try to
	//    the field up by number. This seems hard to make work for
	//    extensions.
	//
	// 2. A partially qualified path to a field or extension. We try
	//    to do symbol resolution in the current scope.
	//
	// 3. The above in [] when it shouldn't be.
	//
	// 4. The above as a string literal.
	//
	// Everything else is unrecoverable.
	ty := args.Type()
	cannotResolveKey := func() {
		e.Errorf("cannot resolve %s name for `%s`", taxa.Field, ty.FullName()).Apply(
			report.Snippetf(expr, "field referenced here"),
			report.Snippetf(args.annotation, "expected `%s` field due to this", ty.Name()),
		)
	}

	var member Member
	var path string
	var hasBrackets, isPath, isNumber, isString bool
	key := expr
again:
	switch key.Kind() {
	case ast.ExprKindPath:
		path = key.AsPath().Canonicalized()
		if strings.Contains(path, "/") {
			if ty.IsAny() {
				// Any type names for an actual any are diagnosed elsewhere.
				return Member{}
			}

			// This appears to be an Any type name.
			e.Errorf("unexpected %s", taxa.TypeURL).Apply(
				report.Snippet(expr),
				report.Snippetf(args.annotation, "expected this to be `google.protobuf.Any`"),
				report.Notef("%s may only appear in a `google.protobuf.Any`-typed %s", taxa.Dict),
			)
			return Member{}
		}

		isPath = true

	case ast.ExprKindArray:
		array := key.AsArray()
		if hasBrackets || array.Elements().Len() != 1 {
			// Diagnosed in the parser.
			return Member{}
		}
		hasBrackets = true

		key = array.Elements().At(0)
		goto again

	case ast.ExprKindLiteral:
		lit := key.AsLiteral()
		if lit.Kind() == token.Number {
			n, ok := lit.AsInt()
			if ok && n < math.MaxInt32 {
				member = ty.MemberByNumber(int32(n))
				if !member.IsZero() {
					isNumber = true
					goto validate
				}
			}
			cannotResolveKey()
			return Member{}
		}

		path, _ = lit.AsString()
		isString = true

	default:
		cannotResolveKey()
		return Member{}
	}

	// Try checking if this is just a member of the message directly.
	if !hasBrackets {
		member = ty.MemberByName(path)
	}
	if member.IsZero() {
		if isPath && !hasBrackets && strings.HasPrefix(path, "(") {
			// This was already diagnosed in legalize_option.go.
			//
			// TODO: we should try to do better here and use the contents of
			// the () as the symbol lookup target.
			return Member{}
		}

		// Otherwise kick off full symbol resolution.
		sym := symbolRef{
			Context: e.Context,
			Report:  nil, // Suppress diagnostics.

			scope: e.scope,
			name:  FullName(path),
			span:  expr,
		}.resolve()

		if sym.IsZero() {
			// This catches cases where a user names a non-extension field with
			// [], but name lookup does not find it.
			if member = ty.MemberByName(path); !member.IsZero() {
				goto validate
			}

			cannotResolveKey()
			return Member{}
		} else if !sym.Kind().IsMessageField() {
			e.Error(errTypeCheck{
				want:       fmt.Sprintf("`%s` field", ty.FullName()),
				got:        sym,
				expr:       expr,
				annotation: args.annotation,
			})
			return Member{}
		}
		// NOTE: Absolute paths in this position are diagnosed in the parser.

		member = sym.AsMember()
	}

validate:
	// Validate that the member is actually of the correct type.
	if member.Container() != ty {
		e.Error(errTypeCheck{
			want:       fmt.Sprintf("`%s` field", ty.FullName()),
			got:        fmt.Sprintf("`%s` field", member.Container().FullName()),
			expr:       expr,
			annotation: args.annotation,
		})
		return Member{}
	}

	// Validate that the key was spelled correctly: if it is a field,
	// it is a single identifier with the name of that field, and has no
	// brackets; if it is an extension, it is the FQN and it has brackets.
	wrongPath := member.IsMessageField() && path != member.Name()
	misspelled := !isPath || hasBrackets != member.IsExtension() || wrongPath

	if misspelled {
		replace := member.Name()
		if member.IsExtension() {
			replace = fmt.Sprintf("[%s]", member.FullName())
		}

		d := e.Errorf("%s `%s` referenced incorrectly", member.noun(), member.FullName()).Apply(
			report.Snippetf(expr, "referenced here"),
			report.SuggestEdits(expr, fmt.Sprintf("reference it as `%s`", replace), report.Edit{
				Start: 0, End: expr.Span().Len(),
				Replace: replace,
			}),
		)

		if hasBrackets && !member.IsExtension() {
			d.Apply(report.Notef("%s must only be used when referencing extensions or concrete `Any` types", taxa.Brackets))
		}

		if !hasBrackets && member.IsExtension() {
			d.Apply(report.Notef("extension names must be surrounded by %s", taxa.Brackets))
		}

		if wrongPath {
			d.Apply(report.Notef("field names must be a single identifier"))
		}

		if !hasBrackets {
			if isNumber {
				d.Apply(report.Notef("due to a parser quirk, `.protoc` rejects numbers here, even though textproto does not"))
			}
			if isString {
				d.Apply(report.Notef("due to a parser quirk, `.protoc` rejects quoted strings here, even though textproto does not"))
			}
		}
	}

	return member
}

func (e *evaluator) evalMessage(args evalArgs, expr ast.ExprDict) Value {
	if !args.Type().IsMessage() {
		e.Error(args.mismatch(taxa.Dict))
		return Value{}
	}

	var message MessageValue
	switch {
	case args.isConcreteAny:
		message = args.target.Elements().At(0).AsMessage()
	case args.target.IsZero():
		message = newMessage(e.Context, args.rawField)
		args.target = message.AsValue()
	default:
		message = appendMessage(args.target)
	}

	if args.Type().IsAny() {
		// Check if this is a valid concrete Any. There should be exactly
		// one [host/path] key in the dictionary. If there is *at least one*,
		// we choose the first one, and diagnose all other keys as invalid.

		var url string
		var urlExpr ast.ExprField
		var key ast.ExprAny
	urlSearch:
		for expr := range seq.Values(expr.Elements()) {
			key = expr.Key()
			var hasBrackets bool
		again:
			switch key.Kind() {
			case ast.ExprKindPath:
				path := key.AsPath().Canonicalized()
				if strings.Contains(path, "/") {
					url = path
					urlExpr = expr
					break urlSearch
				}

			case ast.ExprKindArray:
				array := key.AsArray()
				if hasBrackets || array.Elements().Len() != 1 {
					// Diagnosed in the parser.
					continue
				}
				hasBrackets = true

				key = array.Elements().At(0)
				goto again
			}
		}

		if url != "" {
			// First, scold all the other fields.
			first := true
			for expr := range seq.Values(expr.Elements()) {
				if expr == urlExpr {
					continue
				}

				d := e.Errorf("unexpected field in `Any` expression").Apply(
					report.Snippet(expr.Key()),
					report.Notef("the %s must be the only field", taxa.TypeURL),
				)
				if first {
					first = false
					d.Apply(report.Snippetf(urlExpr.Key(), "expected this to be the only field"))
				}
			}

			splitURL := func(path ast.Path) (before, after ast.Path) {
				// Figure out what part of the key expression actually contains
				// the hostname. Look for the last component whose separator is a /.
				pc, _ := iterx.Last(iterx.Filter(path.Components, func(pc ast.PathComponent) bool {
					return pc.Separator().Text() == "/"
				}))
				hostSpan := path.Span()
				hostSpan.End = pc.Span().Start

				before, after = pc.SplitBefore()
				return before, after.ToRelative()
			}

			// Next, resolve the type name. protoc only allows one /, but
			// we allow multiple and simply diagnose the hostname.
			host, path, _ := stringsx.CutLast(url, "/")
			hostPath, typePath := splitURL(key.AsPath().Path)

			switch host {
			case "type.googleapis.com", "type.googleprod.com":
				break
			case "":
				e.Errorf("missing hostname in %s", taxa.TypeURL).Apply(
					report.Snippet(urlExpr.Key()),
					report.Notef("`protoc` requires that the hostname be one of "+
						"`type.googleapis.com` or `type.googleprod.com`. "+
						"This is a quirk of textformat; the compiler does not "+
						"actually make any network requests."),
				)
			default:
				e.Errorf("unsupported hostname `%s` in %s", host, taxa.TypeURL).Apply(
					report.Snippet(hostPath),
					report.Notef("`protoc` will reject any hostname that are not "+
						"`type.googleapis.com` or `type.googleprod.com`. "+
						"This is a quirk of textformat; the compiler does not "+
						"actually make any network requests."),
				)
			}

			// Now try to resolve a concrete type. We do it exactly like
			// we would for a field type, but *not* including scalar types.
			ty := symbolRef{
				Context: e.Context,
				Report:  e.Report,

				scope: e.scope,
				name:  FullName(path),
				span:  typePath,

				skipIfNot: SymbolKind.IsType,
				accept:    func(sk SymbolKind) bool { return sk == SymbolKindMessage },
				want:      taxa.MessageType,

				allowScalars:  false,
				suggestImport: true,
			}.resolve().AsType()

			if ty.IsZero() {
				// Diagnosed for us by resolve().
				return Value{}
			}

			// Check that the URL contains the full name of the type.
			if path != string(ty.FullName()) {
				_, typePath := splitURL(key.AsPath().Path)
				e.Errorf("partly-qualified name in %s", taxa.TypeURL).Apply(
					report.Snippetf(typePath, "type referenced here"),
					report.SuggestEdits(typePath, fmt.Sprintf("replace with %s", taxa.FullyQualifiedName), report.Edit{
						Start: 0, End: typePath.Span().Len(),
						Replace: string(ty.FullName()),
					}),
					report.Notef("%s require %ss", taxa.TypeURL, taxa.FullyQualifiedName),
				)
			}

			// Apply the Any type and recurse.
			abstract := args.target
			args.target = newConcrete(message, ty, url).AsValue()
			args.expr = urlExpr.Value()
			args.annotation = urlExpr.Key()
			args.isConcreteAny = true
			_ = e.eval(args)
			return abstract // Want to return the outer any here!
		}
	}

	for expr := range seq.Values(expr.Elements()) {
		field := e.evalKey(args, expr.Key())
		if field.IsZero() {
			continue
		}

		copied := args
		copied.textFormat = true
		copied.isConcreteAny = false
		copied.expr = expr.Value()
		copied.annotation = field.AST().Type()
		copied.field = field
		copied.rawField = ref[rawMember]{}

		slot := message.insert(field)
		if slot.Nil() {
			copied.target = Value{}
		} else {
			value := wrapValue(e.Context, *slot)

			switch {
			case field.Presence() == presence.Repeated:
				copied.target = wrapValue(e.Context, *slot)

			case value.Field() != field:
				// A different member of a oneof was set.
				e.Error(errSetMultipleTimes{
					member: field.Oneof(),
					first:  value.key(),
					second: expr.Key(),
				})
				copied.target = Value{}

			case field.Element().IsMessage():
				copied.target = wrapValue(e.Context, *slot)

			default:
				e.Error(errSetMultipleTimes{
					member: field,
					first:  value.key(),
					second: expr.Key(),
				})
				copied.target = Value{}
			}
		}

		v := e.eval(copied)
		if slot.Nil() && !v.IsZero() {
			v.raw.expr = expr.AsAny()

			// Make sure to pick up a freshly allocated value, if this
			// was the first iteration.
			*slot = e.arenas.values.Compress(v.raw)
		}
	}

	return message.AsValue()
}

// evalLiteral evaluates a literal expression.
func (e *evaluator) evalLiteral(args evalArgs, expr ast.ExprLiteral, neg bool) (rawValueBits, bool) {
	scalar := args.Type().Predeclared()
	if args.Type().IsEnum() {
		scalar = predeclared.Int32
	}

	switch expr.Kind() {
	case token.Number:
		// Handle floats first, since all number formats can be used as floats.
		if scalar.IsFloat() {
			if n, ok := expr.AsFloat(); ok {
				// If the number contains no decimal point, check that it has no
				// 0x prefix. Hex literals are not permitted for float-typed
				// values, but we don't know that until here, much later than
				// all the other base checks in the compiler.
				text := expr.Text()
				if !taxa.IsFloatText(text) && (strings.HasPrefix(text, "0x") || strings.HasPrefix(text, "0X")) {
					e.Errorf("unsupported base for %s", taxa.Float).Apply(
						report.SuggestEdits(expr, "use a decimal literal instead", report.Edit{
							Start: 0, End: len(text),
							Replace: strconv.FormatFloat(n, 'g', 40, 64),
						}),
						report.Notef("Protobuf does not support hexadecimal %s", taxa.Float),
					)
				}

				if neg {
					n = -n
				}
				if scalar == predeclared.Float32 {
					// This will, among other things, snap n to Infinity or zero
					// if it is in-range for float64 but not float32.
					n = float64(float32(n))
				}

				// Emit a diagnostic if the value is snapped to infinity.
				// TODO: Should we emit a diagnostic when rounding produces
				// the value 0.0 but expr.Text() contains non-zero digits?
				if math.IsInf(n, 0) {
					d := e.Warnf("%s rounds to infinity", taxa.Float).Apply(
						report.Snippetf(expr, "this value is beyond the dynamic range of `%s`", scalar),
						report.SuggestEdits(expr, "replace with `inf`", report.Edit{
							Start: 0, End: len(text),
							Replace: "inf", // The sign is not part of the expression.
						}),
					)

					// If possible, show the power-of-10 exponent of the value.
					f := new(big.Float)
					if _, _, err := f.Parse(expr.Text(), 0); err == nil {
						maxExp := 308
						if scalar == predeclared.Float32 {
							maxExp = 38
						}

						exp2 := f.MantExp(nil)                      // ~ log2 f
						exp10 := int(float64(exp2) / math.Log2(10)) // log10 f = log2 f / log2 10
						d.Apply(report.Notef(
							"this value is of order 1e%d; `%s` can only represent around 1e%d",
							exp10, scalar, maxExp))
					}
				}

				// 32-bit floats are stored as 64-bit floats; this conversion is
				// lossless.
				return rawValueBits(math.Float64bits(n)), true
			}
		}

		if n, ok := expr.AsInt(); ok {
			switch args.memberNumber {
			case enumNumber:
				return e.checkIntBounds(args, true, enumNumberBits, neg, n)
			case fieldNumber:
				return e.checkIntBounds(args, false, fieldNumberBits, neg, n)
			case messageSetNumber:
				return e.checkIntBounds(args, false, messageSetNumberBits, neg, n)
			}

			if !scalar.IsNumber() {
				e.Error(args.mismatch(taxa.Int))
				return 0, false
			}

			return e.checkIntBounds(args, scalar.IsSigned(), scalar.Bits(), neg, n)
		}

		if n := expr.AsBigInt(); n != nil {
			switch args.memberNumber {
			case enumNumber:
				return e.checkIntBounds(args, true, enumNumberBits, neg, n)
			case fieldNumber:
				return e.checkIntBounds(args, false, fieldNumberBits, neg, n)
			case messageSetNumber:
				return e.checkIntBounds(args, false, messageSetNumberBits, neg, n)
			}

			if !scalar.IsNumber() {
				e.Error(args.mismatch(taxa.Int))
				return 0, false
			}
			return e.checkIntBounds(args, scalar.IsSigned(), scalar.Bits(), neg, n)
		}

		if _, ok := expr.AsFloat(); ok {
			e.Error(args.mismatch(taxa.Float))
			return 0, false
		}

	case token.String:
		if scalar != predeclared.String && scalar != predeclared.Bytes {
			e.Error(args.mismatch(PredeclaredType(predeclared.String)))
			return 0, false
		}

		data, _ := expr.AsString()
		return newScalarBits(e.Context, data), true
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
	err := func() {
		e.Error(errLiteralRange{
			errTypeCheck: args.mismatch(nil),
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
	default:
		panic("unreachable")
	}

	if signed {
		hi := (int64(1) << (bits - 1)) - 1
		lo := ^hi // Ensure that lo is sign-extended to 64 bits.

		if neg {
			v = -v
		}
		v := int64(v)

		// If bits == 64, we may be in a situation where - overflows. For
		// example, if the input value is uint64(math.MaxInt32+1), then -
		// is the identity (this is the only value other than 0 that its
		// own two's complement).
		//
		// To detect this, we have to check that the sign of v is consistent
		// with neg. If -v > 0 or v < 0, overflow has occurred.
		if (neg && tooLarge) || (neg && v > 0) || v < lo {
			err()
			return rawValueBits(lo), false
		}
		if (!neg && tooLarge) || (!neg && v < 0) || v > hi {
			err()
			return rawValueBits(hi), false
		}
	} else {
		if neg {
			err()
			return 0, false
		}

		hi := (uint64(1) << bits) - 1
		if tooLarge || v > hi {
			err()
			return rawValueBits(hi), false
		}
	}

	if bits == fieldNumberBits {
		if v == 0 {
			err()
			return 0, false
		}

		// Check that this is not one of the special reserved numbers.
		if v >= firstReserved && v <= lastReserved {
			err()
			return rawValueBits(v), false
		}
	}

	return rawValueBits(v), true
}

// evalPath evaluates a path expression.
func (e *evaluator) evalPath(args evalArgs, expr ast.Path) (rawValueBits, bool) {
	if ty := args.Type(); ty.IsEnum() {
		// We can just plumb the text of the expression directly here, since
		// if it's anything that isn't an identifier, this lookup will fail.
		value := ty.MemberByName(expr.Span().Text())

		// TODO: This depends on field numbers being resolved before options,
		// but some options need to be resolved first.
		if !value.IsZero() {
			return newScalarBits(e.Context, value.Number()), true
		}

		// Allow fall-through, which proceeds to eventually hit full symbol
		// resolution at the bottom.
	}

	scalar := args.Type().Predeclared()

	// If we see a name that matches one of the predeclared names, resolve
	// to it, just like it would for type lookup.
	//
	// TODO: When implementing message literals, we need to make sure to accept
	// all of the non-standard forms that are allowed only inside of them.
	switch name := expr.AsPredeclared(); name {
	case predeclared.Max:
		ok := args.allowMax
		if ok {
			switch args.memberNumber {
			case enumNumber:
				return enumNumberMax, ok
			case fieldNumber:
				return fieldNumberMax, ok
			case messageSetNumber:
				return messageSetNumberMax, ok
			}
		} else {
			e.Errorf("%s outside of %s", taxa.PredeclaredMax, taxa.Range).Apply(
				report.Snippet(expr),
				report.Notef(
					"the special %s expression is only allowed in a %s",
					taxa.PredeclaredMax, taxa.Range),
			)
			return 0, false
		}

		if !scalar.IsNumber() {
			e.Error(args.mismatch(taxa.PredeclaredMax))
			return 0, false
		}

		if scalar.IsFloat() {
			return newScalarBits(e.Context, math.Inf(0)), ok
		}

		n := uint64(1) << scalar.Bits()
		if scalar.IsSigned() {
			n >>= 1
		}
		n--
		return rawValueBits(n), ok

	case predeclared.True, predeclared.False:
		if scalar != predeclared.Bool {
			e.Error(args.mismatch(PredeclaredType(predeclared.Bool)))
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
			e.Error(args.mismatch(taxa.Float))
			return 0, false
		}

		switch name {
		case predeclared.Inf:
			return newScalarBits(e.Context, math.Inf(0)), true
		case predeclared.NAN:
			return newScalarBits(e.Context, math.Float64frombits(nanBits)), true
		}
	}

	// Match the "non standard" symbols for true, false, inf, and nan. Make
	// sure to warn when users do it in text mode, and error when outside of
	// it.
	text := expr.Span().Text()
	switch scalar {
	case predeclared.Bool:
		if slicesx.Among(text, "False", "f", "True", "t") {
			value := slicesx.Among(text, "True", "t")
			var d *report.Diagnostic
			if args.textFormat {
				d = e.Warnf("non-canonical `bool` literal")
			} else {
				d = e.Errorf("non-canonical `bool` literal outside of %s", taxa.Dict)
			}

			d.Apply(
				report.Snippet(expr),
				report.SuggestEdits(expr, fmt.Sprintf("replace with `%v`", value), report.Edit{
					Start: 0, End: len(text),
					Replace: strconv.FormatBool(value),
				}),
				report.Notef("within %ss only, `%s` is permitted as a `bool`, but should be avoided", taxa.Dict, text),
			)

			if value {
				return 1, args.textFormat
			}
			return 0, args.textFormat
		}

	case predeclared.Float32, predeclared.Float64:
		var value float64
		var canonical string

		switch {
		case strings.EqualFold(text, "inf"), strings.EqualFold(text, "infinity"):
			canonical = "inf"
			value = math.Inf(0)

		case strings.EqualFold(text, "nan"):
			canonical = "nan"
			value = math.Float64frombits(nanBits)
		}

		var d *report.Diagnostic
		if args.textFormat {
			d = e.Warnf("non-canonical %s", taxa.Float)
		} else {
			d = e.Errorf("non-canonical %s outside of %s", taxa.Float, taxa.Dict)
		}

		d.Apply(
			report.Snippet(expr),
			report.SuggestEdits(expr, fmt.Sprintf("replace with `%v`", canonical), report.Edit{
				Start: 0, End: len(text),
				Replace: canonical,
			}),
			report.Notef("within %ss only, some %ss are case-insensitive", taxa.Dict, taxa.Float),
		)

		return newScalarBits(e.Context, value), args.textFormat
	}

	// Perform symbol lookup in the current scope. This isn't what protoc
	// does, but it allows us to produce better diagnostics.
	sym := symbolRef{
		Context: e.Context,
		Report:  e.Report,

		scope: e.scope,
		name:  FullName(expr.Canonicalized()),
		span:  expr,

		allowScalars: true,
	}.resolve()

	if ty := sym.AsType(); !ty.IsZero() {
		e.Error(args.mismatch(fmt.Sprintf("type reference `%s`", ty.FullName())))
	} else if ev := sym.AsMember(); ev.IsEnumValue() {
		if ev.Container() == args.Type() {
			e.Errorf("qualified enum value reference").Apply(
				report.Snippet(expr),
				report.SuggestEdits(expr, "replace it with the value's name", report.Edit{
					Start: 0, End: expr.Span().Len(),
					Replace: ev.Name(),
				}),
				report.Notef("Protobuf requires single identifiers when referencing to the names of enum values"),
			)
			return newScalarBits(e.Context, ev.Number()), false
		}

		e.Error(args.mismatch(ev.Container()))
	} else if !sym.IsZero() {
		e.Error(args.mismatch(sym))
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
				itoa(uint64(firstReserved)),
				itoa(uint64(lastReserved))),
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
