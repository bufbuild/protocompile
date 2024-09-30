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

package ast

import (
	"github.com/bufbuild/protocompile/experimental/report"
)

// parse implements the core parser loop.
func parse(errs *report.Report, c *Context) {
	cursor := c.Stream()
	file := c.Root()
	var mark CursorMark
	for !cursor.Done() {
		cursor.ensureProgress(mark)
		mark = cursor.Mark()

		next := parseDecl(errs, cursor, "")
		if next != nil {
			file.Append(next)
		}
	}
}

// parseDecl parses any Protobuf declaration.
//
// This function will always advance cursor if it is not empty.
func parseDecl(errs *report.Report, cursor *Cursor, where string) Decl {
	first := cursor.Peek()
	if first.Nil() {
		return nil
	}

	if first.Text() == ";" {
		cursor.Pop()

		// This is an empty decl.
		return cursor.Context().NewDeclEmpty(first)
	}

	if first.Text() == "{" {
		cursor.Pop()
		return parseBody(errs, first, first.Children(), where)
	}

	// We need to parse a path here. At this point, we need to generate a
	// diagnostic if there is anything else in our way before hitting parsePath.
	if !canStartPath(first) {
		// Consume the token, emit a diagnostic, and throw it away.
		cursor.Pop()
		var whereStr string
		if where == "" {
			whereStr = "in file scope"
		} else {
			whereStr = "in `" + where + "`"
		}

		errs.Error(errUnexpected{
			node:  first,
			where: whereStr,
			want:  []string{"identifier", "`.`", "`;`", "`(...)`", "`{...}`"},
		})
		return nil
	}

	// Parse a type followed by a path. This is the "most general" prefix of almost all
	// possible productions in a decl. If the type is a TypePath which happens to be
	// a keyword, we try to parse the appropriate thing (with one token of lookahead),
	// and otherwise parse a field.
	ty, path := parseType(errs, cursor)

	var kw Token
	if path, ok := ty.(TypePath); ok {
		kw = path.AsIdent()
	}

	type exprComma struct {
		expr  Expr
		comma Token
	}

	// Check for the various special cases.
	next := cursor.Peek()
	switch kw.Text() {
	case "syntax", "edition":
		if where == "enum" || !path.Nil() {
			// Inside of an enum, fields without types are valid, and that is ambiguous with
			// a syntax node.
			break
		}

		eq := parsePunct(errs, cursor, punctArgs{
			want:  "=",
			where: "in " + kw.Text() + " declaration",
		})

		value := parseExpr(errs, cursor)
		// Only diagnose a missing semicolon if we successfully parsed some
		// kind of partially-valid expression. Otherwise, we might diagnose
		// the same extraneous ; twice.
		semi := parsePunct(errs, cursor, punctArgs{
			want:           ";",
			where:          "in " + kw.Text() + " declaration",
			diagnoseUnless: value == nil,
		})

		return cursor.Context().NewDeclSyntax(DeclSyntaxArgs{
			Keyword:   kw,
			Equals:    eq,
			Value:     value,
			Semicolon: semi,
		})

	case "package":
		if next.Text() != ";" {
			// If it's not followed by a semi, let it be diagnosed as a field.
			break
		}

		cursor.Pop()
		return cursor.Context().NewDeclPackage(DeclPackageArgs{
			Keyword:   kw,
			Path:      path,
			Semicolon: next,
		})

	case "import":
		modifier := path.AsIdent().Name()
		if !path.Nil() && modifier != "public" && modifier != "weak" {
			// For import to be valid, it needs to either be alone or the path
			// must be public or weak.
			break
		}

		var filePath Token
		if next := cursor.Peek(); next.Kind() == TokenString {
			filePath = next
			cursor.Pop()
		} else {
			// If it's not followed by a quoted string, let it be diagnosed as a field.
			break
		}

		// Check for a trailing semicolon.
		semi := parsePunct(errs, cursor, punctArgs{
			want:           ";",
			diagnoseUnless: filePath.Nil(),
			where:          "in import",
		})

		return cursor.Context().NewDeclImport(DeclImportArgs{
			Keyword:   kw,
			Modifier:  path.AsIdent(),
			FilePath:  filePath,
			Semicolon: semi,
		})

	case "reserved", "extensions":
		if next.Text() == "=" {
			// If whatever follows the path is an =, we're going to assume this is
			// meant to be a field.
			break
		}

		var (
			done, bad bool
			exprs     []exprComma
		)

		// Convert the trailing path, if there is any, into an expression, and check for the
		// first comma.
		if !path.Nil() {
			expr := parseOperator(errs, cursor, ExprPath{Path: path}, 0)
			var comma Token
			if next := cursor.Peek(); next.Text() == "," {
				comma = cursor.Pop()
			} else {
				done = true
			}
			exprs = append(exprs, exprComma{expr, comma})
			bad = bad || exprs == nil
		}

		if !done {
			// Parse expressions until we hit a semicolon or the [ of compact options.
			delimited := commaDelimited(true, errs, cursor, func(cursor *Cursor) (Expr, bool) {
				next := cursor.Peek().Text()
				if next == ";" || next == "[" {
					return nil, false
				}
				expr := parseExpr(errs, cursor)
				bad = bad || exprs == nil
				return expr, expr != nil
			})

			delimited(func(expr Expr, comma Token) bool {
				exprs = append(exprs, exprComma{expr, comma})
				return true
			})
		}

		var options Token
		if next := cursor.Peek(); next.Text() == "[" {
			options = cursor.Pop()
		}

		// Parse a semicolon, if possible.
		semi := parsePunct(errs, cursor, punctArgs{
			want:           ";",
			where:          "in `" + kw.Text() + "` range",
			diagnoseUnless: bad,
		})

		range_ := cursor.Context().NewDeclRange(DeclRangeArgs{
			Keyword:   kw,
			Options:   options,
			Semicolon: semi,
		})
		for _, e := range exprs {
			range_.AppendComma(e.expr, e.comma)
		}

		if !options.Nil() {
			parseOptions(errs, options, range_.Options())
		}

		return range_
	}

	args := DeclDefArgs{
		Type: ty,
		Name: path,
	}

	var inputs, outputs, braces Token

	// Try to parse the various "followers".
	var mark CursorMark
	for !cursor.Done() {
		cursor.ensureProgress(mark)
		mark = cursor.Mark()

		next := cursor.Peek()
		if next.Text() == "(" {
			cursor.Pop()
			if !inputs.Nil() {
				errs.Error(ErrMoreThanOne{
					First:  inputs,
					Second: next,
					what:   "method input parameter list",
				})
			} else {
				inputs = next
			}
			continue
		}

		if next.Text() == "returns" {
			args.Returns = cursor.Pop()
			next := parsePunct(errs, cursor, punctArgs{
				want:  "(...)",
				where: "after `returns`",
			})

			if !outputs.Nil() && !next.Nil() {
				errs.Error(ErrMoreThanOne{
					First:  outputs,
					Second: next,
					what:   "method input parameter list",
				})
			} else {
				outputs = next
			}
			continue
		}

		if next.Text() == "[" {
			cursor.Pop()
			if !args.Options.Nil() {
				errs.Error(ErrMoreThanOne{
					First:  args.Options,
					Second: next,
					what:   "compact options list",
				})
			} else {
				args.Options = next
			}
		}

		if next.Text() == "{" {
			cursor.Pop()
			if !braces.Nil() {
				errs.Error(ErrMoreThanOne{
					First:  args.Options,
					Second: next,
					what:   "definition body",
				})
			} else {
				braces = next
			}
		}

		// This will slurp up a value *not* prefixed with an =, too, but that
		// case will be diagnosed.
		isEq := next.Text() == "="
		if isEq || (canStartExpr(next) && braces.Nil() && args.Options.Nil()) {
			if isEq {
				args.Equals = cursor.Pop()
			}

			next := parseExpr(errs, cursor)
			if next == nil || next.Context() == nil {
				continue // parseExpr generates diagnostics for this case.
			}

			if args.Value != nil {
				what := "field tag"
				if kw.Text() == "option" {
					what = "option value"
				} else if ty == nil {
					what = "enum value"
				}

				errs.Error(ErrMoreThanOne{
					First:  args.Value,
					Second: next,
					what:   what,
				})
			} else if args.Equals.Nil() {
				errs.Error(errUnexpected{
					node:  next,
					where: "without leading `=`",
					got:   "expression",
				})
			}

			continue
		}

		break
	}

	if braces.Nil() {
		args.Semicolon = parsePunct(errs, cursor, punctArgs{
			want:  ";",
			where: "after declaration",
		})
	}

	parseTypes := func(parens Token, types TypeList) {
		delimited := commaDelimited(true, errs, parens.Children(), func(cursor *Cursor) (Type, bool) {
			ty, path := parseType(errs, cursor)
			if !path.Nil() {
				errs.Error(errUnexpected{
					node:  path,
					where: "in method parameter list",
					got:   "path",
				})
			}
			return ty, ty != nil
		})

		delimited(func(ty Type, comma Token) bool {
			types.AppendComma(ty, comma)
			return true
		})
	}

	def := cursor.Context().NewDeclDef(args)
	if !inputs.Nil() {
		parseTypes(inputs, def.WithSignature().Inputs())
	}
	if !outputs.Nil() {
		parseTypes(outputs, def.WithSignature().Outputs())
	}
	if !args.Options.Nil() {
		parseOptions(errs, args.Options, def.WithOptions())
	}
	if !braces.Nil() {
		where := where
		switch kw.Text() {
		case "message", "enum", "service", "extend", "group", "oneof", "rpc":
			where = kw.Text()
		}

		def.SetBody(parseBody(errs, braces, braces.Children(), where))
	}

	return def
}

