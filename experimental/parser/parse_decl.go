// Copyright 2020-2024 Buf Technologies, Inc.
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
)

type exprComma struct {
	expr  ast.ExprAny
	comma token.Token
}

// parseDecl parses any Protobuf declaration.
//
// This function will always advance cursor if it is not empty.
func parseDecl(p *parser, c *token.Cursor, in taxa.Subject) ast.DeclAny {
	first := c.Peek()
	if first.Nil() {
		return ast.DeclAny{}
	}

	if first.Text() == ";" {
		c.Pop()

		// This is an empty decl.
		return p.NewDeclEmpty(first).AsAny()
	}

	// This is a bare declaration body.
	if first.Text() == "{" && !first.IsLeaf() {
		c.Pop()
		return parseBody(p, first, in).AsAny()
	}

	// We need to parse a path here. At this point, we need to generate a
	// diagnostic if there is anything else in our way before hitting parsePath.
	if !canStartPath(first) {
		// Consume the token, emit a diagnostic, and throw it away.
		c.Pop()

		p.Error(errUnexpected{
			what:  first,
			where: in.In(),
			want: taxa.NewSet(
				taxa.Ident,
				taxa.Period,
				taxa.Semicolon,
				taxa.Parens,
				taxa.Braces,
			),
		})
		return ast.DeclAny{}
	}

	// Parse a type followed by a path. This is the "most general" prefix of almost all
	// possible productions in a decl. If the type is a TypePath which happens to be
	// a keyword, we try to parse the appropriate thing (with one token of lookahead),
	// and otherwise parse a field.
	mark := c.Mark()
	ty, path := parseType(p, c, in.In(), true)

	// Extract a putative leading keyword from this. Note that a field's type,
	// if relative, cannot start with any of the following identifiers:
	//
	// message enum oneof reserved
	// extensions extend option
	// optional required repeated
	//
	// This is used here to disambiguated between a generic DeclDef and one of
	// the other decl nodes.
	var kw token.Token
	if path := ty.AsPath(); !path.Nil() {
		kw = path.AsIdent()
	}

	// Check for the various special cases.
	next := c.Peek()
	switch kw.Text() {
	case "syntax", "edition":
		// Syntax and edition are parsed only at the top level. Otherwise, they
		// start a def.
		if in != taxa.TopLevel {
			break
		}

		args := ast.DeclSyntaxArgs{
			Keyword: kw,
		}

		in := taxa.Syntax
		if kw.Text() == "edition" {
			in = taxa.Edition
		}

		eq, err := p.Punct(c, "=", in.In())
		args.Equals = eq
		if err != nil {
			p.Error(err)
		}

		// Regardless of if we see an = sign, try to parse an expression if we
		// can.
		if !args.Equals.Nil() || canStartExpr(c.Peek()) {
			// If there is a trailing ; instead of an expression, make sure we
			// grab it. Otherwise, parseExpr will throw it away.
			if semi := c.Peek(); semi.Text() == ";" {
				p.Error(errUnexpected{
					what:  semi,
					where: in.In(),
					want:  taxa.Expr.AsSet(),
				})
				args.Semicolon = c.Pop()
			} else {
				args.Value = parseExpr(p, c, in.In())
			}
		}

		if args.Semicolon.Nil() {
			// Before we grab the ;, parse custom options.
			if next := c.Peek(); next.Text() == "[" && !next.IsLeaf() {
				args.Options = parseOptions(p, c.Pop(), in)
			}

			args.Semicolon, err = p.Punct(c, ";", in.After())
			// Only diagnose a missing semicolon if we successfully parsed some
			// kind of partially-valid expression. Otherwise, we might diagnose
			// the same extraneous/missing ; twice.
			if err != nil && !args.Value.Nil() {
				p.Error(err)
			}
		}

		return p.NewDeclSyntax(args).AsAny()

	case "package":
		// Package is only parsed only at the top level. Otherwise, it starts
		// a def.
		if in != taxa.TopLevel {
			break
		}

		args := ast.DeclPackageArgs{
			Keyword: kw,
			Path:    path,
		}

		if next := c.Peek(); next.Text() == "[" && !next.IsLeaf() {
			args.Options = parseOptions(p, c.Pop(), in)
		}

		semi, err := p.Punct(c, ";", taxa.Package.After())
		args.Semicolon = semi
		if err != nil {
			p.Error(err)
		}

		return p.NewDeclPackage(args).AsAny()

	case "import":
		// We parse imports inside of any body. However, outside of the top
		// level, we interpret import foo as a field. import foo.bar is still
		// an import, because we want to diagnose what is clearly an attempt to
		// import by path rather than by file.
		//
		// TODO: this treats import public inside of a message as a field, which
		// may result in worse diagnostics.
		if in != taxa.TopLevel && !path.AsIdent().Nil() {
			break
		}
		// This is definitely a field.
		if next.Text() == "=" {
			break
		}

		args := ast.DeclImportArgs{
			Keyword: kw,
		}

		in := taxa.Import
		modifier := path.AsIdent().Name()
		switch {
		case modifier == "public":
			in = taxa.PublicImport
			args.Modifier = path.AsIdent()
		case modifier == "weak":
			in = taxa.WeakImport
			args.Modifier = path.AsIdent()
		case !path.Nil():
			// This will catch someone writing `import foo.bar;` when we legalize.
			args.ImportPath = ast.ExprPath{Path: path}.AsAny()
		}

		if args.ImportPath.Nil() && canStartExpr(next) {
			args.ImportPath = parseExpr(p, c, in.In())
		}

		if next := c.Peek(); next.Text() == "[" && !next.IsLeaf() {
			args.Options = parseOptions(p, c.Pop(), in)
		}

		semi, err := p.Punct(c, ";", in.After())
		args.Semicolon = semi
		if err != nil && args.ImportPath.Nil() {
			p.Error(err)
		}

		return p.NewDeclImport(args).AsAny()

	case "reserved", "extensions":
		if next.Text() == "=" {
			// If whatever follows the path is an =, we're going to assume this
			// is trying to be a field.
			break
		}

		// Otherwise, rewind the cursor to before we parsed a type, and
		// parse a range instead. Rewinding is necessary because otherwise we get
		// into an annoying situation where if we have e.g. reserved foo to bar;
		// we have already consumed reserved foo, but we want to push foo
		// through the expression machinery to get foo to bar as a single
		// expression.
		c.Rewind(mark)
		return parseRange(p, c).AsAny()
	}

	return parseDef(p, c, kw, ty, path).AsAny()
}

