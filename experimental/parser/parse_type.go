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
	"github.com/bufbuild/protocompile/experimental/internal/astx"
	"github.com/bufbuild/protocompile/experimental/internal/taxa"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/experimental/token/keyword"
	"github.com/bufbuild/protocompile/internal/ext/iterx"
	"github.com/bufbuild/protocompile/internal/ext/slicesx"
)

// parseType attempts to parse a type, optionally followed by a non-absolute
// path (depending on what pathAfter says).
//
// May return nil if parsing completely fails.
func parseType(p *parser, c *token.Cursor, where taxa.Place) ast.TypeAny {
	ty, _ := parseTypeImpl(p, c, where, false)
	return ty
}

// parseType attempts to parse a type, optionally followed by a non-absolute
// path (depending on what pathAfter says).
//
// This function is called in many situations that seem a bit weird to be
// parsing a type in, such as at the top level. This is because of an essential
// ambiguity in Protobuf's grammar (or rather, the version of it that we parse):
// message Foo can start either a field (message Foo;) or a message (message Foo
// {}). Thus, in such contexts we always parse a type-and-path, and based on
// what comes next, reinterpret the type as potentially being a keyword.
//
// This function assumes that we have decided to definitely parse a type, and
// will emit diagnostics to that effect. As such, the current token position on
// c should not be nil.
//
// May return nil if parsing completely fails.
// TODO: return something like ast.TypeError instead.
func parseTypeAndPath(p *parser, c *token.Cursor, where taxa.Place) (ast.TypeAny, ast.Path) {
	return parseTypeImpl(p, c, where, true)
}

