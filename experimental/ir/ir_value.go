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
	"cmp"
	"fmt"
	"iter"
	"math"
	"slices"

	"google.golang.org/protobuf/encoding/protowire"

	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/ast/predeclared"
	"github.com/bufbuild/protocompile/experimental/id"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/internal/arena"
	"github.com/bufbuild/protocompile/internal/ext/mapsx"
	"github.com/bufbuild/protocompile/internal/ext/slicesx"
	"github.com/bufbuild/protocompile/internal/intern"
)

// Value is an evaluated expression, corresponding to an option in a Protobuf
// file.
type Value id.Node[Value, *File, *rawValue]

// rawValue is a [rawValueBits] with field information attached to it.
type rawValue struct {
	// Expressions that contributes to this value.
	//
	// The representation of this field is quite complicated, to deal with
	// potentially complicated source ASTs. The worst case is as follows.
	// Consider:
	//
	//   option foo = { a: [1, 2, 3], a: [4, 5] }; // (*)
	//
	// Here, two ast.FieldExprs contribute to the value of a, but there are
	// five subexpressions for the elements of a. We would like to be able to
	// report both those FieldExprs, *and* report an expression for each value
	// therein.
	//
	// However, there is another potentially subtle case we *do not* have to
	// deal with (for a singular message field a):
	//
	//   option foo = { a { b: 1 }, a { c: 2 } };
	//
	// This is an error, because foo.a has already been set when we process
	// the second value. If a is repeated, each of these produces a separate
	// element.
	//
	// Because case (*) is rare, we adopt a compression strategy here. exprs
	// refers to all contributing expressions for the value. If any array
	// expressions occurred, elemIndices will be non-nil, and will be a prefix
	// sum of the number of values that each expr in exprs contributes. This is
	// binary-searched by Element.AST to find the AST nodes of each element.
	//
	// Specifically, elemIndices[i] will be the number of elements that every
	// expression, up to an including exprs[i], contributes. This layout is
	// chosen because it significantly simplifies construction and searching of
	// this slice.
	exprs       []id.Dyn[ast.ExprAny, ast.ExprKind]
	elemIndices []uint32

	// The AST nodes for the path of the option (compact or otherwise) that
	// specify this value. This is intended for diagnostics.
	//
	// For example, the node
	//
	//  option a.b.c = 9;
	//
	// results in a field a: {b: {c: 9}}, which is four rawValues deep.
	// Each of these will have the same optionPath, for a.b.c.
	//
	// There will be one such value for each contributing expression, to deal
	// with the repeated field case
	//
	//   option f = 1; option f = 2;
	optionPaths []ast.PathID

	// The field that this value sets. This is where type information comes
	// from.
	//
	// NOTE: we steal the high bit of the pointer to indicate whether or not
	// bits refers to a slice. If the pointer part is negative, bits is a
	// repeated field with multiple elements.
	field Ref[Member]
	bits  rawValueBits

	// The message which contains this value.
	container id.ID[MessageValue]
}

// rawValueBits is used to represent the actual value for all types, according to
// the following encoding:
//  1. All numeric types, including bool and enums. This holds the bits.
//  2. String and bytes. This holds an intern.ID.
//  3. Messages. This holds an id.ID[Message].
//  4. Repeated fields with two or more entries. This holds an
//     arena.Pointer[[]rawValueBits], where each value is interpreted as a
//     non-array with this value's type.
//     This exploits the fact that arrays cannot contain other arrays.
//     Note that within the IR, map fields do not exist, and are represented as
//     the repeated message fields that they will ultimately become.
type rawValueBits uint64

// OptionSpan returns a representative span for the option that set this value.
//
// The Spanner will be an [ast.ExprField], if it is set in an [ast.ExprDict].
func (v Value) OptionSpan() source.Spanner {
	if v.IsZero() || len(v.Raw().exprs) == 0 {
		return nil
	}

	c := v.Context().AST()
	expr := id.WrapDyn(c, v.Raw().exprs[0])
	if field := expr.AsField(); !field.IsZero() {
		return field
	}
	return source.Join(ast.ExprPath{Path: v.Raw().optionPaths[0].In(c)}, expr)
}

