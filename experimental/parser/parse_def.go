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

package parser

import (
	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/internal/taxa"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/experimental/token/keyword"
	"github.com/bufbuild/protocompile/internal/ext/slicesx"
)

type defParser struct {
	*parser
	c *token.Cursor

	kw token.Token
	in taxa.Noun

	args ast.DeclDefArgs

	inputs, outputs token.Token
	braces          token.Token

	outputTy ast.TypeAny // Used only for diagnostics.
}

type defFollower interface {
	// what returns the noun for this follower.
	what(*defParser) taxa.Noun

	// canStart returns whether this follower can be parsed next.
	canStart(*defParser) bool
	// parse parses this follower and returns its span; returns nil on failure.
	parse(*defParser) source.Span
	// prev returns the span of the first value parsed for this follower, or nil
	// if it has not been parsed yet.
	prev(*defParser) source.Span
}

var defFollowers = []defFollower{
	defInputs{}, defOutputs{},
	defValue{}, defOptions{},
	defBody{},
}

// parse parses a generic definition.
func (p *defParser) parse() ast.DeclDef {
	// Try to parse the various "followers". We try to parse as many as
	// possible: if we have `foo = 5 = 6`, we want to parse the second = 6,
	// diagnose it, and throw it away.
	var mark token.CursorMark
	var skipSemi bool
	lastFollower := -1
	for !p.c.Done() {
		ensureProgress(p.c, &mark)

		idx := -1
		for i := range defFollowers {
			if defFollowers[i].canStart(p) {
				idx = i
				break
			}
		}
		if idx < 0 {
			break
		}

		next := defFollowers[idx].parse(p)
		if next.IsZero() {
			continue
		}

		switch {
		case idx < lastFollower:
			// TODO: if we have already seen a follower at idx, we should
			// suggest removing this follower. Otherwise, we should suggest
			// moving this follower before the previous one.

			f := defFollowers[lastFollower]
			p.Error(errUnexpected{
				what:  next,
				where: f.what(p).After(),
				prev:  f.prev(p),
				got:   defFollowers[idx].what(p),
			})
		case idx == lastFollower:
			f := defFollowers[lastFollower]
			p.Error(errMoreThanOne{
				first:  f.prev(p),
				second: next,
				what:   f.what(p),
			})
		default:
			lastFollower = idx
		}

		if lastFollower == len(defFollowers)-1 {
			// Once we parse a body, we're done.
			skipSemi = true
			break
		}
	}

	// If we didn't see any braces, this def needs to be ended by a semicolon.
	if !skipSemi {
		semi, err := parseSemi(p.parser, p.c, taxa.Def)
		p.args.Semicolon = semi
		if err != nil {
			p.Error(err)
		}
	}

	if p.in == taxa.Enum {
		// Convert something that looks like an enum value into one.
		if tyPath := p.args.Type.AsPath(); !tyPath.IsZero() && p.args.Name.IsZero() &&
			// The reason for this is because if the user writes `message {}`, we want
			// to *not* turn it into an enum value with a body.
			//
			// TODO: Add a case for making sure that `rpc(foo)` is properly
			// diagnosed as an anonymous method.
			p.braces.IsZero() {
			p.args.Name = tyPath.Path
			p.args.Type = ast.TypeAny{}
		}
	}

	def := p.NewDeclDef(p.args)

	if !p.inputs.IsZero() {
		parseTypeList(p.parser, p.inputs, def.WithSignature().Inputs(), taxa.MethodIns)
	}
	if !p.outputs.IsZero() {
		parseTypeList(p.parser, p.outputs, def.WithSignature().Outputs(), taxa.MethodOuts)
	} else if !p.outputTy.IsZero() {
		span := p.outputTy.Span()
		p.Errorf("missing `(...)` around method return type").Apply(
			report.Snippet(span),
			report.SuggestEdits(
				span,
				"insert (...) around the return type",
				report.Edit{Start: 0, End: 0, Replace: "("},
				report.Edit{Start: span.Len(), End: span.Len(), Replace: ")"},
			),
		)
		seq.Append(def.WithSignature().Outputs(), p.outputTy)
	}

	if !p.braces.IsZero() {
		var in taxa.Noun
		switch p.kw.Text() {
		case "message":
			in = taxa.Message
		case "enum":
			in = taxa.Enum
		case "service":
			in = taxa.Service
		case "extend":
			in = taxa.Extend
		case "group":
			in = taxa.Field
		case "oneof":
			in = taxa.Oneof
		case "rpc":
			in = taxa.Method
		default:
			in = taxa.Def
		}

		def.SetBody(parseBody(p.parser, p.braces, in))
	}

	return def
}

type defInputs struct{}

func (defInputs) what(*defParser) taxa.Noun  { return taxa.MethodIns }
func (defInputs) canStart(p *defParser) bool { return p.c.Peek().Keyword() == keyword.Parens }

func (defInputs) parse(p *defParser) source.Span {
	next := p.c.Next()
	if next.IsLeaf() {
		return source.Span{} // Diagnosed by the lexer.
	}

	if p.inputs.IsZero() {
		p.inputs = next
	}
	return next.Span()
}

