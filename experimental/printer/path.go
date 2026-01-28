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

import (
	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/token/keyword"
)

// printPath prints a path (e.g., "foo.bar.baz" or "(custom.option)").
func (p *printer) printPath(path ast.Path) {
	if path.IsZero() {
		return
	}

	for pc := range path.Components {
		// Print separator (dot or slash) if present
		if !pc.Separator().IsZero() {
			p.printToken(pc.Separator())
		}

		// Print the name component
		if !pc.Name().IsZero() {
			if extn := pc.AsExtension(); !extn.IsZero() {
				// Extension path component like (foo.bar)
				// The Name() token is the fused parens containing the extension path
				nameTok := pc.Name()
				if !nameTok.IsSynthetic() && !nameTok.IsLeaf() {
					p.printFusedBrackets(nameTok, func(child *printer) {
						child.printPath(extn)
					})
				} else {
					// Synthetic - emit manually
					p.text(keyword.LParen.String())
					p.printPath(extn)
					p.text(keyword.RParen.String())
				}
			} else {
				// Simple identifier
				p.printToken(pc.Name())
			}
		}
	}
}