// OptionSpans returns an indexer over spans for the option that set this value.
//
// The Spanner will be an [ast.ExprField], if it is set in an [ast.ExprDict].
func (v Value) OptionSpans() seq.Indexer[source.Spanner] {
	var slice []id.Dyn[ast.ExprAny, ast.ExprKind]
	if !v.IsZero() {
		slice = v.Raw().exprs
	}

	return seq.NewFixedSlice(slice, func(_ int, p id.Dyn[ast.ExprAny, ast.ExprKind]) source.Spanner {
		c := v.Context().AST()
		expr := id.WrapDyn(c, p)
		if field := expr.AsField(); !field.IsZero() {
			return field
		}
		return source.Join(ast.ExprPath{Path: v.Raw().optionPaths[0].In(c)}, expr)
	})
}

// ValueAST returns a representative expression that evaluated to this value.
//
// For complicated options (such as repeated fields), there may be more than
// one contributing expression; this will just return *one* of them.
func (v Value) ValueAST() ast.ExprAny {
	if v.IsZero() || len(v.Raw().exprs) == 0 {
		return ast.ExprAny{}
	}

	c := v.Context().AST()
	expr := id.WrapDyn(c, v.Raw().exprs[0])
	if field := expr.AsField(); !field.IsZero() {
		return field.Value()
	}

	return expr
}

// ValueASTs returns all expressions that contributed to evaluating this value.
//
// There may be more than one such expression, for repeated fields set more
// than once.
func (v Value) ValueASTs() seq.Indexer[ast.ExprAny] {
	var slice []id.Dyn[ast.ExprAny, ast.ExprKind]
	if !v.IsZero() {
		slice = v.Raw().exprs
	}

	return seq.NewFixedSlice(slice, func(_ int, p id.Dyn[ast.ExprAny, ast.ExprKind]) ast.ExprAny {
		c := v.Context().AST()
		expr := id.WrapDyn(c, p)
		if field := expr.AsField(); !field.IsZero() {
			return field.Value()
		}
		return expr
	})
}

// KeyAST returns a representative AST node for the message key that evaluated
// from this value.
func (v Value) KeyAST() ast.ExprAny {
	if v.IsZero() || len(v.Raw().exprs) == 0 {
		return ast.ExprAny{}
	}

	c := v.Context().AST()
	expr := id.WrapDyn(c, v.Raw().exprs[0])
	if field := expr.AsField(); !field.IsZero() {
		return field.Key()
	}
	return ast.ExprPath{Path: v.Raw().optionPaths[0].In(c)}.AsAny()
}

// KeyASTs returns the AST nodes for each key associated with a value in
// [Value.ValueASTs].
//
// This will either be the key value from an [ast.FieldExpr] (which need not be
// an [ast.PathExpr], in the case of an extension) or the [ast.PathExpr]
// associated with the left-hand-side of an option setting.
func (v Value) KeyASTs() seq.Indexer[ast.ExprAny] {
	var slice []id.Dyn[ast.ExprAny, ast.ExprKind]
	if !v.IsZero() {
		slice = v.Raw().exprs
	}

	return seq.NewFixedSlice(slice, func(n int, p id.Dyn[ast.ExprAny, ast.ExprKind]) ast.ExprAny {
		c := v.Context().AST()
		expr := id.WrapDyn(c, p)
		if field := expr.AsField(); !field.IsZero() {
			return field.Key()
		}
		return ast.ExprPath{Path: v.Raw().optionPaths[n].In(c)}.AsAny()
	})
}

// OptionPaths returns the AST nodes for option paths that set this node.
//
// There will be one path per value returned from [Value.ValueASTs]. Generally,
// you'll want to use [Value.KeyASTs] instead.
func (v Value) OptionPaths() seq.Indexer[ast.Path] {
	var slice []ast.PathID
	if !v.IsZero() {
		slice = v.Raw().optionPaths
	}

	return seq.NewFixedSlice(slice, func(_ int, e ast.PathID) ast.Path {
		c := v.Context().AST()
		return e.In(c)
	})
}

// Field returns the field this value sets, which includes the value's type
// information.
//
// NOTE: [Member.Element] returns google.protobuf.Any, the concrete type of the
// values in [Value.Elements] may be distinct from it.
func (v Value) Field() Member {
	if v.IsZero() {
		return Member{}
	}

	field := v.Raw().field
	if int32(field.id) < 0 {
		field.id ^= -1
	}
	return GetRef(v.Context(), field)
}

// Container returns the message value which contains this value, assuming it
// is not a top-level value.
//
// This function is analogous to [Member.Container], which returns the type
// that contains a member; in particular, for extensions, it returns an
// extendee.
func (v Value) Container() MessageValue {
	if v.IsZero() {
		return MessageValue{}
	}
	return id.Wrap(v.Context(), v.Raw().container)
}

