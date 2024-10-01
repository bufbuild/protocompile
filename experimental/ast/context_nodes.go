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

	c.decls.empties.Append(rawDeclEmpty{semi: semicolon.raw})
	return decl[DeclEmpty](c.decls.empties.Len()).With(c)
}

// NewDeclSyntax creates a new DeclPragma node.
func (c *Context) NewDeclSyntax(args DeclSyntaxArgs) DeclSyntax {
	c.panicIfNotOurs(args.Keyword, args.Equals, args.Value, args.Options, args.Semicolon)

	c.decls.syntaxes.Append(rawDeclSyntax{
		keyword: args.Keyword.raw,
		equals:  args.Equals.raw,
		options: args.Options.rawOptions(),
		semi:    args.Semicolon.raw,
	})

	decl := decl[DeclSyntax](c.decls.syntaxes.Len()).With(c)
	decl.SetValue(args.Value)

	return decl
}

// NewDeclPackage creates a new DeclPackage node.
func (c *Context) NewDeclPackage(args DeclPackageArgs) DeclPackage {
	c.panicIfNotOurs(args.Keyword, args.Path, args.Options, args.Semicolon)

	c.decls.packages.Append(rawDeclPackage{
		keyword: args.Keyword.raw,
		path:    args.Path.raw,
		options: args.Options.rawOptions(),
		semi:    args.Semicolon.raw,
	})
	return decl[DeclPackage](c.decls.packages.Len()).With(c)
}

// NewDeclImport creates a new DeclImport node.
func (c *Context) NewDeclImport(args DeclImportArgs) DeclImport {
	c.panicIfNotOurs(args.Keyword, args.Modifier, args.ImportPath, args.Options, args.Semicolon)

	c.decls.imports.Append(rawDeclImport{
		keyword:  args.Keyword.raw,
		modifier: args.Modifier.raw,
		options:  args.Options.rawOptions(),
		semi:     args.Semicolon.raw,
	})
	return decl[DeclImport](c.decls.imports.Len()).With(c)
}

// NewDeclDef creates a new DeclDef node.
func (c *Context) NewDeclDef(args DeclDefArgs) DeclDef {
	c.panicIfNotOurs(
		args.Keyword, args.Type, args.Name, args.Returns,
		args.Equals, args.Value, args.Options, args.Body, args.Semicolon)

	c.decls.defs.Append(rawDeclDef{
		name:    args.Name.raw,
		equals:  args.Equals.raw,
		options: args.Options.rawOptions(),
		semi:    args.Semicolon.raw,
	})
	decl := decl[DeclDef](c.decls.defs.Len()).With(c)

	if args.Type != nil {
		decl.SetType(args.Type)
	} else {
		decl.SetType(TypePath{Path: rawPath{args.Keyword.raw, args.Keyword.raw}.With(c)})
	}

	if !args.Returns.Nil() {
		decl.raw.signature = &rawSignature{
			returns: args.Returns.raw,
		}
	}

	decl.SetValue(args.Value)
	decl.SetBody(args.Body)

	return decl
}

// NewDeclBody creates a new DeclBody node
func (c *Context) NewDeclBody(braces Token) DeclBody {
	c.panicIfNotOurs(braces)

	c.decls.bodies.Append(rawDeclBody{braces: braces.raw})
	return decl[DeclBody](c.decls.bodies.Len()).With(c)
}

// NewDeclRange creates a new DeclRange node.
func (c *Context) NewDeclRange(args DeclRangeArgs) DeclRange {
	c.panicIfNotOurs(args.Keyword, args.Options, args.Semicolon)

	c.decls.ranges.Append(rawDeclRange{
		keyword: args.Keyword.raw,
		options: args.Options.rawOptions(),
		semi:    args.Semicolon.raw,
	})
	decl := decl[DeclRange](c.decls.ranges.Len()).With(c)

	return decl
}

// NewExprPrefixed creates a new ExprPrefixed node.
func (c *Context) NewExprPrefixed(args ExprPrefixedArgs) ExprPrefixed {
	c.panicIfNotOurs(args.Prefix, args.Expr)

	raw := c.exprs.prefixes.Append(rawExprPrefixed{
		prefix: args.Prefix.raw,
	})
	expr := ExprPrefixed{
		withContext: withContext{c},
		idx:         c.exprs.prefixes.Len() - 1,
		raw:         raw,
	}
	expr.SetExpr(args.Expr)
	return expr
}

// NewExprRange creates a new ExprRange node.
func (c *Context) NewExprRange(args ExprRangeArgs) ExprRange {
	c.panicIfNotOurs(args.Start, args.To, args.End)

	raw := c.exprs.ranges.Append(rawExprRange{
		to: args.To.raw,
	})
	expr := ExprRange{
		withContext: withContext{c},
		idx:         c.exprs.ranges.Len() - 1,
		raw:         raw,
	}
	expr.SetBounds(args.Start, args.End)
	return expr
}

// NewExprArray creates a new ExprArray node.
func (c *Context) NewExprArray(brackets Token) ExprArray {
	c.panicIfNotOurs(brackets)

	raw := c.exprs.arrays.Append(rawExprArray{
		brackets: brackets.raw,
	})
	return ExprArray{
		withContext: withContext{c},
		idx:         c.exprs.arrays.Len() - 1,
		raw:         raw,
	}
}

// NewExprDict creates a new ExprDict node.
func (c *Context) NewExprDict(braces Token) ExprDict {
	c.panicIfNotOurs(braces)

	raw := c.exprs.dicts.Append(rawExprDict{
		braces: braces.raw,
	})
	return ExprDict{
		withContext: withContext{c},
		idx:         c.exprs.dicts.Len() - 1,
		raw:         raw,
	}
}

// NewExprPrefixed creates a new ExprPrefixed node.
func (c *Context) NewExprKV(args ExprKVArgs) ExprKV {
	c.panicIfNotOurs(args.Key, args.Colon, args.Value)

	raw := c.exprs.fields.Append(rawExprKV{
		colon: args.Colon.raw,
	})
	expr := ExprKV{
		withContext: withContext{c},
		idx:         c.exprs.fields.Len() - 1,
		raw:         raw,
	}
	expr.SetKey(args.Key)
	expr.SetValue(args.Value)
	return expr
}

// NewTypePrefixed creates a new TypeModified node.
func (c *Context) NewTypePrefixed(args TypePrefixedArgs) TypePrefixed {
	c.panicIfNotOurs(args.Prefix, args.Type)

	raw := c.types.modifieds.Append(rawPrefixed{
		prefix: args.Prefix.raw,
	})
	ty := TypePrefixed{
		withContext: withContext{c},
		idx:         c.types.modifieds.Len() - 1,
		raw:         raw,
	}
	ty.SetType(args.Type)

	return ty
}

// NewTypeGeneric creates a new TypeGeneric node.
func (c *Context) NewTypeGeneric(args TypeGenericArgs) TypeGeneric {
	c.panicIfNotOurs(args.Path, args.AngleBrackets)

	ty := c.types.generics.Append(rawGeneric{
		path: args.Path.raw,
		args: rawTypeList{brackets: args.AngleBrackets.raw},
	})

	return TypeGeneric{
		withContext: withContext{c},
		idx:         c.types.generics.Len() - 1,
		raw:         ty,
	}
}

// NewOptions creates a new Options node.
func (c *Context) NewOptions(brackets Token) Options {
	c.panicIfNotOurs(brackets)
	c.options.Append(optionsImpl{
		brackets: brackets.raw,
	})
	return rawOptions(c.options.Len()).With(c)
}
