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
	// Indent is the string used for each level of indentation.
	// Defaults to two spaces if empty.
	Indent string

	// MaxWidth is the maximum line width before the printer attempts
	// to break lines. A value of 0 means no limit.
	MaxWidth int

	// Format, when true, normalizes whitespace according to formatting rules.
	// When false (default), preserves original whitespace.
	Format bool
}

// withDefaults returns a copy of opts with default values applied.
func (opts Options) withDefaults() Options {
	if opts.Indent == "" {
		opts.Indent = "  "
	}
	return opts
}

// domOptions converts printer options to dom.Options.
func (opts Options) domOptions() dom.Options {
	return dom.Options{
		MaxWidth:     opts.MaxWidth,
		TabstopWidth: len(opts.Indent),
	}
}