// parseBody parses a ({}-delimited) body of declarations.
func parseBody(p *parser, braces token.Token, in taxa.Subject) ast.DeclBody {
	body := p.NewDeclBody(braces)

	// Drain the contents of the body into it. Remember,
	// parseDecl must always make progress if there is more to
	// parse.
	c := braces.Children()
	for !c.Done() {
		if next := parseDecl(p, c, in); !next.Nil() {
			body.Append(next)
		}
	}

	return body
}

// parseRange parses a reserved/extensions range.
func parseRange(p *parser, c *token.Cursor) ast.DeclRange {
	// Consume the keyword token.
	kw := c.Pop()

	in := taxa.Extensions
	if kw.Text() == "reserved" {
		in = taxa.Reserved
	}

	// Consume expressions until we hit a semicolon or the [ of a compact
	// options. Note that this means that we do not parse reserved [1, 2, 3];
	// "correctly".
	var (
		// badExpr keeps track of whether we exited the loop due to a parse
		// error or because we hit ; or [ or EOF.
		badExpr bool
		exprs   []exprComma
	)

	// Parse expressions until we hit a semicolon or the [ of compact options.
	elems := commas(p, c, true, func(c *token.Cursor) (ast.ExprAny, bool) {
		next := c.Peek().Text()
		if next == ";" || next == "[" {
			return ast.ExprAny{}, false
		}

		expr := parseExpr(p, c, in.In())
		badExpr = badExpr || expr.Nil()

		return expr, !expr.Nil()
	})
	elems(func(expr ast.ExprAny, comma token.Token) bool {
		exprs = append(exprs, exprComma{expr, comma})
		return true
	})

	var options ast.CompactOptions
	if next := c.Peek(); next.Text() == "[" && !next.IsLeaf() {
		options = parseOptions(p, c.Pop(), in)
	}

	// Parse a semicolon, if possible.
	semi, err := p.Punct(c, ";", in.After())
	if err != nil && !badExpr {
		p.Error(err)
	}

	r := p.NewDeclRange(ast.DeclRangeArgs{
		Keyword:   kw,
		Options:   options,
		Semicolon: semi,
	})
	for _, e := range exprs {
		r.AppendComma(e.expr, e.comma)
	}

	return r
}

