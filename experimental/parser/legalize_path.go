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
	"github.com/bufbuild/protocompile/experimental/token/keyword"
	"github.com/bufbuild/protocompile/internal/ext/iterx"
)

// pathOptions is configuration for [legalizePath].
type pathOptions struct {
	// If set, the path must be relative.
	AllowAbsolute bool

	// If set, the path may contain precisely one `/` separator.
	AllowSlash bool

	// If set, the path may contain extension components.
	AllowExts bool

	// If nonzero, the maximum number of bytes in the path.
	MaxBytes int

	// If nonzero, the maximum number of components in the path.
	MaxComponents int
}

// legalizePath legalizes a path to satisfy the configuration in opts.
func legalizePath(p *parser, where taxa.Place, path ast.Path, opts pathOptions) (ok bool) {
	ok = true

	var bytes, components int
	var slash token.Token
	for i, pc := range iterx.Enumerate(path.Components) {
		bytes += pc.Separator().Span().Len()
		// Just Len() here is technically incorrect, because it could be an
		// extension, but MaxBytes is never used with AllowExts.
		bytes += pc.Name().Span().Len()
		components++

		if i == 0 && !opts.AllowAbsolute && pc.Separator().Text() == "." {
			p.Errorf("unexpected absolute path %s", where).Apply(
				report.Snippetf(path, "expected a path without a leading `%s`", pc.Separator().Text()),
				report.SuggestEdits(path, "remove the leading `.`", report.Edit{Start: 0, End: 1}),
			)
			ok = false
			continue
		}

		if pc.Separator().Keyword() == keyword.Slash {
			if !opts.AllowSlash {
				p.Errorf("unexpected %s in path %s", taxa.Slash, where).Apply(
					report.Snippetf(pc.Separator(), "help: replace this with a %s", taxa.Dot),
				)
				ok = false
				continue
			} else if !slash.IsZero() {
				p.Errorf("type URL can only contain a single %s", taxa.Slash).Apply(
					report.Snippet(pc.Separator()),
					report.Snippetf(slash, "first one is here"),
				)
				ok = false
				continue
			}
			slash = pc.Separator()
		}

		if ext := pc.AsExtension(); !ext.IsZero() {
			if opts.AllowExts {
				ok = legalizePath(p, where, ext, pathOptions{
					AllowAbsolute: true,
					AllowExts:     false,
				})
				if !ok {
					continue
				}
			} else {
				p.Errorf("unexpected nested extension path %s", where).Apply(
					// Use Name() here so we get the outer parens of the extension.
					report.Snippet(pc.Name()),
				)
				ok = false
				continue
			}
		}
	}

	if ok {
		if opts.MaxBytes > 0 && bytes > opts.MaxBytes {
			p.Errorf("path %s is too large", where).Apply(
				report.Snippet(path),
				report.Notef("Protobuf imposes a limit of %v bytes here", opts.MaxBytes),
			)
		} else if opts.MaxComponents > 0 && components > opts.MaxComponents {
			p.Errorf("path %s is too large", where).Apply(
				report.Snippet(path),
				report.Notef("Protobuf imposes a limit of %v components here", opts.MaxComponents),
			)
		}
	}

	return ok
}
