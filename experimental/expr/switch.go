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

package expr

import (
	"github.com/bufbuild/protocompile/experimental/id"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/experimental/token/keyword"
)

// Switch selects one of several blocks to execute based on a number of cases.
//
// # Grammar
//
//	Switch := `switch` Expr? `{` Case* `}`
type Switch id.Node[Switch, *Context, *rawSwitch]

// Case is a case block within a [Switch].
//
// # Grammar
//
//	Case := (`case` (Expr `,`)* Expr? | `else`) `:`
type Case id.Node[Case, *Context, *rawCase]

// SwitchArgs is arguments for [Nodes.NewSwitch].
type SwitchArgs struct {
	Switch token.Token
	Arg    Expr
	Braces token.Token
}

// CaseArgs is arguments for [Nodes.NewCase].
type CaseArgs struct {
	Keyword token.Token
	Alts    Params
	Colon   token.Token
	Block   Block
}

type rawSwitch struct {
	cases  id.Seq[Case, *Context, *rawCase]
	arg    id.Dyn[Expr, Kind]
	kw     token.ID
	braces token.ID
}

type rawCase struct {
	kw, colon token.ID
	alts      id.ID[Params]
	block     id.ID[Block]
}

// AsAny type-erases this type value.
//
// See [Expr] for more information.
func (e Switch) AsAny() Expr {
	if e.IsZero() {
		return Expr{}
	}
	return id.WrapDyn(e.Context(), id.NewDyn(KindSwitch, id.ID[Expr](e.ID())))
}

// SwitchToken returns this expression's switch token.
func (e Switch) SwitchToken() token.Token {
	if e.IsZero() {
		return token.Zero
	}
	return id.Wrap(e.Context().Stream(), e.Raw().kw)
}

// Arg returns the optional argument to the switch.
func (e Switch) Arg() Expr {
	if e.IsZero() {
		return Expr{}
	}
	return id.WrapDyn(e.Context(), e.Raw().arg)
}

// Braces returns the braces for the block within the switch.
func (e Switch) Braces() token.Token {
	if e.IsZero() {
		return token.Zero
	}
	return id.Wrap(e.Context().Stream(), e.Raw().braces)
}

// Cases returns the cases contained within the switch.
func (e Switch) Cases() seq.Inserter[Case] {
	var cases *id.Seq[Case, *Context, *rawCase]
	if !e.IsZero() {
		cases = &e.Raw().cases
	}
	return cases.Inserter(e.Context())
}

// Span implements [source.Spanner].
func (e Switch) Span() source.Span {
	return source.Join(e.SwitchToken(), e.Arg(), e.Braces())
}

// Kind returns which kind of case this is.
func (e Case) Kind() keyword.Keyword {
	return e.KeywordToken().Keyword()
}

// KeywordToken returns this switch's "case" or "else" keyword.
func (e Case) KeywordToken() token.Token {
	if e.IsZero() {
		return token.Zero
	}
	return id.Wrap(e.Context().Stream(), e.Raw().kw)
}

// Colon returns the colon at the end of the alternatives.
func (e Case) Colon() token.Token {
	if e.IsZero() {
		return token.Zero
	}
	return id.Wrap(e.Context().Stream(), e.Raw().colon)
}

// Alts returns the potential alternatives for this case.
func (e Case) Alts() Params {
	if e.IsZero() {
		return Params{}
	}
	return id.Wrap(e.Context(), e.Raw().alts)
}

// Block returns the block of expressions for this case.
func (e Case) Block() Block {
	if e.IsZero() {
		return Block{}
	}
	return id.Wrap(e.Context(), e.Raw().block)
}

// Span implements [source.Spanner].
func (e Case) Span() source.Span {
	return source.Join(e.KeywordToken(), e.Alts(), e.Colon(), e.Block())
}