// Elements returns an indexer over the elements within this value.
//
// If the value is not an array, it contains the singular element within;
// otherwise, it returns the elements of the array.
//
// The indexer will be nonempty except for the zero Value. That is to say, unset
// fields of [MessageValue]s are not represented as a distinct "empty" Value.
func (v Value) Elements() seq.Indexer[Element] {
	return seq.NewFixedSlice(v.getElements(), func(n int, bits rawValueBits) Element {
		return Element{
			withContext: id.WrapContext(v.Context()),
			index:       n,
			value:       v,
			bits:        bits,
		}
	})
}

// Outlined to promote inlining of Elements().
func (v Value) getElements() []rawValueBits {
	var slice []rawValueBits
	switch {
	case v.IsZero():
		break
	case int32(v.Raw().field.id) < 0:
		slice = *v.slice()
	default:
		slice = slicesx.One(&v.Raw().bits)
	}
	return slice
}

// IsZeroValue is a shortcut for [Element.IsZeroValue].
func (v Value) IsZeroValue() bool {
	if v.IsZero() {
		return false
	}
	return v.Elements().At(0).IsZeroValue()
}

// AsBool is a shortcut for [Element.AsBool], if this value is singular.
func (v Value) AsBool() (value, ok bool) {
	if v.IsZero() || v.Field().IsRepeated() {
		return false, false
	}
	return v.Elements().At(0).AsBool()
}

// AsUInt is a shortcut for [Element.AsUInt], if this value is singular.
func (v Value) AsUInt() (uint64, bool) {
	if v.IsZero() || v.Field().IsRepeated() {
		return 0, false
	}
	return v.Elements().At(0).AsUInt()
}

// AsInt is a shortcut for [Element.AsInt], if this value is singular.
func (v Value) AsInt() (int64, bool) {
	if v.IsZero() || v.Field().IsRepeated() {
		return 0, false
	}
	return v.Elements().At(0).AsInt()
}

// AsEnum is a shortcut for [Element.AsEnum], if this value is singular.
func (v Value) AsEnum() Member {
	if v.IsZero() || v.Field().IsRepeated() {
		return Member{}
	}
	return v.Elements().At(0).AsEnum()
}

// AsFloat is a shortcut for [Element.AsFloat], if this value is singular.
func (v Value) AsFloat() (float64, bool) {
	if v.IsZero() || v.Field().IsRepeated() {
		return 0, false
	}
	return v.Elements().At(0).AsFloat()
}

// AsString is a shortcut for [Element.AsString], if this value is singular.
func (v Value) AsString() (string, bool) {
	if v.IsZero() || v.Field().IsRepeated() {
		return "", false
	}
	return v.Elements().At(0).AsString()
}

// AsMessage is a shortcut for [Element.AsMessage], if this value is singular.
func (v Value) AsMessage() MessageValue {
	if v.IsZero() {
		return MessageValue{}
	}

	m := v.Elements().At(0).AsMessage()

	if m.TypeURL() != "" {
		// If this is the concrete version of an Any message, it is effectively
		// singular: even if the reported field is a repeated g.p.Any, we treat
		// Any as having a singular "concrete" field that contains the actual
		// value (see [MessageValue.Concrete]).
		return m
	}

	if v.Field().IsRepeated() {
		return MessageValue{}
	}
	return m
}

// slice returns the underlying slice for this value.
//
// If this value isn't already in slice form, this puts it into it.
func (v Value) slice() *[]rawValueBits {
	if int32(v.Raw().field.id) < 0 {
		return v.Context().arenas.arrays.Deref(arena.Pointer[[]rawValueBits](v.Raw().bits))
	}

	slice := v.Context().arenas.arrays.New([]rawValueBits{v.Raw().bits})
	v.Raw().bits = rawValueBits(v.Context().arenas.arrays.Compress(slice))
	v.Raw().field.id ^= -1
	return slice
}

// Marshal converts this value into a wire format record and appends it to buf.
//
// If r is not nil, it will be used to record diagnostics generated during the
// marshal operation.
func (v Value) Marshal(buf []byte, r *report.Report) []byte {
	var ranges [][2]int
	buf, _ = v.marshal(buf, r, &ranges)
	return deleteRanges(buf, ranges)
}

