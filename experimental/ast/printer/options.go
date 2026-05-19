// Copyright 2020-2026 Buf Technologies, Inc.
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
	"math"

	"github.com/bufbuild/protocompile/experimental/dom"
)

// Options controls the printer's overall behavior.
type Options struct {
	// Formatting controls behavior in format mode. Honored only when
	// [Options.Format] is true. Start from a preset such as [Default]
	// or [Legacy] and override individual fields as needed.
	Formatting Formatting

	// Format enables AST transforms and layout decisions driven by
	// [Options.Formatting]. When false, [Options.Formatting] is
	// ignored and the printer round-trips the AST as-is: source
	// whitespace is preserved verbatim from token trivia.
	Format bool
}

// Formatting groups options that drive formatting behavior. Each
// field corresponds to a specific decision the printer makes during
// AST transforms or emission.
//
// Two presets are provided: [Default] (the recommended modern
// behavior) and [Legacy] (byte-for-byte compatibility with
// the legacy protobuf formatter). Construct a zero value directly
// only when opting out of every default-true knob is intended;
// otherwise start from a preset.
type Formatting struct {
	// MaxWidth is the column budget for flat-vs-broken layout
	// decisions: [dom.Group] breaks a group whose flat width exceeds
	// this value. Consulted wherever the printer wraps content in
	// [dom.Group]. Zero is treated as unset and replaced by the
	// default; set to [math.MaxInt] explicitly to disable width-based
	// breaking.
	//
	// Default: 100.
	// Legacy: [math.MaxInt] (the legacy formatter makes no width-based
	// layout decisions).
	MaxWidth int

	// TabstopWidth is the number of spaces in a single indentation
	// level, used by [dom.Indent]. Zero is treated as unset and
	// replaced by the default.
	//
	// Default: 2. Legacy: 2.
	TabstopWidth int

	// BodyLayout selects how decl-bearing body scopes (`{ ... }`)
	// decide between flat and broken layout. Applies uniformly to
	// message, enum, service, oneof, extend, and RPC-method bodies.
	//
	// Default: [LayoutDynamic]. Legacy: [LayoutStrict].
	BodyLayout LayoutStrategy

	// LiteralLayout selects how literal expression scopes decide
	// between flat and broken layout. Applies uniformly to compact
	// options, array literals, and dict (message) literals.
	//
	// Default: [LayoutDynamic]. Legacy: [LayoutStrict].
	LiteralLayout LayoutStrategy

	// CanonicalizeFileOrder applies a canonical multi-tier ordering
	// to top-level file declarations:
	//
	//   1. syntax / edition
	//   2. package
	//   3. imports (alphabetized; `import option` after regular
	//      imports per Edition 2024)
	//   4. file-level options (plain before extension, alphabetized
	//      within each subgroup)
	//   5. everything else, in source order
	//
	// Default: true. Legacy: true.
	CanonicalizeFileOrder bool

	// RewriteTrailingLineCommentsToBlock controls how the printer
	// handles trailing `//` line comments in "tight" contexts —
	// positions where a `//` would consume a following inline token
	// (paths, single-option compact-options values, option values
	// before `;`, and the close-bracket of a dict-field-value scope).
	//
	// When true, trailing `//` comments in those tight contexts are
	// rewritten as `/* ... */` (with `*/` in the body escaped to
	// `* /` to keep the synthesized block comment sound). Trailing
	// `//` comments in safe positions (where the next token is on a
	// new line) are left as `//`. Matches the legacy formatter's
	// behavior; modifies user comment text as a side effect.
	//
	// When false, trailing `//` comments are emitted verbatim. If
	// the layout would otherwise render a scope flat such that a `//`
	// trailing would consume following inline content, the layout
	// falls back to broken for that scope instead.
	//
	// Default: false. Legacy: true.
	RewriteTrailingLineCommentsToBlock bool

	// NormalizeBlockComments rewrites the interior whitespace of
	// multi-line `/* ... */` comments to a canonical form.
	//
	// When true, the prefix-style normalization algorithm runs on
	// every multi-line block comment: if every non-empty interior
	// line begins with the same non-alphanumeric character (e.g.
	// `*`), strip per-line whitespace and re-emit with one space
	// before the prefix character; otherwise unindent by the minimum
	// shared indent and re-indent every line with three spaces.
	// Matches the legacy formatter's behavior; modifies user comment
	// text as a side effect.
	//
	// When false, multi-line block comments are emitted with their
	// interior whitespace preserved verbatim from source.
	//
	// Default: false. Legacy: true.
	NormalizeBlockComments bool

	// TrailingBlockCommentsOnNewLine controls placement of trailing
	// `/* ... */` comments when the surrounding bracket scope is
	// rendered multi-line.
	//
	// When true, trailing block comments are emitted on their own
	// line after the value, separated from the value vertically.
	// Matches the legacy formatter's behavior; also covers compound-
	// string interior block comments.
	//
	// When false, trailing block comments stay inline with their
	// value on the same line.
	//
	// Default: false. Legacy: true.
	TrailingBlockCommentsOnNewLine bool

	// PairLeadingBlockComments controls placement of leading
	// `/* ... */` comments on elements inside an expanded scope (an
	// element of an array or dict literal that has been broken
	// multi-line).
	//
	// When true, a leading block comment is rendered on the same line
	// as its element (`/* Before */ {rule: "child"}`). Matches the
	// legacy formatter's behavior.
	//
	// When false, the comment is placed on its own line above the
	// element.
	//
	// Default: false. Legacy: true.
	PairLeadingBlockComments bool
}

