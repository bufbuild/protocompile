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

//nolint:revive
package exprx

import (
	"fmt"

	"github.com/bufbuild/protocompile/experimental/expr"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/experimental/token/keyword"
	exprpb "github.com/bufbuild/protocompile/internal/gen/buf/compiler/expr/v1alpha1"
)

// ToProtoOptions contains options for the [File.ToProto] function.
type ToProtoOptions struct {
	// If set, no spans will be serialized.
	//
	// This operation only destroys non-semantic information.
	OmitSpans bool

	// If set, the contents of the file the AST was parsed from will not
	// be serialized.
	OmitFile bool
}

// ToProto converts this AST into a Protobuf representation, which may be
// serialized.
//
// Note that package ast does not support deserialization from this proto;
// instead, you will need to re-parse the text file included in the message.
// This is because the AST is much richer than what is stored in this message;
// the message only provides enough information for further semantic analysis
// and diagnostic generation, but not for pretty-printing.
//
// Panics if the AST contains a cycle (e.g. a message that contains itself as
// a nested message). Parsed ASTs will never contain cycles, but users may
// modify them into a cyclic state.
func ToProto(e expr.Expr, options ToProtoOptions) *exprpb.Expr {
	return (&protoEncoder{ToProtoOptions: options}).expr(e)
}

// protoEncoder is the state needed for converting an AST node into a Protobuf message.
type protoEncoder struct {
	ToProtoOptions

	stackMap map[source.Spanner]struct{}
}

// checkCycle panics if v is visited cyclically.
//
// Should be called like this, so that on function exit the entry is popped:
//
//	defer c.checkCycle(v)()
func (c *protoEncoder) checkCycle(v source.Spanner) func() {
	if c.stackMap == nil {
		c.stackMap = make(map[source.Spanner]struct{})
	}

	_, cycle := c.stackMap[v]
	c.stackMap[v] = struct{}{}

	if cycle {
		panic(fmt.Sprintf("protocompile/ast: called File.ToProto on a cyclic AST %v", v.Span()))
	}

	return func() { delete(c.stackMap, v) }
}

func (c *protoEncoder) span(s source.Spanner) *exprpb.Span {
	if c.OmitSpans || s == nil {
		return nil
	}

	span := s.Span()
	if span.IsZero() {
		return nil
	}

	return &exprpb.Span{
		Start: uint32(span.Start),
		End:   uint32(span.End),
	}
}

func (c *protoEncoder) expr(e expr.Expr) *exprpb.Expr {
	if e.IsZero() {
		return nil
	}

	pb := new(exprpb.Expr)
	defer c.checkCycle(e)()
	switch e.Kind() {
	case expr.KindBlock:
		pb.Expr = &exprpb.Expr_Block{Block: c.block(e.AsBlock())}
	case expr.KindCall:
		pb.Expr = &exprpb.Expr_Call{Call: c.call(e.AsCall())}
	case expr.KindControl:
		pb.Expr = &exprpb.Expr_Control{Control: c.control(e.AsControl())}
	case expr.KindFor:
		pb.Expr = &exprpb.Expr_For{For: c.for_(e.AsFor())}
	case expr.KindFunc:
		pb.Expr = &exprpb.Expr_Func{Func: c.func_(e.AsFunc())}
	case expr.KindIf:
		pb.Expr = &exprpb.Expr_If{If: c.if_(e.AsIf())}
	case expr.KindOp:
		pb.Expr = &exprpb.Expr_Op{Op: c.op(e.AsOp())}
	case expr.KindRecord:
		pb.Expr = &exprpb.Expr_Record{Record: c.params(e.AsRecord().Entries())}
	case expr.KindSwitch:
		pb.Expr = &exprpb.Expr_Switch{Switch: c.switch_(e.AsSwitch())}
	case expr.KindToken:
		pb.Expr = &exprpb.Expr_Token{Token: c.token(e.AsToken())}
	default:
		panic("unexpected expr.Kind")
	}

	return pb
}

func (c *protoEncoder) block(e expr.Block) *exprpb.Block {
	if e.IsZero() {
		return nil
	}

	pb := &exprpb.Block{
		Span: c.span(e),
	}
	for expr := range seq.Values(e.Exprs()) {
		pb.Exprs = append(pb.Exprs, c.expr(expr))
	}
	return pb
}

func (c *protoEncoder) call(e expr.Call) *exprpb.Call {
	if e.IsZero() {
		return nil
	}

	return &exprpb.Call{
		Callee: c.expr(e.Callee()),
		Params: c.params(e.Args()),
		Span:   c.span(e),
	}
}

func (c *protoEncoder) control(e expr.Control) *exprpb.Control {
	if e.IsZero() {
		return nil
	}

	var kind exprpb.Control_Kind
	switch e.Kind() {
	case keyword.Return:
		kind = exprpb.Control_RETURN
	case keyword.Break:
		kind = exprpb.Control_BREAK
	case keyword.Continue:
		kind = exprpb.Control_CONTINUE
	}

	return &exprpb.Control{
		Kind:   kind,
		Args:   c.params(e.Args()),
		Cond:   c.expr(e.Condition()),
		Span:   c.span(e),
		KwSpan: c.span(e.KeywordToken()),
		IfSpan: c.span(e.IfToken()),
	}
}

func (c *protoEncoder) for_(e expr.For) *exprpb.For {
	if e.IsZero() {
		return nil
	}

	return &exprpb.For{
		Vars:    c.params(e.Vars()),
		Iter:    c.expr(e.Iterator()),
		Block:   c.block(e.Block()),
		Span:    c.span(e),
		ForSpan: c.span(e.ForToken()),
		InSpan:  c.span(e.InToken()),
	}
}