// marshal is the recursive part of [Value.Marshal].
//
// See marshalFramed for the meanings of ranges and the int return value.
func (v Value) marshal(buf []byte, r *report.Report, ranges *[][2]int) ([]byte, int) {
	if r != nil {
		defer r.AnnotateICE(report.Snippetf(v.ValueAST(), "while marshalling this value"))
	}

	scalar := v.Field().Element().Predeclared()
	if v.Field().IsRepeated() && v.Elements().Len() > 1 {
		// Packed fields.
		switch {
		case scalar.IsVarint(), v.Field().Element().IsEnum():
			var bytes int
			for v := range seq.Values(v.Elements()) {
				bits := uint64(v.bits)
				if scalar.IsZigZag() {
					bits = protowire.EncodeZigZag(int64(bits))
				}
				bytes += protowire.SizeVarint(bits)
			}

			buf = protowire.AppendTag(buf, protowire.Number(v.Field().Number()), protowire.BytesType)
			buf = protowire.AppendVarint(buf, uint64(bytes))
			for v := range seq.Values(v.Elements()) {
				bits := uint64(v.bits)
				if scalar.IsZigZag() {
					bits = protowire.EncodeZigZag(int64(bits))
				}
				buf = protowire.AppendVarint(buf, bits)
			}
			return buf, 0

		case scalar.IsFixed():
			buf = protowire.AppendTag(buf, protowire.Number(v.Field().Number()), protowire.BytesType)
			buf = protowire.AppendVarint(buf, uint64(v.Elements().Len()*scalar.Bits()/8))

			for v := range seq.Values(v.Elements()) {
				bits := uint64(v.bits)
				switch {
				case scalar == predeclared.Float32:
					f64, _ := v.AsFloat()
					f32 := math.Float32bits(float32(f64))
					buf = protowire.AppendFixed32(buf, f32)
				case scalar.Bits() == 32:
					buf = protowire.AppendFixed32(buf, uint32(bits))
				default:
					buf = protowire.AppendFixed64(buf, bits)
				}
			}
			return buf, 0
		}
	}

	var n int
	for v := range seq.Values(v.Elements()) {
		switch {
		case scalar.IsVarint(), v.Field().Element().IsEnum():
			buf = protowire.AppendTag(buf, protowire.Number(v.Field().Number()), protowire.VarintType)
			bits := uint64(v.bits)
			if scalar.IsZigZag() {
				bits = protowire.EncodeZigZag(int64(bits))
			}
			buf = protowire.AppendVarint(buf, bits)
		case scalar == predeclared.Float32:
			buf = protowire.AppendTag(buf, protowire.Number(v.Field().Number()), protowire.Fixed32Type)
			f64, _ := v.AsFloat()
			f32 := math.Float32bits(float32(f64))
			buf = protowire.AppendFixed32(buf, f32)
		case scalar.IsFixed() && scalar.Bits() == 32:
			buf = protowire.AppendTag(buf, protowire.Number(v.Field().Number()), protowire.Fixed32Type)
			buf = protowire.AppendFixed32(buf, uint32(v.bits))
		case scalar.IsFixed():
			buf = protowire.AppendTag(buf, protowire.Number(v.Field().Number()), protowire.Fixed64Type)
			buf = protowire.AppendFixed64(buf, uint64(v.bits))
		case scalar.IsString():
			s, _ := v.AsString()

			buf = protowire.AppendTag(buf, protowire.Number(v.Field().Number()), protowire.BytesType)
			buf = protowire.AppendVarint(buf, uint64(len(s)))
			buf = append(buf, s...)

		default: // Message type.
			m := v.AsMessage()

			var k int
			var group bool // TODO: v.Field().IsGroup()
			if group {
				buf = protowire.AppendTag(buf, protowire.Number(v.Field().Number()), protowire.StartGroupType)
				buf, k = m.marshal(buf, r, ranges)
				buf = protowire.AppendTag(buf, protowire.Number(v.Field().Number()), protowire.EndGroupType)
			} else {
				buf = protowire.AppendTag(buf, protowire.Number(v.Field().Number()), protowire.BytesType)
				buf, k = marshalFramed(buf, r, ranges, func(buf []byte) ([]byte, int) {
					return m.marshal(buf, r, ranges)
				})
			}
			n += k
		}
	}

	return buf, n
}

