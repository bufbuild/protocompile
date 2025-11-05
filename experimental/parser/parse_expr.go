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
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/experimental/token/keyword"
	"github.com/bufbuild/protocompile/internal/ext/slicesx"
)

// parseExpr attempts to parse a full expression.
//
// May return nil if parsing completely fails.
// TODO: return something like ast.ExprError instead.
func parseExpr(p *parser, c *token.Cursor, where taxa.Place) ast.ExprAny {
	return parseExprInfix(p, c, where, ast.ExprAny{}, 0)
}

// parseExprInfix parses an infix expression.
//
// prec is the precedence; higher values mean tighter binding. This function calls itself
// with higher (or equal) precedence values.
func parseExprInfix(p *parser, c *token.Cursor, where taxa.Place, lhs ast.ExprAny, prec int) ast.ExprAny {
	if lhs.IsZero() {
		lhs = parseExprPrefix(p, c, where)
		if lhs.IsZero() || c.Done() {
			return lhs
		}
	}

	next := peekTokenExpr(p, c)
	switch prec {
	case 0:
		if where.Subject() == taxa.Array || where.Subject() == taxa.Dict {
			switch next.Keyword() {
			case keyword.Eq: // Allow equals signs, which are usually a mistake.
				p.Errorf("unexpected `=` in expression").Apply(
					report.Snippet(next),
					justify(p.File().Stream(), next.Span(), "replace this with an `:`", justified{
						report.Edit{Start: 0, End: 1, Replace: ":"},
						justifyLeft,
					}),
					report.Notef("a %s use `=`, not `:`, for setting fields", taxa.Dict),
				)
				fallthrough
			case keyword.Colon:
				return p.NewExprField(ast.ExprFieldArgs{
					Key:   lhs,
					Colon: c.Next(),
					Value: parseExprInfix(p, c, where, ast.ExprAny{}, prec+1),
				}).AsAny()

			case keyword.Braces, keyword.Less, keyword.Brackets:
				// This is for colon-less, array or dict-valued fields.
				if next.IsLeaf() {
					break
				}

				// The previous expression cannot also be a key-value pair, since
				// this messes with parsing of dicts, which are not comma-separated.
				//
				// In other words, consider the following, inside of an expression
				// context:
				//
				// foo: bar { ... }
				//
				// We want to diagnose the { as unexpected here, and it is better
				// for that to be done by whatever is calling parseExpr since it
				// will have more context.
				//
				// We also do not allow this inside of arrays, because we want
				// [a {}] to parse as [a, {}] not [a: {}].
				if lhs.Kind() == ast.ExprKindField || where.Subject() == taxa.Array {
					break
				}

				return p.NewExprField(ast.ExprFieldArgs{
					Key: lhs,
					// Why not call parseExprSolo? Suppose the following
					// (invalid) production:
					//
					// foo { ... } to { ... }
					//
					// Calling parseExprInfix will cause this to be parsed
					// as a range expression, which will be diagnosed when
					// we legalize.
					Value: parseExprInfix(p, c, where, ast.ExprAny{}, prec+1),
				}).AsAny()
			}
		}

		return parseExprInfix(p, c, where, lhs, prec+1)

	case 1:
		//nolint:gocritic // This is a switch for consistency with the rest of the file.
		switch next.Keyword() {
		case keyword.To:
			return p.NewExprRange(ast.ExprRangeArgs{
				Start: lhs,
				To:    c.Next(),
				End:   parseExprInfix(p, c, taxa.KeywordTo.After(), ast.ExprAny{}, prec),
			}).AsAny()
		}

		return parseExprInfix(p, c, where, lhs, prec+1)

	default:
		return lhs
	}
}

