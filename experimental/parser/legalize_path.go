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
	"github.com/bufbuild/protocompile/experimental/internal/taxa"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/token"
)

// pathOptions is configuration for [legalizePath].
type pathOptions struct {
	// If set, the path must be relative.
	Relative bool

	// If set, the path may contain precisely one `/` separator.
	AllowSlash bool

	// If set, the path may contain extension components.
	AllowExts bool
}

// legalizePath legalizes a path to satisfy the configuration in opts
func legalizePath(p *parser, where taxa.Place, path ast.Path, opts pathOptions) (ok bool) {
	ok = true

	var i int
	var slash token.Token
	path.Components(func(pc ast.PathComponent) bool {
		if i == 0 && opts.Relative {
			if !pc.Separator().IsZero() {
				p.Errorf("unexpected absolute path %s", where).Apply(
					report.Snippetf(path, "expected a path without a leading `%s`", pc.Separator().Text()),
				)
				ok = false
				return false
			}
		}

		if pc.Separator().Text() == "/" {
			if !opts.AllowSlash {
				p.Errorf("unexpected `/` in path %s", where).Apply(
					report.Snippetf(pc.Separator(), "help: replace this with a `.`"),
				)
				ok = false
				return false
			} else if !slash.IsZero() {
				p.Errorf("unexpected `/` in path %s", where).Apply(
					report.Snippet(pc.Separator()),
					report.Snippetf(slash, "previous one is here"),
				)
				ok = false
				return false
			}
			slash = pc.Separator()
		}

		if ext := pc.AsExtension(); !ext.IsZero() {
			if opts.AllowExts {
				ok = legalizePath(p, where, ext, pathOptions{
					Relative:  false,
					AllowExts: false,
				})
				if !ok {
					return false
				}
			} else {
				p.Errorf("unexpected nested extension path %s", where).Apply(
					// Use Name() here so we get the outer parens of the extension.
					report.Snippet(pc.Name()),
				)
				ok = false
				return false
			}
		}

		i++
		return true
	})

	return ok
}
