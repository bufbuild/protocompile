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
				if i > 0 {
					r.Errorf("syntax declaration must be the first declaration in a file").With(
						report.Snippetf(decl, "expected this to be the first declaration"),
						report.Snippetf(node.At(0), "previous declaration"),
					)
				}
				syntax = decl

			case DeclPackage:
				if i != 0 {
					if _, ok := node.At(i - 1).(DeclSyntax); !ok {
						r.Errorf("package declaration can only come after a syntax declaration").With(
							report.Snippetf(decl, "expected this to follow a syntax declaration"),
							report.Snippetf(node.At(i-1), "previous declaration"),
						)
					}
				}
				pkg = decl
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

		if !node.Options().Nil() {
			r.Errorf("options are not permitted on syntax declarations").With(
				report.Snippetf(node.Options(), "help: remove this"),
			)
		}

		// NOTE: node can only be nil if an error occurred in the parser.
		if node.Value() == nil {
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

	case DeclPackage:
		if _, ok := parent.(File); !ok {
			r.Error(ErrInvalidChild{parent, node})
		}

		if !node.Options().Nil() {
			r.Errorf("options are not permitted on syntax declarations").With(
				report.Snippetf(node.Options(), "help: remove this"),
			)
		}

		if node.Path().Nil() {
			r.Errorf("missing package name").With(
				report.Snippetf(node, "help: add a path after `package`"),
			)
			return
		}

		var idx int
		node.Path().Components(func(pc PathComponent) bool {
			if pc.Separator().Text() == "/" {
				r.Errorf("package names cannot contain slashes").With(
					report.Snippet(pc.Separator()),
				)
				return false
			}

			if idx == 0 && !pc.Separator().Nil() {
				r.Errorf("package names cannot be absolute paths").With(
					report.Snippetf(pc.Separator(), "help: remove this dot"),
				)
				return false
			}

			if pc.IsExtension() {
				r.Errorf("package names cannot contain extension names").With(
					report.Snippet(pc.Name()),
				)
				return false
			}

			idx++
			return true
		})

	case DeclImport:
		if _, ok := parent.(File); !ok {
			r.Error(ErrInvalidChild{parent, node})
		}

		if node.IsWeak() {
			r.Warnf("weak imports are discouraged and broken in some runtimes").With(
				report.Snippet(node.Modifier()),
			)
		}

		if !node.Options().Nil() {
			r.Errorf("options are not permitted on syntax declarations").With(
				report.Snippetf(node.Options(), "help: remove this"),
			)
		}

		switch expr := node.ImportPath().(type) {
		case ExprLiteral:
			if expr.Token.Kind() == TokenString {
				value, _ := expr.Token.AsString()
				if !expr.Token.IsPureString() {
					r.Warnf("import path should be a single, escape-less string").With(
						report.Snippetf(expr.Token, `help: change this to "%s"`, value),
					)
				}
				return
			}
		case ExprPath:
			r.Errorf("cannot import by Protobuf symbol").With(
				report.Snippetf(expr, "expected a quoted filesystem path"),
			)
			return
		}

		r.Error(errUnexpected{
			node:  node.ImportPath(),
			where: "in " + describe(node),
			want:  []string{"string literal"},
		})

	case DeclDef:

	case DeclRange:
	}
}