// parseExprPrefix parses a prefix expression.
//
// This is separate from "solo" expressions because if we every gain suffix-type
// expressions, such as f(), we need to parse -f() as -(f()), not (-f)().
func parseExprPrefix(p *parser, c *token.Cursor, where taxa.Place) ast.ExprAny {
	next := peekTokenExpr(p, c)
	switch {
	case next.IsZero():
		return ast.ExprAny{}

	case next.Keyword() == keyword.Minus:
		c.Next()
		inner := parseExprPrefix(p, c, taxa.Minus.After())
		return p.NewExprPrefixed(ast.ExprPrefixedArgs{
			Prefix: next,
			Expr:   inner,
		}).AsAny()

	default:
		return parseExprSolo(p, c, where)
	}
}

// parseExprSolo attempts to parse a "solo" expression, which is an expression that
// does not contain any operators.
//
// May return nil if parsing completely fails.
func parseExprSolo(p *parser, c *token.Cursor, where taxa.Place) ast.ExprAny {
	next := peekTokenExpr(p, c)
	switch {
	case next.IsZero():
		return ast.ExprAny{}

	case next.Kind() == token.String, next.Kind() == token.Number:
		return ast.ExprLiteral{File: p.File(), Token: c.Next()}.AsAny()

	case canStartPath(next):
		return ast.ExprPath{Path: parsePath(p, c)}.AsAny()

	case slicesx.Among(next.Keyword(), keyword.Braces, keyword.Less, keyword.Brackets):
		body := c.Next()
		in := taxa.Dict
		if body.Keyword() == keyword.Brackets {
			in = taxa.Array
		}

		// Due to wanting to not have <...> be a token tree by default in the
		// lexer, we need to perform rather complicated parsing here to handle
		// <a: b> syntax messages. (ugh)
		angles := body.Keyword() == keyword.Less
		children := c
		if !angles {
			children = body.Children()
		}

		elems := delimited[ast.ExprAny]{
			p:    p,
			c:    children,
			what: taxa.DictField,
			in:   in,

			delims:   []keyword.Keyword{keyword.Comma, keyword.Semi},
			required: false,
			exhaust:  !angles,
			trailing: true,
			parse: func(c *token.Cursor) (ast.ExprAny, bool) {
				expr := parseExpr(p, c, in.In())
				return expr, !expr.IsZero()
			},
			start: canStartExpr,
			stop: func(t token.Token) bool {
				return angles && t.Keyword() == keyword.Greater
			},
		}

		if in == taxa.Array {
			elems.what = taxa.Expr
			elems.delims = []keyword.Keyword{keyword.Comma}
			elems.required = true
			elems.trailing = false

			array := p.NewExprArray(body)
			elems.appendTo(array.Elements())
			return array.AsAny()
		}

		dict := p.NewExprDict(body)
		for expr, comma := range elems.iter {
			field := expr.AsField()
			if field.IsZero() {
				p.Error(errUnexpected{
					what:  expr,
					where: in.In(),
					want:  taxa.DictField.AsSet(),
				})

				field = p.NewExprField(ast.ExprFieldArgs{Value: expr})
			}

			dict.Elements().AppendComma(field, comma)
		}

		// If this is a pair of angle brackets, we need to fuse them.
		if angles {
			if c.Peek().Keyword() == keyword.Greater {
				token.Fuse(body, c.Next())
			}
		}

		return dict.AsAny()

	default:
		p.Error(errUnexpected{
			what:  next,
			where: where,
			want:  taxa.Expr.AsSet(),
		})

		return ast.ExprAny{}
	}
}

// peekTokenExpr peeks a token and generates an expression-specific diagnostic
// if the cursor is exhausted.
func peekTokenExpr(p *parser, c *token.Cursor) token.Token {
	next := c.Peek()
	if next.IsZero() {
		token, span := c.SeekToEnd()
		err := errUnexpected{
			what:  span,
			where: taxa.Expr.In(),
			want:  taxa.Expr.AsSet(),
			got:   taxa.EOF,
		}
		if !token.IsZero() {
			err.got = taxa.Classify(token)
		}

		p.Error(err)
	}
	return next
}