func (v Value) suggestEdit(path, expr string, format string, args ...any) report.DiagnosticOption {
	key := v.KeyAST()
	value := v.ValueASTs().At(0)
	joined := source.Join(key, value)

	return report.SuggestEdits(
		joined,
		fmt.Sprintf(format, args...),
		report.Edit{
			Start: 0, End: key.Span().Len(),
			Replace: path,
		},
		report.Edit{
			Start:   value.Span().Start - joined.Start,
			End:     value.Span().End - joined.Start,
			Replace: expr,
		},
	)
}

// Element is an element within a [Value].
//
// This exists because array values contain multiple non-array elements; this
// type provides uniform access to such elements. See [Value.Elements].
type Element struct {
	withContext
	index int
	value Value
	bits  rawValueBits
}

// AST returns the expression this value was evaluated from.
func (e Element) AST() ast.ExprAny {
	if e.IsZero() || e.value.Raw().exprs == nil {
		return ast.ExprAny{}
	}

	idx := e.ValueNodeIndex()
	c := e.Context().AST()
	expr := id.WrapDyn(c, e.value.Raw().exprs[idx])
	if field := expr.AsField(); !field.IsZero() {
		expr = field.Value()
	}

	if array := expr.AsArray(); !array.IsZero() && e.value.Raw().elemIndices != nil {
		// We need to index into the array expression. The index is going to be
		// offset by the number of expressions before this one, which we
		// can get via elemIndices.
		n := int(e.value.Raw().elemIndices[idx]) - e.index - 1
		expr = array.Elements().At(n)
	}
	return expr
}

// ValueNodeIndex returns the index into [Value.ValueASTs] for this element's
// contributing expression. This can be used to obtain other ASTs related to
// this element, e.g.
//
//	key := e.Value().MessageKeys().At(e.ValueNodeIndex())
func (e Element) ValueNodeIndex() int {
	// We do O(log n) work here, because this function doesn't get called except
	// for diagnostics.

	idx := e.index
	if e.value.Raw().elemIndices != nil {
		// Figure out which expression contributes the value for e. We're looking
		// for the least upper bound.
		//
		// For example, if we have expressions [1, 2], [3, 4, 5], elemIndices
		// will be [2, 5], and we have that BinarySearch returns
		//
		// 0 -> 0, false
		// 1 -> 0, false
		// 2 -> 0, true
		// 3 -> 1, false
		// 4 -> 1, false
		var exact bool
		idx, exact = slices.BinarySearch(e.value.Raw().elemIndices, uint32(e.index))
		if exact {
			idx++
		}
	}

	return idx
}

// Value is the [Value] this element came from.
func (e Element) Value() Value {
	return e.value
}

// Field returns the field this value sets, which includes the value's type
// information.
func (e Element) Field() Member {
	return e.Value().Field()
}

// Type returns the type of this element.
//
// Note that this may be distinct from [Member.Element]. In the case that this is
// a google.protobuf.Any-typed field, this function will return the concrete
// type if known, rather than Any.
func (e Element) Type() Type {
	if msg := e.AsMessage(); !msg.IsZero() {
		// This will always be the concrete type, except in the case of
		// something naughty like my_any: { type_url: "...", value: "..." };
		// in that case this will be Any.
		return msg.Type()
	}

	return e.Field().Element()
}

// IsZeroValue returns whether this element contains the zero value for its type.
//
// Always returns false for repeated or message-typed fields.
func (e Element) IsZeroValue() bool {
	if e.IsZero() || e.Field().IsRepeated() || e.Field().Element().IsMessage() {
		return false
	}

	return e.value.Raw().bits == 0
}

// AsBool returns the bool value of this element.
//
// Returns ok == false if this is not a bool.
func (e Element) AsBool() (value, ok bool) {
	if e.Type().Predeclared() != predeclared.Bool {
		return false, false
	}
	return e.bits != 0, true
}

// AsUInt returns the value of this element as an unsigned integer.
//
// Returns false if this is not an unsigned integer.
func (e Element) AsUInt() (uint64, bool) {
	if !e.Type().Predeclared().IsUnsigned() {
		return 0, false
	}
	return uint64(e.bits), true
}

// AsInt returns the value of this element as a signed integer.
//
// Returns false if this is not a signed integer (enums are included as signed
// integers).
func (e Element) AsInt() (int64, bool) {
	if !e.Type().Predeclared().IsSigned() && !e.Type().IsEnum() {
		return 0, false
	}
	return int64(e.bits), true
}

