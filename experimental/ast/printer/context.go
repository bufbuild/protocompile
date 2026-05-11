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

// context is the internal formatting behaviours set by the printer based on
// the printed entity.
type context struct {
	// lineToBlock converts trailing // comments to /* */ in contexts
	// where inline tokens follow without a newline break (paths,
	// compact options, option values before `;`).
	lineToBlock bool

	// indentExpr indents compound string parts one level. Set in
	// value contexts after `=` or `:` so multi-part strings break
	// under the assignment.
	indentExpr bool

	// trailingBlockOnNewLine indicates the surrounding scope renders
	// vertically (e.g. a broken bracket scope, or compound-string
	// parts), so trailing /* */ block comments should be placed on
	// their own line rather than inline with the value. Honored by
	// [printer.emitTrailing] only when
	// [Formatting.TrailingBlockCommentsOnNewLine] is true.
	trailingBlockOnNewLine bool

	// pairLeadingBlock indicates the surrounding scope is a broken
	// bracket scope where leading /* */ block comments on elements
	// should be paired with their element on the same line, rather
	// than placed on their own line above. Honored by
	// [printer.emitTrivia] only when
	// [Formatting.PairLeadingBlockComments] is true.
	pairLeadingBlock bool

	// pathInValueContext indicates the current [printer.printPath]
	// emission is for a path used as a value (e.g. `false` in
	// `packed = false`), not as a key, decl name, or extension
	// name. printPath usually resets trailingBlockOnNewLine to keep
	// paths tight, but value-position paths should let a trailing
	// block comment on the path's final token respect the
	// surrounding broken scope's policy.
	pathInValueContext bool
}

// modifier mutates a [context]. Modifiers are applied in order via
// [context.with]; each call site declares which fields it changes by
// composing modifiers, leaving unmentioned fields untouched.
type modifier func(*context)

// lineToBlock returns a [modifier] that sets [context.lineToBlock].
func lineToBlock(v bool) modifier {
	return func(c *context) { c.lineToBlock = v }
}

// indentExpr returns a [modifier] that sets [context.indentExpr].
func indentExpr(v bool) modifier {
	return func(c *context) { c.indentExpr = v }
}

// trailingBlockOnNewLine returns a [modifier] that sets
// [context.trailingBlockOnNewLine].
func trailingBlockOnNewLine(v bool) modifier {
	return func(c *context) { c.trailingBlockOnNewLine = v }
}

// pairLeadingBlock returns a [modifier] that sets
// [context.pairLeadingBlock].
func pairLeadingBlock(v bool) modifier {
	return func(c *context) { c.pairLeadingBlock = v }
}

// pathInValueContext returns a [modifier] that sets
// [context.pathInValueContext].
func pathInValueContext(v bool) modifier {
	return func(c *context) { c.pathInValueContext = v }
}

// with applies the given modifiers to the context and returns a function
// that restores the prior state. Use with defer to scope changes to a
// function body:
//
//	defer p.ctx.with(lineToBlock(true))()
//
// The returned restorer is idempotent and may be called multiple times
// to rewind to the captured state.
func (c *context) with(modifiers ...modifier) func() {
	saved := *c
	for _, m := range modifiers {
		m(c)
	}
	return func() { *c = saved }
}
