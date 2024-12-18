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

// enum is a helper for generating boilerplate related to Go enums.
//
// To generate boilerplate for a given file, use
//
//	//go:generate go run github.com/bufbuild/protocompile/internal/enum
package main

import (
	"debug/buildinfo"
	_ "embed"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"slices"
	"strconv"
	"strings"
	"text/scanner"
	"text/template"
	"unicode"

	"github.com/bufbuild/protocompile/internal/ext/slicesx"
)

var (
	//go:embed generated.go.tmpl
	tmplText string
	tmpl     = template.Must(template.New("generated.go.tmpl").Parse(tmplText))
)

type Directive struct {
	Pos  token.Pos
	Name string
	Args []string
}

func HasDirectives(comments []*ast.CommentGroup) bool {
	for _, g := range comments {
		for _, c := range g.List {
			rest, ok := strings.CutPrefix(c.Text, "//enum:")
			if ok && rest != "" {
				return true
			}
		}
	}
	return false
}

func ParseDirectives(fs *token.FileSet, comments []*ast.CommentGroup) ([]Directive, error) {
	var d []Directive
	for _, g := range comments {
		for _, c := range g.List {
			rest, ok := strings.CutPrefix(c.Text, "//enum:")
			rest = strings.TrimSpace(rest)
			if !ok || rest == "" {
				continue
			}

			var err error
			var args []string
			sc := scanner.Scanner{
				Error: func(_ *scanner.Scanner, msg string) {
					err = fmt.Errorf("%s", msg)
				},
				Mode: scanner.ScanIdents | scanner.ScanStrings | scanner.ScanRawStrings,
				IsIdentRune: func(r rune, _ int) bool {
					return !unicode.IsSpace(r) && r != '"' && r != '`'
				},
			}
			sc.Init(strings.NewReader(rest))
		scan:
			for {
				next := sc.Scan()
				if err != nil {
					return nil, fmt.Errorf("%v: invalid directive: %v", fs.Position(c.Pos()), err)
				}

				switch next {
				case scanner.EOF:
					break scan
				case scanner.Ident:
					args = append(args, sc.TokenText())
				case scanner.String:
					str, _ := strconv.Unquote(sc.TokenText())
					args = append(args, str)
				}
			}

			d = append(d, Directive{
				Pos:  c.Pos(),
				Name: args[0],
				Args: args[1:],
			})
		}
	}
	return d, nil
}

type Enum struct {
	Type    *ast.TypeSpec
	Methods struct {
		String, GoString, FromString string

		StringFunc string
	}
	Docs struct {
		String, GoString, FromString []string
	}

	Values []Value
}

type Value struct {
	Value  *ast.ValueSpec
	String string
	Skip   bool
}

