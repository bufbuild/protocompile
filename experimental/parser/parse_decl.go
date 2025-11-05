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
	"slices"

	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/internal/taxa"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/experimental/token/keyword"
	"github.com/bufbuild/protocompile/internal/ext/slicesx"
)

type exprComma struct {
	expr  ast.ExprAny
	comma token.Token
}

func (e exprComma) Span() source.Span {
	return e.expr.Span()
}

// parseDecl parses any Protobuf declaration.
//
// This function will always advance cursor if it is not empty.
func parseDecl(p *parser, c *token.Cursor, in taxa.Noun) ast.DeclAny {
	first := c.Peek()
	if first.IsZero() {
		return ast.DeclAny{}
	}

	var unexpected []token.Token
	for !c.Done() && !canStartDecl(first) {
		unexpected = append(unexpected, c.Next())
		first = c.Peek()
	}
	switch len(unexpected) {
	case 0:
	case 1:
		p.Error(errUnexpected{
			what:  unexpected[0],
			where: in.In(),
			want:  startsDecl,
		})
	case 2:
		p.Error(errUnexpected{
			what:  source.JoinSeq(slices.Values(unexpected)),
			where: in.In(),
			want:  startsDecl,
			got:   "tokens",
		})
	}

	if first.Keyword() == keyword.Semi {
		c.Next()

		// This is an empty decl.
		return p.NewDeclEmpty(first).AsAny()
	}

	// This is a bare declaration body.
	if canStartBody(first) {
		return parseBody(p, c.Next(), in).AsAny()
	}

	// We need to parse a path here. At this point, we need to generate a
	// diagnostic if there is anything else in our way before hitting parsePath.
	if !canStartPath(first) {
		return ast.DeclAny{}
	}

	// Parse a type followed by a path. This is the "most general" prefix of almost all
	// possible productions in a decl. If the type is a TypePath which happens to be
	// a keyword, we try to parse the appropriate thing (with one token of lookahead),
	// and otherwise parse a field.
	mark := c.Mark()
	ty, path := parseTypeAndPath(p, c, in.In())

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
	if path := ty.AsPath(); !path.IsZero() {
		kw = path.AsIdent()
	}

	// Check for the various special cases.
	next := c.Peek()
	switch kw.Keyword() {
	case keyword.Syntax, keyword.Edition:
		// Syntax and edition are parsed only at the top level. Otherwise, they
		// start a def.
		if in != taxa.TopLevel {
			break
		}

		args := ast.DeclSyntaxArgs{
			Keyword: kw,
		}

		in := taxa.Syntax
		if kw.Keyword() == keyword.Edition {
			in = taxa.Edition
		}

		if c.Done() {
			// If we see an EOF at this point, suggestions from the next
			// few stanzas will be garbage.
			p.Error(errUnexpectedEOF(c, in.In()))
		} else {
			eq, err := parseEquals(p, c, in)
			args.Equals = eq
			if err != nil {
				p.Error(err)
			}

			// Regardless of if we see an = sign, try to parse an expression if we
			// can.
			if !args.Equals.IsZero() || canStartExpr(c.Peek()) {
				args.Value = parseExpr(p, c, in.In())
			}

			args.Options = tryParseOptions(p, c, in)

			args.Semicolon, err = parseSemi(p, c, in)
			// Only diagnose a missing semicolon if we successfully parsed some
			// kind of partially-valid expression. Otherwise, we might diagnose
			// the same extraneous/missing ; twice.
			//
			// For example, consider `syntax = ;`. WHen we enter parseExpr, it
			// will complain about the unexpected ;.
			//
			// TODO: Add something like ExprError and check if args.Value
			// contains one.
			if err != nil && !args.Value.IsZero() {
				p.Error(err)
			}
		}

		return p.NewDeclSyntax(args).AsAny()

	case keyword.Package:
		// Package is only parsed only at the top level. Otherwise, it starts
		// a def.
		//
		// TODO: This is not ideal. What we should do instead is to parse a
		// package unconditionally, and if this is not the top level AND
		// the path is an identifier, rewind and reinterpret this as a field,
		// much like we do with ranges in some cases.
		if in != taxa.TopLevel {
			break
		}
		in := taxa.Package

		args := ast.DeclPackageArgs{
			Keyword: kw,
			Path:    path,
		}

		if c.Done() && path.IsZero() {
			// If we see an EOF at this point, suggestions from the next
			// few stanzas will be garbage.
			p.Error(errUnexpectedEOF(c, in.In()))
		} else {
			args.Options = tryParseOptions(p, c, in)

			semi, err := parseSemi(p, c, in)
			args.Semicolon = semi
			if err != nil {
				p.Error(err)
			}
		}

		return p.NewDeclPackage(args).AsAny()

	case keyword.Import:
		// We parse imports inside of any body. However, outside of the top
		// level, we interpret import foo as a field. import foo.bar is still
		// an import, because we want to diagnose what is clearly an attempt to
		// import by path rather than by file.
		//
		// TODO: this treats import public inside of a message as a field, which
		// may result in worse diagnostics.
		if in != taxa.TopLevel &&
			(!path.AsIdent().IsZero() && next.Kind() != token.String) {
			break
		}
		// This is definitely a field.
		if next.Keyword() == keyword.Eq {
			break
		}

		args := ast.DeclImportArgs{
			Keyword: kw,
		}

		in := taxa.Import

		for path.AsIdent().Keyword().IsModifier() {
			args.Modifiers = append(args.Modifiers, path.AsIdent())
			path = ast.Path{}
			if canStartPath(c.Peek()) {
				path = parsePath(p, c)
			}
		}

		if !path.IsZero() {
			// This will catch someone writing `import foo.bar;` when we legalize.
			args.ImportPath = ast.ExprPath{Path: path}.AsAny()
		}

		if args.ImportPath.IsZero() && canStartExpr(next) {
			args.ImportPath = parseExpr(p, c, in.In())
		}

		args.Options = tryParseOptions(p, c, in)

		if args.ImportPath.IsZero() && c.Done() {
			// If we see an EOF at this point, suggestions from the next
			// few stanzas will be garbage.
			p.Error(errUnexpectedEOF(c, in.In()))
		} else {
			semi, err := parseSemi(p, c, in)
			args.Semicolon = semi
			if err != nil {
				p.Error(err)
			}
		}

		return p.NewDeclImport(args).AsAny()

	case keyword.Reserved, keyword.Extensions:
		if next.Keyword() == keyword.Eq {
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

	def := &defParser{
		parser: p,
		c:      c,
		kw:     kw,
		in:     in,
		args:   ast.DeclDefArgs{Type: ty, Name: path},
	}
	return def.parse().AsAny()
}

// parseBody parses a ({}-delimited) body of declarations.
func parseBody(p *parser, braces token.Token, in taxa.Noun) ast.DeclBody {
	body := p.NewDeclBody(braces)

	// Drain the contents of the body into it. Remember,
	// parseDecl must always make progress if there is more to
	// parse.
	c := braces.Children()
	for !c.Done() {
		if next := parseDecl(p, c, in); !next.IsZero() {
			seq.Append(body.Decls(), next)
		}
	}

	return body
}

// parseRange parses a reserved/extensions range.
func parseRange(p *parser, c *token.Cursor) ast.DeclRange {
	// Consume the keyword token.
	kw := c.Next()

	in := taxa.Extensions
	if kw.Keyword() == keyword.Reserved {
		in = taxa.Reserved
	}

	var (
		// badExpr keeps track of whether we exited the loop due to a parse
		// error or because we hit ; or [ or EOF.
		badExpr bool
		exprs   []exprComma
	)

	// Note that this means that we do not parse `reserved [1, 2, 3];`
	// "correctly": that is, as a reserved range whose first expression is an
	// array. Instead, we parse it as an invalid compact options.
	//
	// TODO: This could be mitigated with backtracking: if the compact options
	// is empty, or if the first comma occurs without seeing an =, we can choose
	// to parse this as an array, instead.
	if !canStartOptions(c.Peek()) {
		var last token.Token
		d := delimited[ast.ExprAny]{
			p: p, c: c,
			what: taxa.Expr,
			in:   in,

			required: true,
			exhaust:  false,
			parse: func(c *token.Cursor) (ast.ExprAny, bool) {
				last = c.Peek()
				expr := parseExpr(p, c, in.In())
				badExpr = expr.IsZero()

				return expr, !expr.IsZero()
			},
			start: canStartExpr,
			stop: func(t token.Token) bool {
				if slicesx.Among(t.Keyword(), keyword.Semi, keyword.Brackets) {
					return true
				}

				// After the first element, stop if we see an identifier
				// coming up. This is for a case like this:
				//
				// reserved 1, 2
				// message Foo {}
				//
				// If we don't do this, message will be interpreted as an
				// expression.
				if !last.IsZero() && t.Kind() == token.Ident {
					// However, this will cause
					//
					// reserved foo, bar baz;
					//
					// to treat baz as a new declaration, rather than assume a
					// missing comma. Distinguishing this case is tricky: the
					// cheapest option is to check whether a newline exists between
					// this token and the last position passed to parse.
					//
					// This case will not be hit for valid syntax, so it's ok
					// to do 2*O(log n) line lookups.
					prev := last.Span().EndLoc()
					next := t.Span().StartLoc()
					return prev.Line != next.Line
				}

				return false
			},
		}

		for expr, comma := range d.iter {
			exprs = append(exprs, exprComma{expr, comma})
		}
	}

	options := tryParseOptions(p, c, in)

	// Parse a semicolon, if possible.
	semi, err := parseSemi(p, c, in)
	if err != nil && (!options.IsZero() || !badExpr) {
		p.Error(err)
	}

	r := p.NewDeclRange(ast.DeclRangeArgs{
		Keyword:   kw,
		Options:   options,
		Semicolon: semi,
	})
	for _, e := range exprs {
		r.Ranges().AppendComma(e.expr, e.comma)
	}

	return r
}

// parseTypeList parses a type list out of a bracket token.
func parseTypeList(p *parser, parens token.Token, types ast.TypeList, in taxa.Noun) {
	types.SetBrackets(parens)
	delimited[ast.TypeAny]{
		p:    p,
		c:    parens.Children(),
		what: taxa.Type,
		in:   in,

		required: true,
		exhaust:  true,
		parse: func(c *token.Cursor) (ast.TypeAny, bool) {
			ty := parseType(p, c, in.In())
			return ty, !ty.IsZero()
		},
		start: canStartPath,
	}.appendTo(types)
}

func tryParseOptions(p *parser, c *token.Cursor, in taxa.Noun) ast.CompactOptions {
	if !canStartOptions(c.Peek()) {
		return ast.CompactOptions{}
	}
	return parseOptions(p, c.Next(), in)
}

// parseOptions parses a ([]-delimited) compact options list.
func parseOptions(p *parser, brackets token.Token, _ taxa.Noun) ast.CompactOptions {
	options := p.NewCompactOptions(brackets)

	delimited[ast.Option]{
		p:    p,
		c:    brackets.Children(),
		what: taxa.Option,
		in:   taxa.CompactOptions,

		required: true,
		exhaust:  true,
		parse: func(c *token.Cursor) (ast.Option, bool) {
			path := parsePath(p, c)
			if path.IsZero() {
				return ast.Option{}, false
			}

			eq := c.Peek()
			switch eq.Text() {
			case ":": // Allow colons, which is usually a mistake.
				p.Errorf("unexpected `:` in compact option").Apply(
					report.Snippet(eq),
					justify(p.File().Stream(), eq.Span(), "replace this with an `=`", justified{
						report.Edit{Start: 0, End: 1, Replace: "="},
						justifyBetween,
					}),
					report.Notef("top-level `option` assignment uses `=`, not `:`"),
				)
				fallthrough
			case "=":
				c.Next()
			default:
				p.Error(errUnexpected{
					what:  eq,
					want:  taxa.Equals.AsSet(),
					where: taxa.CompactOptions.In(),
				})
				eq = token.Zero
			}

			option := ast.Option{
				Path:   path,
				Equals: eq,
				Value:  parseExpr(p, c, taxa.CompactOptions.In()),
			}
			return option, !option.Value.IsZero()
		},
		start: canStartPath,
	}.appendTo(options.Entries())

	return options
}
