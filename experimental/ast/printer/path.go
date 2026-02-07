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

import "github.com/bufbuild/protocompile/experimental/ast"

// printPath prints a path (e.g., "foo.bar.baz" or "(custom.option)") with a leading gap.
func (p *printer) printPath(path ast.Path, gap gapStyle) {
	if path.IsZero() {
		return
	}

	first := true
	for pc := range path.Components {
		// Print separator (dot or slash) if present
		if !pc.Separator().IsZero() {
			p.printToken(pc.Separator(), gapNone)
		}

		// Print the name component
		if !pc.Name().IsZero() {
			componentGap := gapNone
			if first {
				componentGap = gap
				first = false
			}

			if extn := pc.AsExtension(); !extn.IsZero() {
				// Extension path component like (foo.bar).
				// The parens are a scope.
				parens := pc.Name()
				openTok, closeTok := parens.StartEnd()
				slots := p.trivia.scopeSlots(parens.ID())

				p.printToken(openTok, componentGap)
				p.emitSlot(slots, 0)
				p.printPath(extn, gapNone)
				p.emitSlot(slots, 1)
				p.printToken(closeTok, gapNone)
			} else {
				// Simple identifier
				p.printToken(pc.Name(), componentGap)
			}
		}
	}
}