// parseBody parses an (optionally-{}-delimited) body of declarations.
func parseBody(errs *report.Report, token Token, contents *Cursor, where string) DeclBody {
	body := contents.Context().NewDeclBody(token)

	// Drain the contents of the body into it. Remember,
	// parseDecl must always make progress if there is more to
	// parse.
	for !contents.Done() {
		if next := parseDecl(errs, contents, where); next != nil {
			body.Append(next)
		}
	}

	return body
}

// parseOptions parses a compact options list out of a [] token.
func parseOptions(errs *report.Report, brackets Token, options Options) Options {
	cursor := brackets.Children()
	delimited := commaDelimited(true, errs, cursor, func(cursor *Cursor) (Option, bool) {
		path := parsePath(errs, cursor)
		if path.Nil() {
			return Option{}, false
		}

		eq := cursor.Peek()
		if eq.Text() == "=" {
			cursor.Pop()
		} else {
			errs.Error(errUnexpected{
				node: eq,
				want: []string{"`=`"},
			})
			eq = Token{}
		}

		expr := parseExpr(errs, cursor)
		if expr == nil {
			return Option{}, false
		}

		return Option{path, eq, expr}, true
	})

	delimited(func(opt Option, comma Token) bool {
		options.AppendComma(opt, comma)
		return true
	})

	return options
}

