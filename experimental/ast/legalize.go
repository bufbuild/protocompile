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
	"slices"

	"github.com/bufbuild/protocompile/experimental/report"
)

var (
	knownSyntaxes = []string{"proto2", "proto3"}
	knownEditions = []string{"2023"}
)

// legalize generates diagnostics for anything in node that is permitted by the parser
// but not by standard Protobuf.
func legalize(r *report.Report, parent, node Spanner) {
	switch node := node.(type) {
	case File:
		var syntax DeclSyntax
		var pkg DeclPackage
		node.Iter(func(i int, decl Decl) bool {
			switch decl := decl.(type) {
			case DeclSyntax:
				if syntax.Nil() {
					if i > 0 {
						r.Errorf("syntax declaration must be the first declaration in a file").With(
							report.Snippetf(decl, "expected this to be the first declaration"),
							report.Snippetf(node.At(0), "previous declaration"),
						)
					}
					syntax = decl
				} else {
					r.Error(ErrMoreThanOne{
						First:  syntax,
						Second: decl,
						what:   "syntax declaration",
					})
				}
			case DeclPackage:
				if pkg.Nil() {
					if (syntax.Nil() && i > 0) || i > 1 {
						r.Errorf("package declaration can only come after a syntax declaration").With(
							report.Snippetf(decl, "expected this to follow a syntax declaration"),
							report.Snippetf(node.At(i-1), "previous declaration"),
						)
					}
					pkg = decl
				} else {
					r.Error(ErrMoreThanOne{
						First:  pkg,
						Second: decl,
						what:   "package declaration",
					})
				}
			}

			legalize(r, node, decl)
			return true
		})

		if syntax.Nil() {
			r.Warn(ErrNoSyntax{node.Context().Path()})
		}
		if pkg.Nil() {
			r.Warn(ErrNoPackage{node.Context().Path()})
		}

	case DeclSyntax:
		if _, ok := parent.(File); !ok {
			r.Error(ErrInvalidChild{parent, node})
		}

		if node.Value() == nil {
			r.Errorf("missing value after `=` for %s", describe(node)).With(
				report.Snippetf(node, "expected a string literal"),
			)
			return
		}

		what := node.Keyword().Text()
		var values []string
		switch {
		case node.IsSyntax():
			values = knownSyntaxes
		case node.IsEdition():
			values = knownEditions
		}

		switch expr := node.Value().(type) {
		case ExprLiteral:
			if expr.Token.Kind() == TokenString {
				// If ok is false, we assume this has already been diagnosed in the
				// lexer, because TokenString -> this is a string.
				value, ok := expr.Token.AsString()
				if ok && !slices.Contains(values, value) {
					r.Error(ErrUnknownSyntax{Node: node, Value: expr.Token})
				} else if !expr.Token.IsPureString() {
					r.Warnf("%s value should be a single, escape-less string", what).With(
						report.Snippetf(expr.Token, `help: change this to "%s"`, value),
					)
				}
				return
			} else {
				// This might be an unquoted edition.
				value := expr.Token.Text()
				if slices.Contains(values, value) {
					r.Errorf("missing quotes around %s value", what).With(
						report.Snippetf(expr, "help: wrap this in quotes"),
					)
					return
				}
			}
		case ExprPath:
			// Single identifier means the user forgot the quotes around protoN.
			if name := expr.Path.AsIdent(); !name.Nil() {
				if slices.Contains(values, name.Name()) {
					r.Errorf("missing quotes around %s value", what).With(
						report.Snippetf(expr, "help: wrap this in quotes"),
					)
					return
				}
			}
		}

		r.Error(errUnexpected{
			node:  node.Value(),
			where: "in " + describe(node),
			want:  []string{"string literal"},
		})
	}
}