func parseTypeImpl(p *parser, c *token.Cursor, where taxa.Place, pathAfter bool) (ast.TypeAny, ast.Path) {
	var isList, isInMethod bool
	switch where.Subject() {
	case taxa.MethodIns, taxa.MethodOuts,
		taxa.KeywordReturns: // Used when parsing the invalid `returns foo.Bar` production.
		isInMethod = true
		fallthrough
	case taxa.TypeParams:
		isList = true
	}

	// First, parse a path, possibly preceded by a sequence of modifiers.
	//
	// To do this, we repeatedly parse paths, and each time we get a path that
	// starts with an identifier, we interpret it as a modifier. For example,
	// repeated.foo needs to be interpreted as repeated .foo.
	var (
		mods   []token.Token
		tyPath ast.Path
	)
	for !c.Done() && tyPath.IsZero() {
		next := c.Peek()
		if !canStartPath(next) {
			break
		}

		tyPath = parsePath(p, c)
		if tyPath.Absolute() {
			break // Absolute paths cannot start with a modifier, so we are done.
		}

		first, _ := iterx.First(tyPath.Components)
		ident := first.AsIdent()
		if ident.IsZero() {
			break // If this starts with an extension, we're done.
		}

		// Here is a nasty case. Suppose the user has written within message
		// scope something like
		//
		//  package .foo.bar = 5;
		//
		// This is a syntax error but only because the name of the field is a
		// non-trivial path. However, we would like for this to be diagnosed as
		// a package declaration. Thus, if this looks like a bad package, we
		// break it up so that the type is "package" and the path is ".foo.bar",
		// so that the DeclPackage code path can diagnose it.
		//
		// We require that this have no modifiers and that it not be followed by
		// a path, so that the following productions are *not* treated as weird
		// packages:
		//
		//  optional package.foo.bar;
		//  package.foo.bar baz;
		//
		// Note that this does not apply inside of type lists. This is because
		// type lists *only* contain types, and not productions started by
		// keywords.
		//
		// This case applies to the keywords:
		// 	- package
		// 	- extend
		//  - option
		if !isList && len(mods) == 0 &&
			slicesx.Among(ident.Keyword(), keyword.Package, keyword.Extend, keyword.Option) &&
			!canStartPath(c.Peek()) {
			kw, path := tyPath.Split(1)
			if !path.IsZero() {
				return ast.TypePath{Path: kw}.AsAny(), path
			}
		}

		// Check if ident is a modifier, and if so, peel it off.
		//
		// We need to be careful to only peel off `stream` inside of a method
		// type. If the entire path is a single identifier, we always peel it
		// off, since code that follows handles turning it back into a path
		// based on what comes after it.
		var isMod bool
		_, rest := tyPath.Split(1)
		switch k := ident.Keyword(); {
		case k.IsFieldTypeModifier():
			// NOTE: We do not need to look at isInMethod here, because it
			// implies isList (sort of: in the case of writing something
			// like `returns optional.Foo`, this will be parsed as
			// `returns (optional .Foo)`. However, this production is already
			// invalid, because of the missing parentheses, so we don't need to
			// legalize it.
			isMod = !isList || rest.IsZero()

		case k.IsTypeModifier(), k.IsImportModifier():
			// Do not pick these up if rest is non-zero, because that means
			// we're in a case like export.Foo x = 1;. The only case where
			// export/local should be picked up is if the next token is not
			// .Foo or similar.
			isMod = rest.IsZero()

		case k.IsMethodTypeModifier():
			isMod = isInMethod || rest.IsZero()
		}

		if isMod {
			mods = append(mods, ident)
			tyPath = rest
		}
	}

	if tyPath.IsZero() {
		if len(mods) == 0 {
			return ast.TypeAny{}, ast.Path{}
		}

		// Pop the last mod and make that into the type path. This makes
		// `optional optional` work as a type.
		last := mods[len(mods)-1]
		tyPath = astx.NewPath(p.File(), last, last)
		mods = mods[:len(mods)-1]
	}

	ty := ast.TypePath{Path: tyPath}.AsAny()

	// Next, look for some angle brackets. We need to do this before draining
	// mods, because angle brackets bind more tightly than modifiers.
	if angles := c.Peek(); angles.Keyword() == keyword.Less {
		c.Next() // Consume the angle brackets.
		generic := p.NewTypeGeneric(ast.TypeGenericArgs{
			Path:          tyPath,
			AngleBrackets: angles,
		})

		delimited[ast.TypeAny]{
			p:    p,
			c:    c,
			what: taxa.Type,
			in:   taxa.TypeParams,

			required: true,
			exhaust:  false,
			parse: func(c *token.Cursor) (ast.TypeAny, bool) {
				ty := parseType(p, c, taxa.TypeParams.In())
				return ty, !ty.IsZero()
			},
			start: canStartPath,
			stop: func(t token.Token) bool {
				kw := t.Keyword()
				return kw == keyword.Greater ||
					kw == keyword.Eq // Heuristic for stopping reasonably early in the case of map<K, V m = 1;
			},
		}.appendTo(generic.Args())

		// Need to fuse the angle brackets, because the lexer won't do it.
		if c.Peek().Keyword() == keyword.Greater {
			token.Fuse(angles, c.Next())
		}

		ty = generic.AsAny()
	}

	// Now, check for a path that follows all this. If there isn't a path, and
	// ty is (still) a TypePath, and there is still at least one modifier, we
	// interpret the last modifier as the type and the current path type as the
	// path after the type.
	var path ast.Path
	if pathAfter {
		next := c.Peek()
		if canStartPath(next) {
			path = parsePath(p, c)
		} else if !isList && ty.Kind() == ast.TypeKindPath && len(mods) > 0 {
			path = tyPath

			// Pop the last mod and make that into the type. This makes
			// `optional optional = 1` work as a proto3 field.
			last := mods[len(mods)-1]
			tyPath = astx.NewPath(p.File(), last, last)
			mods = mods[:len(mods)-1]
			ty = ast.TypePath{Path: tyPath}.AsAny()
		}
	}

	// Finally, apply any remaining modifiers (in reverse order) to ty.
	for i := len(mods) - 1; i >= 0; i-- {
		ty = p.NewTypePrefixed(ast.TypePrefixedArgs{
			Prefix: mods[i],
			Type:   ty,
		}).AsAny()
	}

	return ty, path
}