// canStartExpr returns whether or not tok can start an expression.
func canStartExpr(tok Token) bool {
	return canStartPath(tok) || tok.Kind() == TokenNumber || tok.Kind() == TokenString ||
		tok.Text() == "-" || tok.Text() == "{" || tok.Text() == "["
}

// parseExpr attempts to parse a full expression.
//
// May return nil if parsing completely fails.
func parseExpr(errs *report.Report, cursor *Cursor) Expr {
	return parseOperator(errs, cursor, nil, 0)
}

// parseOperator parses an operator expression (i.e., an expression that consists of more than
// one sub-expression).
//
// prec is the precedence; higher values mean tighter binding. This function calls itself
// with higher (or equal) precedence values.
func parseOperator(errs *report.Report, cursor *Cursor, expr Expr, prec int) Expr {
	if expr == nil {
		expr = parseAtomicExpr(errs, cursor)
	}
	if expr == nil {
		return nil
	}

	lookahead := cursor.Peek()
	switch prec {
	case 0:
		switch lookahead.Text() {
		case ":", "=": // Allow equals signs, which are usually a mistake.
			expr = cursor.Context().NewExprKV(ExprKVArgs{
				Key:   expr,
				Colon: cursor.Pop(),
				Value: parseOperator(errs, cursor, nil, prec+1),
			})
		case "{": // This is for colon-less, dict-values fields.
			// The previous expression cannot also be a key-value pair, since
			// this messes with parsing of dicts, which are not comma-separated.
			if _, isKV := expr.(ExprKV); !isKV {
				expr = cursor.Context().NewExprKV(ExprKVArgs{
					Key:   expr,
					Value: parseOperator(errs, cursor, nil, prec+1),
				})
			}
		}
	case 1:
		switch lookahead.Text() {
		case "to":
			expr = cursor.Context().NewExprRange(ExprRangeArgs{
				Start: expr,
				To:    cursor.Pop(),
				End:   parseOperator(errs, cursor, nil, prec),
			})
		}
	}

	return expr
}