// LayoutStrategy controls how a scope (a syntactic construct that can
// be rendered flat on one line or broken across multiple lines, e.g.
// a message body, compact-options bracket, array literal, dict
// literal, or RPC signature) decides between flat and broken layout.
type LayoutStrategy int

const (
	// LayoutDynamic preserves source intent as the baseline and lets
	// the dom layer enforce the width budget on top:
	//
	//   - If the source had this scope flat (no newline between open
	//     and close), the printer keeps it flat — unless its flat
	//     width exceeds [Formatting.MaxWidth], in which case dom
	//     breaks it.
	//
	//   - If the source had this scope broken (any newline between
	//     open and close), the printer keeps it broken regardless of
	//     width.
	//
	// Whitespace is tightened within whichever shape applies, but
	// the flat/broken decision respects what the user wrote.
	LayoutDynamic LayoutStrategy = iota

	// LayoutStrict applies hardcoded shape rules per scope type,
	// regardless of source structure or width. Matches the legacy
	// protobuf formatter.
	LayoutStrict
)

// Legacy returns [Formatting] options that reproduce the
// legacy protobuf formatter (`buf format`) byte-for-byte. Use this
// preset for drop-in replacement of the legacy formatter:
//
//	printer.PrintFile(printer.Options{
//	    Format:     true,
//	    Formatting: printer.Legacy(),
//	}, file)
//
// For new code, prefer [Default] — see its docstring for the
// differences.
func Legacy() Formatting {
	return Formatting{
		MaxWidth:                           math.MaxInt,
		TabstopWidth:                       2,
		BodyLayout:                         LayoutStrict,
		LiteralLayout:                      LayoutStrict,
		CanonicalizeFileOrder:              true,
		RewriteTrailingLineCommentsToBlock: true,
		NormalizeBlockComments:             true,
		TrailingBlockCommentsOnNewLine:     true,
		PairLeadingBlockComments:           true,
	}
}

// Default returns the recommended [Formatting] options for new
// callers:
//
//	printer.PrintFile(printer.Options{
//	    Format:     true,
//	    Formatting: printer.Default(),
//	}, file)
//
// Compared to [Legacy], Default uses [LayoutDynamic] for
// body and literal scopes (preserving source intent for flat-vs-
// broken decisions) and disables the four comment-rewriting and
// comment-repositioning knobs that the legacy formatter enabled.
func Default() Formatting {
	return Formatting{
		MaxWidth:                           100,
		TabstopWidth:                       2,
		BodyLayout:                         LayoutDynamic,
		LiteralLayout:                      LayoutDynamic,
		CanonicalizeFileOrder:              true,
		RewriteTrailingLineCommentsToBlock: false,
		NormalizeBlockComments:             false,
		TrailingBlockCommentsOnNewLine:     false,
		PairLeadingBlockComments:           false,
	}
}

// withDefaults returns a copy of opts with default values applied.
func (opts Options) withDefaults() Options {
	opts.Formatting = opts.Formatting.withDefaults()
	return opts
}

// withDefaults returns a copy of f with sentinel-based defaults
// applied to int fields. Bool and [LayoutStrategy] fields are not
// auto-defaulted: Go's zero value cannot be distinguished from an
// explicit override, so callers should start from a preset such as
// [Default] or [Legacy] when those fields matter.
func (f Formatting) withDefaults() Formatting {
	if f.MaxWidth == 0 {
		f.MaxWidth = 100
	}
	if f.TabstopWidth == 0 {
		f.TabstopWidth = 2
	}
	// Delegate any remaining int-field defaulting to dom so the
	// printer stays in sync with [dom.Options]'s contract.
	dd := f.domOptions().WithDefaults()
	f.MaxWidth = dd.MaxWidth
	f.TabstopWidth = dd.TabstopWidth
	return f
}

// domOptions converts formatting options to [dom.Options] for the
// width-aware fields. Used by [PrintFile] and [Print] when invoking
// [dom.Render].
func (f Formatting) domOptions() dom.Options {
	return dom.Options{
		MaxWidth:     f.MaxWidth,
		TabstopWidth: f.TabstopWidth,
	}
}

// domOptions converts printer options to [dom.Options].
func (opts Options) domOptions() dom.Options {
	return opts.Formatting.domOptions()
}
