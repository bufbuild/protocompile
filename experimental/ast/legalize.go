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
	"fmt"
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
		imports := map[string]DeclImport{}
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

			// Note that this causes only imports in the file level to be deduplicated;
			// this is on purpose.
			case DeclImport:
				if str, ok := decl.ImportPath().(ExprLiteral); ok {
					path, ok := str.Token.AsString()
					if !ok {
						break
					}

					if prev, ok := imports[path]; ok {
						r.Warn(ErrDuplicateImport{
							First:  prev,
							Second: decl,
							Path:   path,
						})
					} else {
						imports[path] = decl
					}
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

		expectQuoted := func(value string, tok Token) bool {
			if !slices.Contains(values, value) {
				return false
			}

			r.Errorf("missing quotes around %s value", what).With(
				report.Snippetf(tok, "help: wrap this in quotes"),
			)
			return true
		}

		switch expr := node.Value().(type) {
		case ExprLiteral:
			if expr.Token.Kind() == TokenString {
				// If ok is false, we assume this has already been diagnosed in the
				// lexer, because TokenString -> this is a string.
				value, ok := expr.Token.AsString()
				if ok && !slices.Contains(values, value) {
					r.Error(ErrUnknownSyntax{Node: node, Value: expr.Token})
				} else {
					legalizePureString(r, "`"+what+"` value", expr.Token)
				}
				return
			} else if expectQuoted(expr.Token.Text(), expr.Token) {
				// This might be an unquoted edition.
				return
			}
		case ExprPath:
			// Single identifier means the user forgot the quotes around proto3
			// or such.
			if name := expr.Path.AsIdent(); !name.Nil() && expectQuoted(name.Name(), name) {
				return
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
		case nil:
			// Select a token to place the suggestion after.
			importAfter := node.Modifier()
			if importAfter.Nil() {
				importAfter = node.Keyword()
			}

			r.Errorf("import is missing a file path").With(
				report.Snippetf(importAfter, "help: insert the name of the file to import after this keyword"),
			)
			return
		case ExprLiteral:
			if expr.Token.Kind() == TokenString {
				legalizePureString(r, "import path", expr.Token)
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
		switch parent := parent.(type) {
		case File:
		case DeclDef:
			if _, ok := parent.Classify().(DefMessage); !ok {
				r.Error(ErrInvalidChild{parent, node})
			}
		default:
			r.Error(ErrInvalidChild{parent, node})
		}

		// This part of legalization "essentially" re-implements Classify, but
		// generates diagnostics instead of failing.
		kw := node.Keyword()
		switch kwText := kw.Text(); kwText {
		case "message", "enum", "service", "extends", "oneof":
			if kwText != "extends" && node.Name().AsIdent().Nil() {
				r.Error(errUnexpected{
					node:  node.Name(),
					where: fmt.Sprintf("in %s name", kwText),
					want:  []string{"identifier"},
				})
			}

			if sig := node.Signature(); !sig.Nil() {
				r.Error(errUnexpected{
					node:  sig,
					where: fmt.Sprintf("in %s definition", kwText),
				})
			}
			if value := node.Value(); value != nil {
				r.Error(errUnexpected{
					node:  value,
					where: fmt.Sprintf("in %s definition", kwText),
				})
			} else if eq := node.Equals(); !eq.Nil() {
				r.Error(errUnexpected{
					node:  value,
					where: fmt.Sprintf("in %s definition", kwText),
				})
			}
			if options := node.Options(); !options.Nil() {
				r.Errorf("compact options are not permitted on %s definitions", kwText).With(
					report.Snippetf(node.Options(), "help: remove this"),
				)
			}

			// Parent must be file or message, unless it's a service,
			// in which case it must be file, or unless it's a oneof, in which
			// case it must be message.
			switch parent := parent.(type) {
			case File:
				if kwText == "oneof" {
					r.Error(ErrInvalidChild{parent, node})
				}
			case DeclDef:
				if kwText == "service" || parent.Keyword().Text() != "message" {
					r.Error(ErrInvalidChild{parent, node})
				}
			default:
				r.Error(ErrInvalidChild{parent, node})
			}
		case "group":
			if node.Name().AsIdent().Nil() {
				r.Error(errUnexpected{
					node:  node.Name(),
					where: "in group name",
					want:  []string{"identifier"},
				})
			}

			if sig := node.Signature(); !sig.Nil() {
				r.Error(errUnexpected{
					node:  sig,
					where: fmt.Sprintf("in %s definition", kwText),
				})
			}

			if value := node.Value(); value == nil {
				var numberAfter Spanner
				if name := node.Name(); !name.Nil() {

				}

				numberAfter = node.Name()
				if numberAfter.Nil() {
					numberAfter = kw
				}

				// TODO: This should be moved to somewhere where we can suggest
				// the next unallocated value as the field number.
				r.Errorf("missing field number").With(
					report.Snippetf(node.Options(), "help: remove this"),
				)
			}
		}

		node.Body().Iter(func(_ int, decl Decl) bool {
			legalize(r, node, decl)
			return true
		})

	case DeclRange:
		parent, ok := parent.(DeclDef)
		if !ok {
			r.Error(ErrInvalidChild{parent, node})
			return
		}
		def := parent.Classify()
		switch def.(type) {
		case DefMessage, DefEnum:
		default:
			r.Error(ErrInvalidChild{parent, node})
			return
		}

		if node.IsReserved() && !node.Options().Nil() {
			r.Errorf("options are not permitted on reserved ranges").With(
				report.Snippetf(node.Options(), "help: remove this"),
			)
		}

		// TODO: Most of this should probably get hoisted to wherever it is that we do
		// type checking once that exists.
		node.Iter(func(_ int, expr Expr) bool {
			switch expr := expr.(type) {
			case ExprRange:
				ensureInt32 := func(expr Expr) {
					_, ok := expr.AsInt32()
					if !ok {
						r.Errorf("mismatched types").With(
							report.Snippetf(expr, "expected `int32`"),
							report.Snippetf(node.Keyword(), "expected due to this"),
						)
					}
				}

				start, end := expr.Bounds()
				ensureInt32(start)
				if path, ok := end.(ExprPath); ok && path.AsIdent().Name() == "max" {
					// End is allowed to be "max".
				} else {
					ensureInt32(end)
				}
				return true
			case ExprPath:
				if node.IsReserved() && !expr.Path.AsIdent().Nil() {
					return true
				}
				// TODO: diagnose against a lone "max" ExprPath in an extension
				// range.
			case ExprLiteral:
				if node.IsReserved() && expr.Token.Kind() == TokenString {
					if text, ok := expr.Token.AsString(); ok && !isASCIIIdent(text) {
						r.Error(ErrNonASCIIIdent{Token: expr.Token})
					} else {
						legalizePureString(r, "reserved field name", expr.Token)
					}
					return true
				}
			}

			_, ok := expr.AsInt32()
			if !ok {
				allowedTypes := "expected `int32` or `int32` range"
				if node.IsReserved() {
					allowedTypes = "expected `int32`, `int32` range, `string`, or identifier"
				}
				r.Errorf("mismatched types").With(
					report.Snippetf(expr, allowedTypes),
					report.Snippetf(node.Keyword(), "expected due to this"),
				)
			}

			return true
		})
	}
}

func legalizePureString(r *report.Report, what string, tok Token) {
	if value, ok := tok.AsString(); ok {
		if !tok.IsPureString() {
			r.Warnf("%s should be a single, escape-less string", what).With(
				report.Snippetf(tok, `help: change this to %q`, value),
			)
		}
	}
}