func Main() error {
	input := os.Getenv("GOFILE")
	output := strings.TrimSuffix(input, ".go") + "_enum.go"

	fs := new(token.FileSet)
	f, err := parser.ParseFile(fs, input, nil, parser.ParseComments)
	if err != nil {
		return err
	}

	comments := ast.NewCommentMap(fs, f, f.Comments)

	constsByType := make(map[string][]*ast.ValueSpec)
	var types []*ast.GenDecl

	for _, decl := range f.Decls {
		decl, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}

		switch decl.Tok {
		case token.TYPE:
			if !HasDirectives(comments[decl]) {
				continue
			}

			if decl.Lparen.IsValid() {
				return fmt.Errorf("%v: //enum: directive on type group", fs.Position(decl.TokPos))
			}

			types = append(types, decl)
		case token.CONST:
			var ty string
			for _, spec := range decl.Specs {
				v := spec.(*ast.ValueSpec) //nolint:errcheck
				if v.Type == nil {
					constsByType[ty] = append(constsByType[ty], v)
				}
				if ident, ok := v.Type.(*ast.Ident); ok {
					ty = ident.Name
					constsByType[ty] = append(constsByType[ty], v)
				}
			}
		}
	}

	imports := map[string]struct{}{"fmt": {}}
	enums := make([]Enum, 0, len(types))
	for _, ty := range types {
		enum := Enum{Type: ty.Specs[0].(*ast.TypeSpec)} //nolint:errcheck
		dirs, err := ParseDirectives(fs, comments[ty])
		if err != nil {
			return err
		}
		for _, d := range dirs {
			var ok bool
			switch d.Name {
			case "import":
				path, ok := slicesx.Get(d.Args, 0)
				if !ok {
					return fmt.Errorf("%v: //enum:import requires an argument", fs.Position(d.Pos))
				}
				imports[path] = struct{}{}

			case "string":
				enum.Methods.String, ok = slicesx.Get(d.Args, 0)
				if !ok {
					enum.Methods.String = "String"
					break
				}
				enum.Docs.String = d.Args[1:]

			case "gostring":
				enum.Methods.GoString, ok = slicesx.Get(d.Args, 0)
				if !ok {
					enum.Methods.GoString = "GoString"
					break
				}
				enum.Docs.GoString = d.Args[1:]

			case "fromstring":
				enum.Methods.FromString, ok = slicesx.Get(d.Args, 0)
				if !ok {
					return fmt.Errorf("%v: //enum:fromstring requires an argument", fs.Position(d.Pos))
				}
				enum.Docs.FromString = d.Args[1:]

			case "stringfunc":
				enum.Methods.StringFunc, ok = slicesx.Get(d.Args, 0)
				if !ok {
					return fmt.Errorf("%v: //enum:stringfunc requires an argument", fs.Position(d.Pos))
				}

			case "doc":
				arg, _ := slicesx.Get(d.Args, 0)
				text, _ := slicesx.Get(d.Args, 1)
				switch arg {
				case "string":
					enum.Docs.String = append(enum.Docs.String, text)
				case "gostring":
					enum.Docs.GoString = append(enum.Docs.GoString, text)
				case "fromstring":
					enum.Docs.FromString = append(enum.Docs.FromString, text)
				default:
					return fmt.Errorf("%v: invalid method for //enum:doc: %q", fs.Position(d.Pos), arg)
				}

			default:
				return fmt.Errorf("%v: unknown type directive %q", fs.Position(d.Pos), d.Name)
			}
		}

		if enum.Methods.String != "" && enum.Docs.String == nil {
			enum.Docs.String = []string{"String implements [fmt.Stringer]."}
		}
		if enum.Methods.GoString != "" && enum.Docs.GoString == nil {
			enum.Docs.GoString = []string{"GoString implements [fmt.GoStringer]."}
		}

		for _, v := range constsByType[enum.Type.Name.Name] {
			value := Value{Value: v}
			dirs, err := ParseDirectives(fs, comments[v])
			if err != nil {
				return err
			}
			for _, d := range dirs {
				switch d.Name {
				case "string":
					name, ok := slicesx.Get(d.Args, 0)
					if !ok {
						return fmt.Errorf("%v: //enum:string requires an argument", fs.Position(d.Pos))
					}
					value.String = strconv.Quote(name)
				case "skip":
					value.Skip = true

				default:
					return fmt.Errorf("%v: unknown const directive %q", fs.Position(d.Pos), d.Name)
				}
			}

			enum.Values = append(enum.Values, value)
		}

		enums = append(enums, enum)
	}

	importList := make([]string, 0, len(imports))
	for imp := range imports {
		importList = append(importList, strconv.Quote(imp))
	}
	slices.Sort(importList)

	out, err := os.Create(output)
	if err != nil {
		return err
	}
	defer out.Close()

	info, err := buildinfo.ReadFile(os.Args[0])
	if err != nil {
		return err
	}

	return tmpl.ExecuteTemplate(out, "generated.go.tmpl", struct {
		Binary, Package string
		Imports         []string
		Enums           []Enum
	}{
		Binary:  info.Path,
		Package: f.Name.Name,
		Imports: importList,
		Enums:   enums,
	})
}

func main() {
	if err := Main(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