func (c *protoEncoder) func_(e expr.Func) *exprpb.Func {
	if e.IsZero() {
		return nil
	}

	return &exprpb.Func{
		Name:      e.Name().Name(),
		Params:    c.params(e.Params()),
		Return:    c.expr(e.Return()),
		Body:      c.expr(e.Body()),
		Span:      c.span(e),
		FuncSpan:  c.span(e.FuncToken()),
		ArrowSpan: c.span(e.Arrow()),
	}
}

func (c *protoEncoder) if_(e expr.If) *exprpb.If {
	if e.IsZero() {
		return nil
	}
	defer c.checkCycle(e)()

	return &exprpb.If{
		Cond:     c.expr(e.Condition()),
		Block:    c.block(e.Block()),
		Else:     c.if_(e.Else()),
		Span:     c.span(e),
		ElseSpan: c.span(e.ElseToken()),
		IfSpan:   c.span(e.IfToken()),
	}
}

func (c *protoEncoder) op(e expr.Op) *exprpb.Op {
	if e.IsZero() {
		return nil
	}

	var op exprpb.Op_Kind
	switch e.Operator() {
	case keyword.Assign:
		op = exprpb.Op_ASSIGN
	case keyword.AssignNew:
		op = exprpb.Op_ASSIGN_NEW
	case keyword.AssignAdd:
		op = exprpb.Op_ASSIGN_ADD
	case keyword.AssignSub:
		op = exprpb.Op_ASSIGN_SUB
	case keyword.AssignMul:
		op = exprpb.Op_ASSIGN_MUL
	case keyword.AssignDiv:
		op = exprpb.Op_ASSIGN_DIV
	case keyword.AssignRem:
		op = exprpb.Op_ASSIGN_REM

	case keyword.Comma:
		op = exprpb.Op_COMMA
	case keyword.Or:
		op = exprpb.Op_OR
	case keyword.And:
		op = exprpb.Op_AND
	case keyword.Not:
		op = exprpb.Op_NOT

	case keyword.Eq:
		op = exprpb.Op_EQ
	case keyword.Ne:
		op = exprpb.Op_NE
	case keyword.Lt:
		op = exprpb.Op_LT
	case keyword.Gt:
		op = exprpb.Op_GT
	case keyword.Le:
		op = exprpb.Op_LE
	case keyword.Ge:
		op = exprpb.Op_GE

	case keyword.Range:
		op = exprpb.Op_RANGE
	case keyword.RangeEq:
		op = exprpb.Op_RANGE_EQ

	case keyword.Add:
		op = exprpb.Op_ADD
	case keyword.Sub:
		op = exprpb.Op_SUB
	case keyword.Mul:
		op = exprpb.Op_MUL
	case keyword.Div:
		op = exprpb.Op_DIV
	case keyword.Rem:
		op = exprpb.Op_REM

	case keyword.Dot:
		op = exprpb.Op_PROPERTY
	}

	return &exprpb.Op{
		Left:   c.expr(e.Left()),
		Right:  c.expr(e.Right()),
		Op:     op,
		Span:   c.span(e),
		OpSpan: c.span(e.OperatorToken()),
	}
}

func (c *protoEncoder) switch_(e expr.Switch) *exprpb.Switch {
	if e.IsZero() {
		return nil
	}

	pb := &exprpb.Switch{
		Arg:        c.expr(e.Arg()),
		Span:       c.span(e),
		SwitchSpan: c.span(e.SwitchToken()),
		BlockSpan:  c.span(e.Braces()),
	}
	for p := range seq.Values(e.Cases()) {
		pb.Cases = append(pb.Cases, &exprpb.Switch_Case{
			Else:      p.Kind() == keyword.Else,
			Alts:      c.params(p.Alts()),
			Block:     c.block(p.Block()),
			Span:      c.span(p),
			KwSpan:    c.span(p.KeywordToken()),
			ColonSpan: c.span(p.Colon()),
		})
	}
	return pb
}

func (c *protoEncoder) token(e expr.Token) *exprpb.Token {
	if e.IsZero() {
		return nil
	}

	pb := &exprpb.Token{
		Span: c.span(e),
	}
	switch e.Kind() {
	case token.Number:
		if n, ok := e.AsNumber().Int(); ok {
			pb.Value = &exprpb.Token_Int{Int: n}
		} else if n, ok := e.AsNumber().Float(); ok {
			pb.Value = &exprpb.Token_Float{Float: n}
		}
	case token.String:
		pb.Value = &exprpb.Token_String_{String_: e.AsString().Text()}
	case token.Ident:
		pb.Value = &exprpb.Token_Ident{Ident: e.Name()}
	}
	return pb
}

func (c *protoEncoder) params(e expr.Params) *exprpb.Params {
	if e.IsZero() {
		return nil
	}

	var brackets exprpb.Brackets
	switch e.Brackets().Keyword() {
	case keyword.Parens:
		brackets = exprpb.Brackets_BRACKETS_PARENS
	case keyword.Brackets:
		brackets = exprpb.Brackets_BRACKETS_SQUARE
	case keyword.Braces:
		brackets = exprpb.Brackets_BRACKETS_CURLY
	case keyword.Angles:
		brackets = exprpb.Brackets_BRACKETS_ANGLE
	}

	pb := &exprpb.Params{
		Brackets: brackets,
		Span:     c.span(e),
	}
	for p := range seq.Values(e) {
		pb.Params = append(pb.Params, &exprpb.Params_Param{
			Name:      c.expr(p.Name),
			Expr:      c.expr(p.Expr),
			Cond:      c.expr(p.Cond),
			Span:      c.span(p),
			ColonSpan: c.span(p.Colon),
			IfSpan:    c.span(p.If),
		})
	}
	return pb
}
