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

import "github.com/bufbuild/protocompile/internal/arena"

// NewDeclEmpty creates a new DeclEmpty node.
func (c *Context) NewDeclEmpty(semicolon Token) DeclEmpty {
	c.panicIfNotOurs(semicolon)

	ptr := c.decls.empties.New(rawDeclEmpty{semi: semicolon.raw})
	return wrapDecl[DeclEmpty](arena.Untyped(ptr), c)
}

// NewDeclSyntax creates a new DeclPragma node.
func (c *Context) NewDeclSyntax(args DeclSyntaxArgs) DeclSyntax {
	c.panicIfNotOurs(args.Keyword, args.Equals, args.Value, args.Options, args.Semicolon)

	ptr := c.decls.syntaxes.New(rawDeclSyntax{
		keyword: args.Keyword.raw,
		equals:  args.Equals.raw,
		options: args.Options.rawOptions(),
		semi:    args.Semicolon.raw,
	})

	decl := wrapDecl[DeclSyntax](arena.Untyped(ptr), c)
	decl.SetValue(args.Value)

	return decl
}

// NewDeclPackage creates a new DeclPackage node.
func (c *Context) NewDeclPackage(args DeclPackageArgs) DeclPackage {
	c.panicIfNotOurs(args.Keyword, args.Path, args.Options, args.Semicolon)

	ptr := c.decls.packages.New(rawDeclPackage{
		keyword: args.Keyword.raw,
		path:    args.Path.raw,
		options: args.Options.rawOptions(),
		semi:    args.Semicolon.raw,
	})
	return wrapDecl[DeclPackage](arena.Untyped(ptr), c)
}

// NewDeclImport creates a new DeclImport node.
func (c *Context) NewDeclImport(args DeclImportArgs) DeclImport {
	c.panicIfNotOurs(args.Keyword, args.Modifier, args.ImportPath, args.Options, args.Semicolon)

	ptr := c.decls.imports.New(rawDeclImport{
		keyword:  args.Keyword.raw,
		modifier: args.Modifier.raw,
		options:  args.Options.rawOptions(),
		semi:     args.Semicolon.raw,
	})
	return wrapDecl[DeclImport](arena.Untyped(ptr), c)
}

// NewDeclDef creates a new DeclDef node.
func (c *Context) NewDeclDef(args DeclDefArgs) DeclDef {
	c.panicIfNotOurs(
		args.Keyword, args.Type, args.Name, args.Returns,
		args.Equals, args.Value, args.Options, args.Body, args.Semicolon)

	ptr := c.decls.defs.New(rawDeclDef{
		name:    args.Name.raw,
		equals:  args.Equals.raw,
		options: args.Options.rawOptions(),
		semi:    args.Semicolon.raw,
	})
	decl := wrapDecl[DeclDef](arena.Untyped(ptr), c)

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

	ptr := c.decls.bodies.New(rawDeclBody{braces: braces.raw})
	return wrapDecl[DeclBody](arena.Untyped(ptr), c)
}

// NewDeclRange creates a new DeclRange node.
func (c *Context) NewDeclRange(args DeclRangeArgs) DeclRange {
	c.panicIfNotOurs(args.Keyword, args.Options, args.Semicolon)

	ptr := c.decls.ranges.New(rawDeclRange{
		keyword: args.Keyword.raw,
		options: args.Options.rawOptions(),
		semi:    args.Semicolon.raw,
	})
	decl := wrapDecl[DeclRange](arena.Untyped(ptr), c)

	return decl
}

// NewExprPrefixed creates a new ExprPrefixed node.
func (c *Context) NewExprPrefixed(args ExprPrefixedArgs) ExprPrefixed {
	c.panicIfNotOurs(args.Prefix, args.Expr)

	ptr := c.exprs.prefixes.New(rawExprPrefixed{
		prefix: args.Prefix.raw,
	})
	expr := ExprPrefixed{
		withContext: withContext{c},
		ptr:         arena.Untyped(ptr),
		raw:         ptr.In(&c.exprs.prefixes),
	}
	expr.SetExpr(args.Expr)
	return expr
}

// NewExprRange creates a new ExprRange node.
func (c *Context) NewExprRange(args ExprRangeArgs) ExprRange {
	c.panicIfNotOurs(args.Start, args.To, args.End)

	ptr := c.exprs.ranges.New(rawExprRange{
		to: args.To.raw,
	})
	expr := ExprRange{
		withContext: withContext{c},
		ptr:         arena.Untyped(ptr),
		raw:         ptr.In(&c.exprs.ranges),
	}
	expr.SetBounds(args.Start, args.End)
	return expr
}

// NewExprArray creates a new ExprArray node.
func (c *Context) NewExprArray(brackets Token) ExprArray {
	c.panicIfNotOurs(brackets)

	ptr := c.exprs.arrays.New(rawExprArray{
		brackets: brackets.raw,
	})
	return ExprArray{
		withContext: withContext{c},
		ptr:         arena.Untyped(ptr),
		raw:         ptr.In(&c.exprs.arrays),
	}
}

// NewExprDict creates a new ExprDict node.
func (c *Context) NewExprDict(braces Token) ExprDict {
	c.panicIfNotOurs(braces)

	ptr := c.exprs.dicts.New(rawExprDict{
		braces: braces.raw,
	})
	return ExprDict{
		withContext: withContext{c},
		ptr:         arena.Untyped(ptr),
		raw:         ptr.In(&c.exprs.dicts),
	}
}

// NewExprPrefixed creates a new ExprPrefixed node.
func (c *Context) NewExprKV(args ExprKVArgs) ExprKV {
	c.panicIfNotOurs(args.Key, args.Colon, args.Value)

	ptr := c.exprs.fields.New(rawExprKV{
		colon: args.Colon.raw,
	})
	expr := ExprKV{
		withContext: withContext{c},
		ptr:         arena.Untyped(ptr),
		raw:         ptr.In(&c.exprs.fields),
	}
	expr.SetKey(args.Key)
	expr.SetValue(args.Value)
	return expr
}

// NewTypePrefixed creates a new TypeModified node.
func (c *Context) NewTypePrefixed(args TypePrefixedArgs) TypePrefixed {
	c.panicIfNotOurs(args.Prefix, args.Type)

	ptr := c.types.modifieds.New(rawPrefixed{
		prefix: args.Prefix.raw,
	})
	ty := TypePrefixed{
		withContext: withContext{c},
		ptr:         arena.Untyped(ptr),
		raw:         ptr.In(&c.types.modifieds),
	}
	ty.SetType(args.Type)

	return ty
}

// NewTypeGeneric creates a new TypeGeneric node.
func (c *Context) NewTypeGeneric(args TypeGenericArgs) TypeGeneric {
	c.panicIfNotOurs(args.Path, args.AngleBrackets)

	ptr := c.types.generics.New(rawGeneric{
		path: args.Path.raw,
		args: rawTypeList{brackets: args.AngleBrackets.raw},
	})

	return TypeGeneric{
		withContext: withContext{c},
		ptr:         arena.Untyped(ptr),
		raw:         ptr.In(&c.types.generics),
	}
}

// NewOptions creates a new Options node.
func (c *Context) NewOptions(brackets Token) Options {
	c.panicIfNotOurs(brackets)
	ptr := c.options.New(optionsImpl{
		brackets: brackets.raw,
	})
	return rawOptions(ptr).With(c)
}