// AsEnum returns the value of this element as a known enum value.
//
// Returns zero if this is not an enum or if the enum value is out of range.
func (e Element) AsEnum() Member {
	ty := e.Type()
	if !ty.IsEnum() {
		return Member{}
	}
	return ty.MemberByNumber(int32(e.bits))
}

// AsFloat returns the value of this element as a floating-point number.
//
// Returns false if this is not a float.
func (e Element) AsFloat() (float64, bool) {
	if !e.Type().Predeclared().IsFloat() {
		return 0, false
	}
	return math.Float64frombits(uint64(e.bits)), true
}

// AsString returns the value of this element as a string.
//
// Returns false if this is not a string.
func (e Element) AsString() (string, bool) {
	if !e.Type().Predeclared().IsString() {
		return "", false
	}
	return e.Context().session.intern.Value(intern.ID(e.bits)), true
}

// AsMessage returns the value of this element as a message literal.
//
// Returns the zero value if this is not a message.
func (e Element) AsMessage() MessageValue {
	// Avoid infinite recursion: Type() calls AsMessage().
	if !e.Field().Element().IsMessage() {
		return MessageValue{}
	}

	return id.Wrap(e.Context(), id.ID[MessageValue](e.bits))
}

// MessageValue is a message literal, represented as a list of ordered
// key-value pairs.
type MessageValue id.Node[MessageValue, *File, *rawMessageValue]

type rawMessageValue struct {
	byName   intern.Map[uint32]
	entries  []id.ID[Value]
	ty       Ref[Type]
	self     id.ID[Value]
	url      intern.ID
	concrete id.ID[MessageValue]
	pseudo   struct {
		defaultValue id.ID[Value]
		jsonName     id.ID[Value]
	}
}

// PseudoFields contains pseudo options, which are special option-like syntax
// for fields which are not real options. They can be accessed via
// [Message.PseudoFields].
type PseudoFields struct {
	Default  Value
	JSONName Value
}

// AsValue returns the [Value] corresponding to this message.
//
// This value can be used to retrieve the associated [Member] and from it the
// message's declared [Type].
func (v MessageValue) AsValue() Value {
	if v.IsZero() {
		return Value{}
	}
	return id.Wrap(v.Context(), v.Raw().self)
}

// Type returns this value's message type.
//
// If v was returned from [MessageValue.Concrete], its type need not be the
// same as v.AsValue()'s (although it can be, in the case of pathological
// Any-within-an-Any messages).
func (v MessageValue) Type() Type {
	if v.IsZero() {
		return Type{}
	}
	return GetRef(v.Context(), v.Raw().ty)
}

// TypeURL returns this value's type URL, if it is the concrete value of an
// Any.
func (v MessageValue) TypeURL() string {
	if v.IsZero() {
		return ""
	}
	return v.Context().session.intern.Value(v.Raw().url)
}

// Concrete returns the concrete version of this value if it is an Any.
//
// If it isn't an Any, or a .Raw()" Any (one not specified with the special type
// URL syntax), this returns v.
func (v MessageValue) Concrete() MessageValue {
	if v.IsZero() || v.Raw().concrete.IsZero() {
		return v
	}

	return id.Wrap(v.Context(), v.Raw().concrete)
}

// Field returns the field corresponding with the given member, if it is set.
func (v MessageValue) Field(field Member) Value {
	if field.Container() != v.Type() {
		return Value{}
	}

	name := field.InternedFullName()
	if o := field.Oneof(); !o.IsZero() {
		name = o.InternedFullName()
	}

	idx, ok := v.Raw().byName[name]
	if !ok {
		return Value{}
	}

	return id.Wrap(v.Context(), v.Raw().entries[idx])
}

// Fields yields the fields within this message literal, in insertion order.
func (v MessageValue) Fields() iter.Seq[Value] {
	return func(yield func(Value) bool) {
		if v.IsZero() {
			return
		}

		for _, p := range v.Raw().entries {
			v := id.Wrap(v.Context(), p)
			if !v.IsZero() && !yield(v) {
				return
			}
		}
	}
}

// pseudoFields returns pseudofields set on this message.
//
// This feature is used for tracking special options that do not correspond to
// real fields in an options message. They are not part of the message value
// and are not returned by Fields().
func (v MessageValue) pseudoFields() PseudoFields {
	if v.IsZero() {
		return PseudoFields{}
	}

	return PseudoFields{
		Default:  id.Wrap(v.Context(), v.Raw().pseudo.defaultValue),
		JSONName: id.Wrap(v.Context(), v.Raw().pseudo.jsonName),
	}
}

