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
	"github.com/bufbuild/protocompile/experimental/seq"
)

func legalizeCompactOptions(p *parser, opt ast.CompactOptions) {
	opts := opt.Entries()
	if opts.Len() == 0 {
		p.Errorf("%s cannot be empty", taxa.CompactOptions).Apply(
			report.Snippetf(opt, "help: remove this"),
		)
		return
	}

	seq.Values(opts)(func(opt ast.Option) bool {
		legalizeOptionEntry(p, opt, opt.Span())
		return true
	})
}

func legalizeOptionEntry(p *parser, opt ast.Option, span report.Span) {
	// We can't perform type-checking yet, so all we can really do here
	// is check that the path is ok for an option. Legalizing the value cannot
	// happen until type-checking in IR construction.

	if opt.Path.IsZero() {
		p.Errorf("missing %v path", taxa.Option).Apply(
			report.Snippet(span),
		)
		return
	}
	legalizePath(p, taxa.Option.In(), opt.Path, pathOptions{
		Relative:  true,
		AllowExts: true,
	})

	if opt.Value.IsZero() {
		p.Errorf("missing %v", taxa.OptionValue).Apply(
			report.Snippet(span),
		)
	}
}