// ParseExpr attempts to parse an "atomic" expression, which is an expression that does not
// contain any infix operators.
//
// May return nil if parsing completely fails.
func parseAtomicExpr(errs *report.Report, cursor *Cursor) (expr Expr) {
	next := cursor.Peek()
	if next.Nil() {
		return nil
	}

	switch {
	case next.Kind() == TokenString, next.Kind() == TokenNumber:
		expr = ExprLiteral{Token: cursor.Pop()}

	case canStartPath(next):
		expr = ExprPath{Path: parsePath(errs, cursor)}

	case next.Text() == "[":
		brackets := cursor.Pop()
		delimited := commaDelimited(true, errs, brackets.Children(), func(cursor *Cursor) (Expr, bool) {
			expr := parseExpr(errs, cursor)
			return expr, expr != nil
		})

		array := cursor.Context().NewExprArray(brackets)
		delimited(func(expr Expr, comma Token) bool {
			array.AppendComma(expr, comma)
			return true
		})

	case next.Text() == "{":
		cursor.Pop()
		delimited := commaDelimited(false, errs, next.Children(), func(cursor *Cursor) (Expr, bool) {
			expr := parseExpr(errs, cursor)
			return expr, expr != nil
		})

		dict := cursor.Context().NewExprDict(next)
		delimited(func(expr Expr, comma Token) bool {
			field, ok := expr.(ExprKV)
			if !ok {
				errs.Error(errUnexpected{
					node: expr,
					got:  "expression",
					want: []string{"key-value pair"},
				})

				field = cursor.Context().NewExprKV(ExprKVArgs{Value: expr})
			}
			dict.AppendComma(field, comma)
			return true
		})

	case next.Text() == "-":
		// NOTE: Protobuf does not (currently) have any suffix expressions, like a function
		// call, but if those are added, this will need to be hoisted into a parsePrefixExpr
		// function that calls parseAtomicExpr.
		cursor.Pop()
		inner := parseAtomicExpr(errs, cursor)
		expr = cursor.Context().NewExprPrefixed(ExprPrefixedArgs{
			Prefix: next,
			Expr:   inner,
		})

	default:
		// Consume the token and diagnose it.
		cursor.Pop()
		errs.Error(errUnexpected{
			node: next,
			want: []string{
				"identifier", "number", "string", "`.`", "`-`", "`(...)`", "`[...]`", "`{...}`",
			},
		})
	}

	return expr
}