func (defInputs) prev(p *defParser) source.Span { return p.inputs.Span() }

type defOutputs struct{}

func (defOutputs) what(*defParser) taxa.Noun  { return taxa.MethodOuts }
func (defOutputs) canStart(p *defParser) bool { return p.c.Peek().Keyword() == keyword.Returns }

func (defOutputs) parse(p *defParser) source.Span {
	// Note that the inputs and outputs of a method are parsed
	// separately, so foo(bar) and foo returns (bar) are both possible.
	returns := p.c.Next()
	if p.args.Returns.IsZero() {
		p.args.Returns = returns
	}

	var ty ast.TypeAny
	list, err := punctParser{
		parser: p.parser, c: p.c,
		want:  keyword.Parens,
		where: taxa.KeywordReturns.After(),
	}.parse()
	if list.IsZero() && canStartPath(p.c.Peek()) {
		// Suppose the user writes `returns my.Response`. This is
		// invalid but reasonable so we want to diagnose it. To do this,
		// we parse a single type w/o parens and diagnose it later.
		ty = parseType(p.parser, p.c, taxa.KeywordReturns.After())
	} else if err != nil {
		p.Error(err)
		return source.Span{}
	}

	if p.outputs.IsZero() && p.outputTy.IsZero() {
		if !list.IsZero() {
			p.outputs = list
		} else {
			p.outputTy = ty
		}
	}

	if !list.IsZero() {
		return source.Join(returns, list)
	}
	return source.Join(returns, ty)
}

func (defOutputs) prev(p *defParser) source.Span {
	if !p.outputTy.IsZero() {
		return source.Join(p.args.Returns, p.outputTy)
	}
	return source.Join(p.args.Returns, p.outputs)
}

type defValue struct{}

func (defValue) what(p *defParser) taxa.Noun {
	switch {
	case p.kw.Text() == "option":
		return taxa.OptionValue
	case p.args.Type.IsZero():
		return taxa.EnumValue
	default:
		return taxa.FieldTag
	}
}

func (defValue) canStart(p *defParser) bool {
	next := p.c.Peek()
	// This will slurp up a value *not* prefixed with an =, too, but
	// that case needs to be diagnosed. This allows us to diagnose
	// e.g.
	//
	//  optional int32 x 5; // Missing =.
	//
	// However, if we've already seen {}, [], or another value, we break
	// instead, since this suggests we're peeking the next def.
	switch {
	case next.Keyword() == keyword.Eq:
		return true
	case canStartPath(next):
		// If the next "expression" looks like a path, this likelier to be
		// due to a missing semicolon than a missing =.
		return false
	case slicesx.Among(next.Keyword(), keyword.Brackets, keyword.Braces):
		// Exclude the two followers after this one.
		return false
	case canStartExpr(next):
		// Don't try to parse an expression if we've already parsed
		// an expression, options, or another expression.
		return p.args.Value.IsZero() && p.args.Options.IsZero() && p.braces.IsZero()
	default:
		return false
	}
}

func (defValue) parse(p *defParser) source.Span {
	eq, err := punctParser{
		parser: p.parser, c: p.c,
		want:   keyword.Eq,
		where:  taxa.Def.In(),
		insert: justifyBetween,
	}.parse()
	if err != nil {
		p.Error(err)
	}

	expr := parseExpr(p.parser, p.c, taxa.Def.In())
	if expr.IsZero() {
		return source.Span{} // parseExpr already generated diagnostics.
	}

	if p.args.Value.IsZero() {
		p.args.Equals = eq
		p.args.Value = expr
	}
	return source.Join(eq, expr)
}

func (defValue) prev(p *defParser) source.Span {
	if p.args.Value.IsZero() {
		return source.Span{}
	}
	return source.Join(p.args.Equals, p.args.Value)
}

type defOptions struct{}

func (defOptions) what(*defParser) taxa.Noun  { return taxa.CompactOptions }
func (defOptions) canStart(p *defParser) bool { return canStartOptions(p.c.Peek()) }

func (defOptions) parse(p *defParser) source.Span {
	next := p.c.Next()
	if next.IsLeaf() {
		return source.Span{} // Diagnosed by the lexer.
	}

	if p.args.Options.IsZero() {
		p.args.Options = parseOptions(p.parser, next, taxa.Def)
	}
	return next.Span()
}

func (defOptions) prev(p *defParser) source.Span { return p.args.Options.Span() }

type defBody struct{}

func (defBody) what(*defParser) taxa.Noun  { return taxa.Body }
func (defBody) canStart(p *defParser) bool { return canStartBody(p.c.Peek()) }

func (defBody) parse(p *defParser) source.Span {
	next := p.c.Next()
	if next.IsLeaf() {
		return source.Span{} // Diagnosed by the lexer.
	}

	if p.braces.IsZero() {
		p.braces = next
	}
	return next.Span()
}

func (defBody) prev(p *defParser) source.Span { return p.braces.Span() }
