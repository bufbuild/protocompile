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

package printer

import "github.com/bufbuild/protocompile/experimental/dom"

// Options controls the formatting behavior of the printer.
type Options struct {
	// Format enables canonical formatting mode. When true, the printer
	// reorders file-level declarations into canonical order (syntax,
	// package, imports, options, defs), sorts imports alphabetically,
	// sorts options (plain before extensions), and normalizes whitespace
	// while preserving comments.
	Format bool

	// The maximum number of columns to render before triggering
	// a break. A value of zero implies an infinite width.
	MaxWidth int

	// The number of columns a tab character counts as. Defaults to 2.
	TabstopWidth int
}

// withDefaults returns a copy of opts with default values applied.
func (opts Options) withDefaults() Options {
	if opts.TabstopWidth == 0 {
		opts.TabstopWidth = 2
	}
	return opts
}

// domOptions converts printer options to dom.Options.
func (opts Options) domOptions() dom.Options {
	return dom.Options{
		MaxWidth:     opts.MaxWidth,
		TabstopWidth: opts.TabstopWidth,
	}
}
