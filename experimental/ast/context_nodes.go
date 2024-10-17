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

	decl := wrapDeclEmpty(c, c.decls.empties.New(rawDeclEmpty{
		semi: semicolon.raw,
	}))

	return decl
}

// NewDeclSyntax creates a new DeclSyntax node.
func (c *Context) NewDeclSyntax(args DeclSyntaxArgs) DeclSyntax {
	c.panicIfNotOurs(args.Keyword, args.Equals, args.Value, args.Options, args.Semicolon)

	return wrapDeclSyntax(c, c.decls.syntaxes.New(rawDeclSyntax{
		keyword: args.Keyword.raw,
		equals:  args.Equals.raw,
		value:   toRawExpr(args.Value),
		options: args.Options.ptr,
		semi:    args.Semicolon.raw,
	}))
}

// NewDeclPackage creates a new DeclPackage node.
func (c *Context) NewDeclPackage(args DeclPackageArgs) DeclPackage {
	c.panicIfNotOurs(args.Keyword, args.Path, args.Options, args.Semicolon)

	return wrapDeclPackage(c, c.decls.packages.New(rawDeclPackage{
		keyword: args.Keyword.raw,
		path:    args.Path.raw,
		options: args.Options.ptr,
		semi:    args.Semicolon.raw,
	}))
}

// NewDeclImport creates a new DeclImport node.
func (c *Context) NewDeclImport(args DeclImportArgs) DeclImport {
	c.panicIfNotOurs(args.Keyword, args.Modifier, args.ImportPath, args.Options, args.Semicolon)

	return wrapDeclImport(c, c.decls.imports.New(rawDeclImport{
		keyword:    args.Keyword.raw,
		modifier:   args.Modifier.raw,
		importPath: toRawExpr(args.ImportPath),
		options:    args.Options.ptr,
		semi:       args.Semicolon.raw,
	}))
}

// NewDeclDef creates a new DeclDef node.
func (c *Context) NewDeclDef(args DeclDefArgs) DeclDef {
	c.panicIfNotOurs(
		args.Keyword, args.Type, args.Name, args.Returns,
		args.Equals, args.Value, args.Options, args.Body, args.Semicolon)

	decl := wrapDeclDef(c, c.decls.defs.New(rawDeclDef{
		name:    args.Name.raw,
		equals:  args.Equals.raw,
		value:   toRawExpr(args.Value),
		options: args.Options.ptr,
		body:    args.Body.ptr,
		semi:    args.Semicolon.raw,
	}))

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

	return decl
}

// NewDeclBody creates a new DeclBody node.
//
// To add declarations to the returned body, use [DeclBody.Append].
func (c *Context) NewDeclBody(braces Token) DeclBody {
	c.panicIfNotOurs(braces)

	return wrapDeclBody(c, c.decls.bodies.New(rawDeclBody{
		braces: braces.raw,
	}))
}

// NewDeclRange creates a new DeclRange node.
//
// To add ranges to the returned declaration, use [DeclRange.Append].
func (c *Context) NewDeclRange(args DeclRangeArgs) DeclRange {
	c.panicIfNotOurs(args.Keyword, args.Options, args.Semicolon)

	return wrapDeclRange(c, c.decls.ranges.New(rawDeclRange{
		keyword: args.Keyword.raw,
		options: args.Options.ptr,
		semi:    args.Semicolon.raw,
	}))
}

// NewExprPrefixed creates a new ExprPrefixed node.
func (c *Context) NewExprPrefixed(args ExprPrefixedArgs) ExprPrefixed {
	c.panicIfNotOurs(args.Prefix, args.Expr)

	ptr := c.exprs.prefixes.New(rawExprPrefixed{
		prefix: args.Prefix.raw,
		expr:   toRawExpr(args.Expr),
	})
	return ExprPrefixed{
		withContext: withContext{c},
		ptr:         ptr,
		raw:         c.exprs.prefixes.Deref(ptr),
	}
}

// NewExprRange creates a new ExprRange node.
func (c *Context) NewExprRange(args ExprRangeArgs) ExprRange {
	c.panicIfNotOurs(args.Start, args.To, args.End)

	ptr := c.exprs.ranges.New(rawExprRange{
		to:    args.To.raw,
		start: toRawExpr(args.Start),
		end:   toRawExpr(args.End),
	})
	return ExprRange{
		withContext: withContext{c},
		ptr:         ptr,
		raw:         c.exprs.ranges.Deref(ptr),
	}
}

// NewExprArray creates a new ExprArray node.
//
// To add elements to the returned expression, use [ExprArray.Append].
func (c *Context) NewExprArray(brackets Token) ExprArray {
	c.panicIfNotOurs(brackets)

	ptr := c.exprs.arrays.New(rawExprArray{
		brackets: brackets.raw,
	})
	return ExprArray{
		withContext: withContext{c},
		ptr:         ptr,
		raw:         c.exprs.arrays.Deref(ptr),
	}
}

// NewExprDict creates a new ExprDict node.
//
// To add elements to the returned expression, use [ExprDict.Append].
func (c *Context) NewExprDict(braces Token) ExprDict {
	c.panicIfNotOurs(braces)

	ptr := c.exprs.dicts.New(rawExprDict{
		braces: braces.raw,
	})
	return ExprDict{
		withContext: withContext{c},
		ptr:         ptr,
		raw:         c.exprs.dicts.Deref(ptr),
	}
}

// NewExprPrefixed creates a new ExprPrefixed node.
func (c *Context) NewExprKV(args ExprKVArgs) ExprKV {
	c.panicIfNotOurs(args.Key, args.Colon, args.Value)

	ptr := c.exprs.kvs.New(rawExprKV{
		key:   toRawExpr(args.Key),
		colon: args.Colon.raw,
		value: toRawExpr(args.Value),
	})
	return ExprKV{
		withContext: withContext{c},
		ptr:         ptr,
		raw:         c.exprs.kvs.Deref(ptr),
	}
}

// NewTypePrefixed creates a new TypePrefixed node.
func (c *Context) NewTypePrefixed(args TypePrefixedArgs) TypePrefixed {
	c.panicIfNotOurs(args.Prefix, args.Type)

	ptr := c.types.prefixed.New(rawTypePrefixed{
		prefix: args.Prefix.raw,
		ty:     toRawType(args.Type),
	})
	return TypePrefixed{
		withContext: withContext{c},
		ptr:         ptr,
		raw:         c.types.prefixed.Deref(ptr),
	}
}

// NewTypeGeneric creates a new TypeGeneric node.
//
// To add arguments to the returned type, use [TypeGeneric.Append].
func (c *Context) NewTypeGeneric(args TypeGenericArgs) TypeGeneric {
	c.panicIfNotOurs(args.Path, args.AngleBrackets)

	ptr := c.types.generics.New(rawTypeGeneric{
		path: args.Path.raw,
		args: rawTypeList{brackets: args.AngleBrackets.raw},
	})
	return TypeGeneric{
		withContext: withContext{c},
		ptr:         ptr,
		raw:         c.types.generics.Deref(ptr),
	}
}

// NewCompactOptions creates a new Options node.
func (c *Context) NewCompactOptions(brackets Token) CompactOptions {
	c.panicIfNotOurs(brackets)

	return wrapOptions(c, c.options.New(rawCompactOptions{
		brackets: brackets.raw,
	}))
}