// parseType attempts to parse a type, optionally followed by a non-absolute path.
//
// This function is called in many situations that seem a bit weird to be parsing a type
// in, such as at the top level. This is because of an essential ambiguity in Protobuf's
// grammar: message Foo can start either a field (message Foo;) or a message (message Foo {}).
// Thus, we always parts a type-and-path, and based on what comes next, reinterpret the type
// as potentially being a keyword.
//
// This function assumes that we have decided to definitely parse a type, and
// will emit diagnostics to that effect. As such, the current token position on cursor
// should not be nil.
//
// May return nil if parsing completely fails.
func parseType(errs *report.Report, cursor *Cursor) (Type, Path) {
	// First, parse a path, possibly preceded by a sequence of modifiers.
	var (
		mods   []Token
		tyPath Path
	)
	for !cursor.Done() && tyPath.Nil() {
		next := cursor.Peek()
		if !canStartPath(next) {
			break
		}

		tyPath = parsePath(errs, cursor)

		// Determine if this path is a modifier followed by a path.
		if tyPath.Absolute() {
			// Absolute paths cannot start with a modifier, so we are done.
			break
		}

		// Peel off the first two path components.
		var components []PathComponent
		tyPath.Components(func(component PathComponent) bool {
			components = append(components, component)
			return len(components) < 2
		})

		ident := components[0].AsIdent()
		if ident.Nil() {
			// If this starts with an extension, we're also done.
			break
		}

		// Check if ident is a modifier, and if so, peel it off.
		if mod := TypePrefixByName(ident.Name()); mod != TypePrefixUnknown {
			mods = append(mods, ident)

			// Drop the first component from the path.
			if len(components) == 1 {
				tyPath = Path{}
			} else if !components[1].Separator().Nil() {
				tyPath.raw[0] = components[1].Separator().raw
			} else {
				tyPath.raw[0] = components[1].Name().raw
			}
		}
	}

	if tyPath.Nil() {
		if len(mods) == 0 {
			return nil, Path{}
		}

		// Pop the last mod and make that into the path. This makes `optional optional` work
		// as a type.
		last := mods[len(mods)-1]
		tyPath = rawPath{last.raw, last.raw}.With(cursor)
		mods = mods[:len(mods)-1]
	}

	ty := Type(TypePath{tyPath})

	// Next, look for some angle brackets. We need to do this before draining `mods`, because
	// angle brackets bind more tightly than modifiers.
	next := cursor.Peek()
	if next.Text() == "<" {
		cursor.Pop() // Consume the angle brackets.
		generic := cursor.Context().NewTypeGeneric(TypeGenericArgs{
			Path:          tyPath,
			AngleBrackets: next,
		})

		delimited := commaDelimited(true, errs, next.Children(), func(cursor *Cursor) (Type, bool) {
			if next := cursor.Peek(); !canStartPath(next) {
				errs.Error(errUnexpected{
					node: next,
					want: []string{"identifier", ".", "(...)"},
				})
				return nil, false
			}

			ty, path := parseType(errs, cursor)
			if !path.Nil() {
				errs.Error(errUnexpected{
					node:  path,
					where: "in type argument list",
					got:   "field name",
				})
			}
			return ty, ty != nil
		})

		delimited(func(ty Type, comma Token) bool {
			generic.Args().AppendComma(ty, comma)
			return true
		})
	}

	// Now, check for a path that follows all this. If there isn't a path, and
	// ty is TypePath, and there is still at least one modifier, we interpret the
	// last modifier as the type and the current path type as the path after the type.
	var path Path
	next = cursor.Peek()
	if canStartPath(next) {
		path = parsePath(errs, cursor)
	} else if _, ok := ty.(TypePath); ok && len(mods) > 0 {
		path = tyPath

		// Pop the last mod and make that into the type. This makes `optional optional = 1` work
		// as a field.
		last := mods[len(mods)-1]
		tyPath = rawPath{last.raw, last.raw}.With(cursor)
		mods = mods[:len(mods)-1]
		ty = TypePath{tyPath}
	}

	// Finally, apply any remaining modifiers, in reverse order, to ty.
	for i := len(mods) - 1; i >= 0; i-- {
		ty = cursor.Context().NewTypePrefixed(TypePrefixedArgs{
			Prefix: mods[i],
			Type:   ty,
		})
	}

	return ty, path
}

// commaDelimited returns an iterator over a comma-delimited list of things out of cursor.
// This automatically handles various corner-cases around commas that occur throughout the
// grammar.
//
// This will completely drain cursor, unless the parse function returns false, which signals
// that the end of the list has been reached.
func commaDelimited[T any](
	commasRequired bool,
	errs *report.Report,
	cursor *Cursor,
	parse func(*Cursor) (T, bool),
) func(func(T, Token) bool) {
	return func(yield func(T, Token) bool) {
		for !cursor.Done() {
			result, ok := parse(cursor)

			// Check for a trailing comma.
			var comma Token
			if next := cursor.Peek(); next.Text() == "," {
				cursor.Pop()
				comma = next
			}

			if !ok || !yield(result, comma) {
				break
			}

			if commasRequired && comma.Nil() {
				if next := cursor.Peek(); !next.Nil() {
					errs.Error(errUnexpected{
						node: next,
						want: []string{"`,`"},
					})
				}
				break
			}
		}
	}
}