// Marshal serializes this message as wire format and appends it to buf.
//
// If r is not nil, it will be used to record diagnostics generated during the
// marshal operation.
func (v MessageValue) Marshal(buf []byte, r *report.Report) []byte {
	var ranges [][2]int
	buf, _ = v.marshal(buf, r, &ranges)
	return deleteRanges(buf, ranges)
}

// marshal is the recursive part of [MessageValue.Marshal].
//
// See marshalFramed for the meanings of ranges and the int return value.
func (v MessageValue) marshal(buf []byte, r *report.Report, ranges *[][2]int) ([]byte, int) {
	if v.IsZero() {
		return buf, 0
	}

	if m := v.Concrete(); m != v { // Manual handling for Any.
		url := m.TypeURL()
		buf = protowire.AppendTag(buf, 1, protowire.BytesType)
		buf = protowire.AppendVarint(buf, uint64(len(url)))
		buf = append(buf, url...)

		buf = protowire.AppendTag(buf, 2, protowire.BytesType)
		return marshalFramed(buf, r, ranges, func(buf []byte) ([]byte, int) {
			return m.marshal(buf, r, ranges)
		})
	}

	var n int
	for v := range v.Fields() {
		var k int
		buf, k = v.marshal(buf, r, ranges)
		n += k
	}
	return buf, n
}

// marshalFramed marshals arbitrary data, as appended by body, with a leading
// length prefix.
//
// The body function must return the number of bytes that it marked as "extra",
// by appending them to ranges. This allows the length prefix to be correct
// after accounting for deletions in deleteRanges. This allows us to marshal
// minimal length prefixes without quadratic time copying buffers around.
func marshalFramed(buf []byte, _ *report.Report, ranges *[][2]int, body func([]byte) ([]byte, int)) ([]byte, int) {
	// To avoid being accidentally quadratic, we encode every message
	// length with five bytes.
	mark := len(buf)
	buf = append(buf, make([]byte, 5)...)
	var n int
	buf, n = body(buf)
	bytes := uint64(len(buf) - (mark + 5) - n)
	if bytes > math.MaxUint32 {
		// This is not reachable today, because input files may be
		// no larger than 4GB. However, that may change at some point,
		// so keeping an ICE around is better than potentially getting
		// corrupt output later.
		//
		// Later, this should probably become a diagnostic.
		panic("protocompile/ir: marshalling options value overflowed length prefixes")
	}

	varint := protowire.AppendVarint(buf[mark:mark], bytes)
	if k := len(varint); k < 5 {
		*ranges = append(*ranges, [2]int{mark + k, mark + 5})
	}
	return buf, n + 5 - len(varint)
}

// deleteRanges deletes the given ranges from a byte array.
func deleteRanges(buf []byte, ranges [][2]int) []byte {
	if len(ranges) == 0 {
		return buf
	}

	slices.SortFunc(ranges, func(a, b [2]int) int {
		return cmp.Compare(a[0], b[0])
	})

	offset := 0
	for i, r1 := range ranges[:len(ranges)-1] {
		r2 := ranges[i+1]
		// Need to delete the interval between r1[0] and r1[1]. We do this
		// by copying r1[1]..r2[0] to r1[0]..
		copy(buf[r1[0]-offset:], buf[r1[1]:r2[0]])
		offset += r1[1] - r1[0]
	}
	// Need to delete the last interval. To do this, we do what we did above,
	// but where r2[0] is the end limit.
	r1 := ranges[len(ranges)-1]
	copy(buf[r1[0]-offset:], buf[r1[1]:])
	offset += r1[1] - r1[0]

	return buf[:len(buf)-offset]
}

// slot is returned by [MessageValue.insert]. It is a helper for making sure
// that the backreference for Value.Container is populated correctly.
type slot struct {
	msg  MessageValue
	slot *id.ID[Value]
}

func (s slot) IsZero() bool {
	return s.slot.IsZero()
}

func (s slot) Value() Value {
	return id.Wrap(s.msg.Context(), *s.slot)
}

func (s slot) Insert(v Value) {
	if !v.Container().IsZero() {
		panic("protocompile/ir: slot.Insert with non-top-level value")
	}
	v.Raw().container = s.msg.ID()
	*s.slot = v.ID()
}