// parseDef parses a generic definition.
func parseDef(p *parser, c *token.Cursor, kw token.Token, ty ast.TypeAny, path ast.Path) ast.DeclDef {
	args := ast.DeclDefArgs{
		Type: ty,
		Name: path,
	}

	var (
		inputs, outputs token.Token
		braces          token.Token

		outputTy ast.TypeAny // Used only for diagnostics.
	)

	// Try to parse the various "followers". We try to parse as many as
	// possible: if we have `foo = 5 = 6`, we want to parse the second = 6,
	// diagnose it, and throw it away.
	var mark token.CursorMark
followers:
	for !c.Done() {
		ensureProgress(c, &mark)

		next := c.Peek()
		switch next.Text() {
		case "(": // Method inputs.
			c.Pop()
			if !inputs.Nil() {
				p.Error(errMoreThanOne{
					first:  inputs,
					second: next,
					what:   taxa.MethodIns,
				})
				continue
			}

			switch {
			case !outputs.Nil(), !outputTy.Nil():
				var prev report.Span
				switch {
				case !outputs.Nil():
					prev = report.Join(args.Returns, outputs)
				case !outputTy.Nil():
					prev = report.Join(args.Returns, outputTy)
				}

				p.Error(errUnexpected{
					what:  next,
					where: taxa.MethodOuts.After(),
					prev:  prev,
					got:   taxa.MethodIns,
				})
			case !args.Options.Nil():
				p.Error(errUnexpected{
					what:  next,
					where: taxa.CompactOptions.After(),
					prev:  args.Options,
					got:   taxa.MethodIns,
				})
			case !braces.Nil():
				p.Error(errUnexpected{
					what:  next,
					where: taxa.Body.After(),
					prev:  braces,
					got:   taxa.MethodIns,
				})
			}

			inputs = next
			continue

		case "returns": // Method outputs.
			// Note that the inputs and outputs of a method are parsed
			// separately, so foo(bar) and foo returns (bar) are both possible.
			returns := c.Pop()

			var ty ast.TypeAny
			list, err := p.Punct(c, "(", taxa.KeywordReturns.After())
			if list.Nil() && canStartPath(c.Peek()) {
				// Suppose the user writes `returns my.Response`. This is
				// invalid but reasonable so we want to diagnose it. To do this,
				// we parse a single type w/o parens and diagnose it later.
				ty, _ = parseType(p, c, taxa.KeywordReturns.After(), false)
			} else if err != nil {
				p.Error(err)
				continue
			}

			var withRet report.Span
			if !list.Nil() {
				withRet = report.Join(returns, list)
			} else {
				withRet = report.Join(returns, ty)
			}

			if !outputs.Nil() || !outputTy.Nil() {
				var out report.Spanner = outputs
				if outputs.Nil() {
					out = outputTy
				}

				p.Error(errMoreThanOne{
					first:  report.Join(args.Returns, out),
					second: withRet,
					what:   taxa.MethodOuts,
				})
				continue
			}

			switch {
			case !args.Options.Nil():
				p.Error(errUnexpected{
					what:  withRet,
					where: taxa.CompactOptions.After(),
					prev:  args.Options,
					got:   taxa.MethodOuts,
				})
			case !braces.Nil():
				p.Error(errUnexpected{
					what:  withRet,
					where: taxa.Body.After(),
					prev:  braces,
					got:   taxa.MethodOuts,
				})
			}

			args.Returns = returns
			if !list.Nil() {
				outputs = list
			} else {
				outputTy = ty
			}
			continue

		case "[": // Compact options.
			c.Pop()
			if next.IsLeaf() {
				// Lexer has already diagnosed this for us.
				continue
			}

			if !args.Options.Nil() {
				p.Error(errMoreThanOne{
					first:  args.Options,
					second: next,
					what:   taxa.CompactOptions,
				})
				continue
			}

			if !braces.Nil() {
				p.Error(errUnexpected{
					what:  next,
					where: taxa.Body.After(),
					prev:  braces,
					got:   taxa.CompactOptions,
				})
			}

			args.Options = parseOptions(p, next, taxa.Def)
			continue

		case "{": // Body for the definition.
			if !braces.Nil() {
				// We *do not* pop or diagnose an extra {}; we want this to be
				// parsed as a loose DeclBody, instead, so we stop parsing here.
				break followers
			}

			c.Pop()
			if next.IsLeaf() {
				// Lexer has already diagnosed this for us.
				continue
			}

			braces = next
			continue
		}

		// A value for the definition.
		//
		// This will slurp up a value *not* prefixed with an =, too, but that
		// case needs to be diagnosed. This allows us to diagnose e.g.
		//
		//  optional int32 x 5; // Missing =.
		//
		// However, if we've already seen {}, [], or another value, we break
		// instead, since this suggests we're peeking the next def.
		var eq token.Token
		if next.Text() == "=" {
			eq = c.Pop()
		}
		if eq.Nil() {
			// If the next "expression" looks like a path, this likelier to be
			// due to a missing semicolon than a missing =.
			if canStartPath(next) {
				break
			}

			if !canStartExpr(next) ||
				!braces.Nil() || !args.Options.Nil() || !args.Value.Nil() {
				break
			}
		} else if args.Equals.Nil() {
			args.Equals = eq
		}

		expr := parseExpr(p, c, taxa.Def.In())
		if expr.Nil() {
			continue // parseExpr already generated diagnostics.
		}

		exprIs := taxa.FieldTag
		if kw.Text() == "option" {
			exprIs = taxa.OptionValue
		} else if ty.Nil() {
			exprIs = taxa.EnumValue
		}

		if !args.Value.Nil() {
			p.Error(errMoreThanOne{
				first: args.Value,
				// This join will make it so that the = is included in the
				// "consider removing this" suggestion.
				second: report.Join(eq, expr),
				what:   exprIs,
			})
			continue
		}

		args.Value = expr
		switch {
		case args.Equals.Nil():
			p.Error(errUnexpected{
				what:  expr,
				where: taxa.Equals.Without(),
				prev:  args.Equals,
				got:   exprIs,
			})
		case !args.Options.Nil():
			p.Error(errUnexpected{
				what:  expr,
				where: taxa.CompactOptions.After(),
				prev:  args.Options,
				got:   exprIs,
			})
		case !braces.Nil():
			p.Error(errUnexpected{
				what:  expr,
				where: taxa.Body.After(),
				prev:  braces,
				got:   exprIs,
			})
		}
	}

	// If we didn't see any braces, this def needs to be ended by a semicolon.
	if braces.Nil() {
		semi, err := p.Punct(c, ";", taxa.Def.After())
		args.Semicolon = semi
		if err != nil {
			p.Error(err)
		}
	}

	// Convert something that looks like an enum value into one.
	if tyPath := ty.AsPath(); !tyPath.Nil() && path.Nil() &&
		// The reason for this is because if the user writes `message {}`, we want
		// to *not* turn it into an enum value with a body.
		//
		// TODO: Add a case for making sure that `rpc(foo)` is properly
		// diagnosed as an anonymous method.
		braces.Nil() {
		args.Name = tyPath.Path
		args.Type = ast.TypeAny{}
	}

	def := p.NewDeclDef(args)

	if !inputs.Nil() {
		parseTypeList(p, inputs, def.WithSignature().Inputs(), taxa.MethodIns)
	}
	if !outputs.Nil() {
		parseTypeList(p, outputs, def.WithSignature().Outputs(), taxa.MethodOuts)
	} else if !outputTy.Nil() {
		p.Errorf("missing `(...)` around method return type").With(
			report.Snippetf(outputTy, "help: replace this with `(%s)`", outputTy.Span().Text()),
		)
		def.WithSignature().Outputs().Append(outputTy)
	}

	if !braces.Nil() {
		var in taxa.Subject
		switch kw.Text() {
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

		def.SetBody(parseBody(p, braces, in))
	}

	return def
}

// parseTypeList parses a type list out of a bracket token.
func parseTypeList(p *parser, parens token.Token, types ast.TypeList, in taxa.Subject) {
	tys := commas(p, parens.Children(), true, func(c *token.Cursor) (ast.TypeAny, bool) {
		ty, _ := parseType(p, c, in.In(), false)
		return ty, !ty.Nil()
	})

	tys(func(ty ast.TypeAny, comma token.Token) bool {
		types.AppendComma(ty, comma)
		return true
	})
}

// parseOptions parses a ([]-delimited) compact options list.
func parseOptions(p *parser, brackets token.Token, _ taxa.Subject) ast.CompactOptions {
	options := p.NewCompactOptions(brackets)

	elems := commas(p, brackets.Children(), true, func(c *token.Cursor) (ast.Option, bool) {
		path := parsePath(p, c)
		if path.Nil() {
			return ast.Option{}, false
		}

		eq := c.Peek()
		switch eq.Text() {
		case ":": // Allow colons, which is usually a mistake.
			p.Errorf("unexpected `:` in compact option").With(
				report.Snippetf(eq, "help: replace this with `=`"),
				report.Note("top-level `option` assignment uses `=`, not `:`"),
			)
			fallthrough
		case "=":
			c.Pop()
		default:
			p.Error(errUnexpected{
				what:  eq,
				want:  taxa.Equals.AsSet(),
				where: taxa.CompactOptions.In(),
			})
			eq = token.Nil
		}

		option := ast.Option{
			Path:   path,
			Equals: eq,
			Value:  parseExpr(p, c, taxa.CompactOptions.In()),
		}
		return option, !option.Value.Nil()
	})

	elems(func(opt ast.Option, comma token.Token) bool {
		options.AppendComma(opt, comma)
		return true
	})

	return options
}