// canStartPath returns whether or not tok can start a path.
func canStartPath(tok Token) bool {
	return tok.Kind() == TokenIdent || tok.Text() == "." || tok.Text() == "/" || tok.Text() == "("
}

// parsePath parses the longest path at cursor. Returns a nil path if
// the next token is neither an identifier, a dot, or a ().
//
// If an invalid token occurs after a dot, returns the longest path up until that dot.
// The cursor is then placed after the dot.
//
// This function assumes that we have decided to definitely parse a path, and
// will emit diagnostics to that effect. As such, the current token position on cursor
// should not be nil.
func parsePath(errs *report.Report, cursor *Cursor) Path {
	start := cursor.Peek()
	if !canStartPath(start) {
		errs.Error(errUnexpected{
			node: start,
			want: []string{"identifier", "`.`", "`(...)`"},
		})
		return Path{}
	}

	// Whether the next unskippable token should be a separator.
	var prevSeparator Token
	if start.Text() == "." || start.Text() == "/" {
		prevSeparator = cursor.Pop()
	}
	end := start
pathLoop:
	for !cursor.Done() {
		next := cursor.Peek()
		first := start == next
		switch {
		case next.Text() == "." || next.Text() == "/":
			if !prevSeparator.Nil() {
				// This is a double dot, so something like foo..bar, ..foo, or foo..
				// We diagnose it and move on -- Path.Components is robust against
				// this kind of pattern.
				errs.Error(errUnexpected{
					node:  next,
					where: "after `" + prevSeparator.Text() + "`",
					want:  []string{"identifier", "`(...)`"},
				})
			}
			prevSeparator = cursor.Pop()

		case next.Kind() == TokenIdent:
			if !first && prevSeparator.Nil() {
				// This means we found something like `foo bar`, which means we
				// should stop consuming components.
				break pathLoop
			}

			end = next
			prevSeparator = Token{}
			cursor.Pop()

		case next.Text() == "(":
			if !first && prevSeparator.Nil() {
				// This means we found something like `foo(bar)`, which means we
				// should stop consuming components.
				break pathLoop
			}

			// Recurse into this token and check it, too, contains a path. We throw
			// the result away once we're done. We also need to check there are no
			// extraneous tokens.
			contents := next.Children()
			parsePath(errs, contents)
			if tok := contents.Peek(); !tok.Nil() {
				errs.Error(errUnexpected{
					node:  start,
					where: "in extension path",
				})
			}

			end = next
			prevSeparator = Token{}
			cursor.Pop()

		default:
			if prevSeparator.Nil() {
				// This means we found something like `foo =`, which means we
				// should stop consuming components.
				break pathLoop
			}

			// This means we found something like foo.1 or bar."xyz" or bar.[...].
			// TODO: Do smarter recovery here. Generally speaking it's likely we should *not*
			// consume this token.
			errs.Error(errUnexpected{
				node: next,
				want: []string{"identifier", "`(...)`"},
			})

			end = prevSeparator // Include the trailing separator.
			break pathLoop
		}
	}

	// NOTE: We do not need to legalize against a single-dot path; that
	// is already done for us by the if nextDot checks.

	return rawPath{start.raw, end.raw}.With(cursor)
}

type punctArgs struct {
	want           string
	diagnoseUnless bool
	where          string
}

// parsePunct attempts to unconditionally parse some punctuation.
//
// If the wrong token is encountered, it DOES NOT consume the token, returning a nil
// token instead. If diagnose is true, this will diagnose the problem.
func parsePunct(errs *report.Report, cursor *Cursor, args punctArgs) Token {
	next := cursor.Peek()
	if next.Text() == args.want {
		return cursor.Pop()
	}
	if !args.diagnoseUnless {
		errs.Error(errUnexpected{
			node:  next,
			where: args.where,
			want:  []string{"`" + args.want + "`"},
		})
	}
	return Token{}
}