// slot adds a new field to this message value, returning a pointer to the
// corresponding entry in the entries array, which can be initialized as-needed.
//
// A conflict occurs if there is already a field with the same number or part of
// the same oneof in this value. To determine whether to diagnose as a duplicate
// field or duplicate oneof, simply compare the field number of entry to that
// of the duplicate. If they are different, they share a oneof.
//
// When a conflict occurs, the existing rawValue pointer will be returned,
// whereas if the value is being inserted for the first time, the returned arena
// pointer will be nil and can be initialized by the caller.
func (v MessageValue) slot(field Member) slot {
	id := field.InternedFullName()
	if o := field.Oneof(); !o.IsZero() {
		id = o.InternedFullName()
	}

	n := len(v.Raw().entries)
	if actual, ok := mapsx.Add(v.Raw().byName, id, uint32(n)); !ok {
		return slot{v, &v.Raw().entries[actual]}
	}

	v.Raw().entries = append(v.Raw().entries, 0)
	return slot{v, slicesx.LastPointer(v.Raw().entries)}
}

// scalar is a type that can be converted into a [rawValueBits].
type scalar interface {
	bool |
		int32 | uint32 | int64 | uint64 |
		float32 | float64 |
		intern.ID | string
}

// newZeroScalar constructs a new scalar value.
func newZeroScalar(file *File, field Ref[Member]) Value {
	return id.Wrap(file, id.ID[Value](file.arenas.values.NewCompressed(rawValue{
		field: field,
	})))
}

// appendRaw appends a scalar value to the given array value.
func appendRaw(array Value, bits rawValueBits) {
	slice := array.slice()
	*slice = append(*slice, bits)
}

// newScalar appends a new message value to the given array value, and returns it.
//
// If anyType is not zero, it will be used as the type of the inner message
// value. This is used for Any-typed fields. Otherwise, the type of field is
// used instead.
func appendMessage(array Value) MessageValue {
	v := id.ID[MessageValue](array.Context().arenas.messages.NewCompressed(rawMessageValue{
		self:   array.ID(),
		ty:     array.Elements().At(0).AsMessage().Raw().ty,
		byName: make(intern.Map[uint32]),
	}))

	slice := array.slice()
	*slice = append(*slice, rawValueBits(v))

	return id.Wrap(array.Context(), v)
}

// newMessage constructs a new message value.
//
// If anyType is not zero, it will be used as the type of the inner message
// value. This is used for Any-typed fields. Otherwise, the type of field is
// used instead.
func newMessage(file *File, field Ref[Member]) MessageValue {
	member := GetRef(file, field)
	raw := id.ID[MessageValue](file.arenas.messages.NewCompressed(rawMessageValue{
		ty:     member.Raw().elem.ChangeContext(member.Context(), file),
		byName: make(intern.Map[uint32]),
	}))

	msg := id.Wrap(file, raw)
	msg.Raw().self = id.ID[Value](file.arenas.values.NewCompressed(rawValue{
		field: field,
		bits:  rawValueBits(raw),
	}))

	return msg
}

// newConcrete constructs a new value to be the concrete representation of
// v with the given type.
func newConcrete(m MessageValue, ty Type, url string) MessageValue {
	if !m.Raw().concrete.IsZero() {
		panic("protocompile/ir: set a concrete type more than once")
	}
	if !m.Type().IsAny() {
		panic("protocompile/ir: set concrete type on non-Any")
	}

	field := m.AsValue().Raw().field
	if int32(field.id) < 0 {
		field.id ^= -1
	}

	msg := newMessage(m.Context(), field)
	msg.Raw().ty = ty.toRef(m.Context())
	msg.Raw().url = m.Context().session.intern.Intern(url)
	m.Raw().concrete = msg.ID()
	return msg
}

// newScalarBits converts a scalar into.Raw() bits for storing in a [Value].
func newScalarBits[T scalar](file *File, v T) rawValueBits {
	switch v := any(v).(type) {
	case bool:
		if v {
			return 1
		}
		return 0

	case int32:
		return rawValueBits(v)
	case uint32:
		return rawValueBits(v)
	case int64:
		return rawValueBits(v)
	case uint64:
		return rawValueBits(v)

	case float32:
		// All float values are stored as binary64. All binary32 floats can be
		// losslessly encoded as binary64, so this conversion does not result
		// in precision loss.
		return rawValueBits(math.Float64bits(float64(v)))
	case float64:
		return rawValueBits(math.Float64bits(v))

	case intern.ID:
		return rawValueBits(v)
	case string:
		return rawValueBits(file.session.intern.Intern(v))
	default:
		panic("unreachable")
	}
}
