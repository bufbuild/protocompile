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

// NewDeclEmpty creates a new DeclEmpty node.
func (c *Context) NewDeclEmpty(semicolon Token) DeclEmpty {
	c.panicIfNotOurs(semicolon)

	decl := wrapDeclEmpty(c, c.decls.empties.NewCompressed(rawDeclEmpty{
		semi: semicolon.raw,
	}))

	return decl
}

// NewDeclSyntax creates a new DeclSyntax node.
func (c *Context) NewDeclSyntax(args DeclSyntaxArgs) DeclSyntax {
	c.panicIfNotOurs(args.Keyword, args.Equals, args.Value, args.Options, args.Semicolon)

	return wrapDeclSyntax(c, c.decls.syntaxes.NewCompressed(rawDeclSyntax{
		keyword: args.Keyword.raw,
		equals:  args.Equals.raw,
		value:   args.Value.raw,
		options: c.options.Compress(args.Options.raw),
		semi:    args.Semicolon.raw,
	}))
}

// NewDeclPackage creates a new DeclPackage node.
func (c *Context) NewDeclPackage(args DeclPackageArgs) DeclPackage {
	c.panicIfNotOurs(args.Keyword, args.Path, args.Options, args.Semicolon)

	return wrapDeclPackage(c, c.decls.packages.NewCompressed(rawDeclPackage{
		keyword: args.Keyword.raw,
		path:    args.Path.raw,
		options: c.options.Compress(args.Options.raw),
		semi:    args.Semicolon.raw,
	}))
}

// NewDeclImport creates a new DeclImport node.
func (c *Context) NewDeclImport(args DeclImportArgs) DeclImport {
	c.panicIfNotOurs(args.Keyword, args.Modifier, args.ImportPath, args.Options, args.Semicolon)

	return wrapDeclImport(c, c.decls.imports.NewCompressed(rawDeclImport{
		keyword:    args.Keyword.raw,
		modifier:   args.Modifier.raw,
		importPath: args.ImportPath.raw,
		options:    c.options.Compress(args.Options.raw),
		semi:       args.Semicolon.raw,
	}))
}

// NewDeclDef creates a new DeclDef node.
func (c *Context) NewDeclDef(args DeclDefArgs) DeclDef {
	c.panicIfNotOurs(
		args.Keyword, args.Type, args.Name, args.Returns,
		args.Equals, args.Value, args.Options, args.Body, args.Semicolon)

	raw := rawDeclDef{
		name:    args.Name.raw,
		equals:  args.Equals.raw,
		value:   args.Value.raw,
		options: c.options.Compress(args.Options.raw),
		body:    c.decls.bodies.Compress(args.Body.raw),
		semi:    args.Semicolon.raw,
	}
	if !args.Type.Nil() {
		raw.ty = args.Type.raw
	} else {
		raw.ty = rawType(rawPath{args.Keyword.raw, args.Keyword.raw})
	}
	if !args.Returns.Nil() {
		raw.signature = &rawSignature{
			returns: args.Returns.raw,
		}
	}

	return wrapDeclDef(c, c.decls.defs.NewCompressed(raw))
}

// NewDeclBody creates a new DeclBody node.
//
// To add declarations to the returned body, use [DeclBody.Append].
func (c *Context) NewDeclBody(braces Token) DeclBody {
	c.panicIfNotOurs(braces)

	return wrapDeclBody(c, c.decls.bodies.NewCompressed(rawDeclBody{
		braces: braces.raw,
	}))
}

// NewDeclRange creates a new DeclRange node.
//
// To add ranges to the returned declaration, use [DeclRange.Append].
func (c *Context) NewDeclRange(args DeclRangeArgs) DeclRange {
	c.panicIfNotOurs(args.Keyword, args.Options, args.Semicolon)

	return wrapDeclRange(c, c.decls.ranges.NewCompressed(rawDeclRange{
		keyword: args.Keyword.raw,
		options: c.options.Compress(args.Options.raw),
		semi:    args.Semicolon.raw,
	}))
}

// NewExprPrefixed creates a new ExprPrefixed node.
func (c *Context) NewExprPrefixed(args ExprPrefixedArgs) ExprPrefixed {
	c.panicIfNotOurs(args.Prefix, args.Expr)

	return ExprPrefixed{exprImpl[rawExprPrefixed]{
		withContext{c},
		c.exprs.prefixes.New(rawExprPrefixed{
			prefix: args.Prefix.raw,
			expr:   args.Expr.raw,
		}),
	}}
}

// NewExprRange creates a new ExprRange node.
func (c *Context) NewExprRange(args ExprRangeArgs) ExprRange {
	c.panicIfNotOurs(args.Start, args.To, args.End)

	return ExprRange{exprImpl[rawExprRange]{
		withContext{c},
		c.exprs.ranges.New(rawExprRange{
			to:    args.To.raw,
			start: args.Start.raw,
			end:   args.End.raw,
		}),
	}}
}

// NewExprArray creates a new ExprArray node.
//
// To add elements to the returned expression, use [ExprArray.Append].
func (c *Context) NewExprArray(brackets Token) ExprArray {
	c.panicIfNotOurs(brackets)

	return ExprArray{exprImpl[rawExprArray]{
		withContext{c},
		c.exprs.arrays.New(rawExprArray{
			brackets: brackets.raw,
		}),
	}}
}

// NewExprDict creates a new ExprDict node.
//
// To add elements to the returned expression, use [ExprDict.Append].
func (c *Context) NewExprDict(braces Token) ExprDict {
	c.panicIfNotOurs(braces)

	return ExprDict{exprImpl[rawExprDict]{
		withContext{c},
		c.exprs.dicts.New(rawExprDict{
			braces: braces.raw,
		}),
	}}
}

// NewExprPrefixed creates a new ExprPrefixed node.
func (c *Context) NewExprKV(args ExprKVArgs) ExprField {
	c.panicIfNotOurs(args.Key, args.Colon, args.Value)

	return ExprField{exprImpl[rawExprField]{
		withContext{c},
		c.exprs.fields.New(rawExprField{
			key:   args.Key.raw,
			colon: args.Colon.raw,
			value: args.Value.raw,
		}),
	}}
}

// NewTypePrefixed creates a new TypePrefixed node.
func (c *Context) NewTypePrefixed(args TypePrefixedArgs) TypePrefixed {
	c.panicIfNotOurs(args.Prefix, args.Type)

	return TypePrefixed{typeImpl[rawTypePrefixed]{
		withContext{c},
		c.types.prefixes.New(rawTypePrefixed{
			prefix: args.Prefix.raw,
			ty:     args.Type.raw,
		}),
	}}
}

// NewTypeGeneric creates a new TypeGeneric node.
//
// To add arguments to the returned type, use [TypeGeneric.Append].
func (c *Context) NewTypeGeneric(args TypeGenericArgs) TypeGeneric {
	c.panicIfNotOurs(args.Path, args.AngleBrackets)

	return TypeGeneric{typeImpl[rawTypeGeneric]{
		withContext{c},
		c.types.generics.New(rawTypeGeneric{
			path: args.Path.raw,
			args: rawTypeList{brackets: args.AngleBrackets.raw},
		}),
	}}
}

// NewCompactOptions creates a new CompactOptions node.
func (c *Context) NewCompactOptions(brackets Token) CompactOptions {
	c.panicIfNotOurs(brackets)

	return wrapOptions(c, c.options.NewCompressed(rawCompactOptions{
		brackets: brackets.raw,
	}))
}
