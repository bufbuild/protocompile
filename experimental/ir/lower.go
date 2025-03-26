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

package ir

import (
	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/internal/intern"
)

const (
	tagSymbolRedefined = "symbol-redefined"
)

// Session is shared global configuration and state for all IR values that are
// being used together.
//
// It is used to track shared book-keeping.
//
// A zero [Session] is ready to use.
type Session struct {
	intern intern.Table
}

// Lower lowers an AST into an IR module.
//
// The ir package does not provide a mechanism for resolving imports; instead,
// they must be provided as an argument to this function.
func (s *Session) Lower(source ast.File, errs *report.Report, importer Importer) (file File, ok bool) {
	prior := len(errs.Diagnostics)
	c := &Context{session: s}
	c.ast = source

	lower(c, errs, importer)

	ok = true
	for _, d := range errs.Diagnostics[prior:] {
		if d.Level() >= report.Error {
			ok = false
			break
		}
	}

	return c.File(), ok
}

func lower(c *Context, r *report.Report, importer Importer) {
	defer r.CatchICE(false, func(d *report.Diagnostic) {
		d.Apply(report.Notef("while lowering %q", c.File().Path()))
	})

	// First, build the Type graph for this file.
	(&walker{File: c.File(), Report: r}).walk()

	// Now, resolve all the imports.
	buildImports(c.File(), r, importer)

	// Next, we can build various symbol tables in preparation for name
	// resolution.
	buildLocalSymbols(c.File(), r)
	buildImportedSymbols(c.File(), r)
}

// sorry panics with an NYI error, which turns into an ICE inside of the
// lowering logic.
func sorry(what string) {
	panic("sorry, not yet implemented: " + what)
}
