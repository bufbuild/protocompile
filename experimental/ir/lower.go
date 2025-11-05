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
	"sync"

	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/internal/intern"
)

// Session is shared global configuration and state for all IR values that are
// being used together.
//
// It is used to track shared book-keeping.
//
// A zero [Session] is ready to use.
type Session struct {
	intern intern.Table

	once     sync.Once
	builtins builtinIDs
}

// Lower lowers an AST into an IR module.
//
// The ir package does not provide a mechanism for resolving imports; instead,
// they must be provided as an argument to this function.
func (s *Session) Lower(source *ast.File, errs *report.Report, importer Importer) (file *File, ok bool) {
	s.init()

	prior := len(errs.Diagnostics)
	file = &File{session: s, ast: source}
	file.path = file.session.intern.Intern(CanonicalizeFilePath(source.Path()))

	errs.SaveOptions(func() {
		errs.SuppressWarnings = errs.SuppressWarnings || file.IsDescriptorProto()
		lower(file, errs, importer)
	})

	ok = true
	for _, d := range errs.Diagnostics[prior:] {
		if d.Level() >= report.Error {
			ok = false
			break
		}
	}

	return file, ok
}

func (s *Session) init() {
	s.once.Do(func() { s.intern.Preload(&s.builtins) })
}

func lower(file *File, r *report.Report, importer Importer) {
	defer r.CatchICE(false, func(d *report.Diagnostic) {
		d.Apply(report.Notef("while lowering %q", file.Path()))
	})

	// First, build the Type graph for this file.
	(&walker{File: file, Report: r}).walk()

	// Now, resolve all the imports.
	buildImports(file, r, importer)

	generateMapEntries(file, r)

	// Next, we can build various symbol tables in preparation for name
	// resolution.
	buildLocalSymbols(file)
	mergeImportedSymbolTables(file, r)

	// Perform "early" name resolution, i.e. field names and extension types.
	resolveNames(file, r)
	resolveEarlyOptions(file)

	// Perform constant evaluation.
	evaluateFieldNumbers(file, r)

	// Check for number overlaps now that we have numbers loaded.
	buildFieldNumberRanges(file, r)

	// Perform "late" name resolution, that is, options.
	resolveOptions(file, r)

	// Figure out what the option targets of everything is, and validate that
	// those are respected. This requires options to be resolved, and must be
	// done in two separate steps.
	populateOptionTargets(file, r)
	validateOptionTargets(file, r)

	// Build feature info for validating features after they are constructed.
	// Then validate all feature settings throughout the file.
	buildAllFeatureInfo(file, r)
	validateAllFeatures(file, r)

	populateJSONNames(file, r)

	// Validate all the little constraint details that didn't get caught above.
	diagnoseUnusedImports(file, r)
	validateConstraints(file, r)
	checkDeprecated(file, r)
}
