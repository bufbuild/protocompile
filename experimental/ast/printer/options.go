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
	"math"

	"github.com/bufbuild/protocompile/experimental/dom"
)

// Options controls the printer's overall behavior.
type Options struct {
	// Format enables AST transforms and layout decisions. When false,
	// the printer round-trips the AST as-is: no transforms, no layout
	// decisions, source whitespace preserved verbatim from token
	// trivia. Formatting is ignored.
	Format bool

	// Formatting controls behavior in format mode. Honored only when
	// Format is true.
	Formatting Formatting

	// Edits are applied to the AST in order before formatting. The
	// mutations happen on the file passed to [PrintFile]; a caller
	// wishing to preserve the original AST must clone it first.
	Edits []Edit
}

// Formatting groups all options that drive formatting behavior. Each
// field corresponds to a specific decision the printer makes during
// AST transforms or emission.
//
// Defaults are applied for unset int fields via [Formatting.withDefaults]
// (sentinel-based: zero is treated as unset). Bool fields are not
// auto-defaulted; callers should start from a preset such as
// [LegacyBufFormat] and override individual fields, or use
// Formatting{} to opt out of all default-true behaviors.
type Formatting struct {
	// MaxWidth is the column budget passed to dom.Group for flat-vs-
	// broken layout decisions. dom breaks a group whose flat width
	// exceeds this value. Consulted wherever the printer wraps content
	// in dom.Group.
	//
	// Zero is treated as unset and replaced by the default. Users
	// wanting "no width limit" set explicitly to math.MaxInt.
	//
	// Default: math.MaxInt (transitional; will become 100 once
	// width-aware layout strategies land). Legacy: math.MaxInt.
	MaxWidth int

	// TabstopWidth is the column count for one indentation level.
	// Used as the `by` argument when wrapping content in dom.Indent.
	//
	// Zero is treated as unset and replaced by the default.
	//
	// Default: 2. Legacy: 2.
	TabstopWidth int

	// BodyLayout selects how decl-bearing body scopes (`{ ... }`)
	// decide between flat and broken layout. Applies uniformly to
	// message/enum/service/oneof/extend/RPC-method bodies.
	//
	// Default: LayoutStrict (transitional; will become LayoutDynamic
	// once dynamic body layout is implemented). Legacy: LayoutStrict.
	BodyLayout LayoutStrategy

	// LiteralLayout selects how literal expression scopes decide
	// between flat and broken layout. Applies uniformly to compact
	// options, array literals, and dict (message) literals.
	//
	// Default: LayoutStrict (transitional; will become LayoutDynamic
	// once dynamic literal layout is implemented). Legacy: LayoutStrict.
	LiteralLayout LayoutStrategy

	// CanonicalizeFileOrder applies a canonical multi-tier ordering to
	// top-level file declarations:
	//   1. syntax / edition
	//   2. package
	//   3. imports (alphabetized within group; `import option` after
	//      regular imports per Edition 2024)
	//   4. file-level options (plain before extension, alphabetized
	//      within each subgroup)
	//   5. everything else, in source order
	//
	// Default: true. Legacy: true.
	CanonicalizeFileOrder bool

	// RewriteTrailingLineCommentsToBlock controls how the printer
	// handles trailing `//` line comments.
	//
	// When true, every trailing `//` comment is rewritten as `/* foo */`
	// (with `*/` in the body escaped to `* /` to keep the synthesized
	// block comment sound). Matches legacy `buf format` behavior;
	// modifies user comment text as a side effect.
	//
	// When false, trailing `//` comments are emitted verbatim. If the
	// layout would otherwise render a scope flat such that a `//`
	// trailing would consume following inline content (e.g. a closing
	// `]` or `;`), the layout falls back to broken for that scope
	// instead. Trailing `//` comments at end of a line where nothing
	// follows are simply left alone.
	//
	// Default: true (transitional; will become false once the layout-
	// fallback alternative lands). Legacy: true.
	RewriteTrailingLineCommentsToBlock bool

	// NormalizeBlockComments rewrites the interior whitespace of
	// multi-line `/* */` comments to a canonical form.
	//
	// When true, the prefix-style normalization algorithm runs on
	// every multi-line block comment.
	//
	// When false, multi-line block comments are emitted with their
	// interior whitespace preserved verbatim from source.
	//
	// Default: true (transitional; will become false once the
	// preserve-verbatim alternative is wired up). Legacy: true.
	NormalizeBlockComments bool

	// TrailingBlockCommentsOnNewLine controls placement of trailing
	// `/* */` comments when the surrounding bracket scope is rendered
	// multi-line.
	//
	// When true, trailing block comments are emitted on their own line
	// after the value, separated from the value vertically. Matches
	// legacy `buf format` behavior and also covers compound-string
	// interior block comments.
	//
	// When false, trailing block comments stay inline with their value
	// on the same line.
	//
	// Default: false. Legacy: true.
	TrailingBlockCommentsOnNewLine bool

	// PairLeadingBlockComments controls placement of leading `/* */`
	// comments on elements inside an expanded scope (e.g. an element
	// of an array or dict literal that's been broken multi-line).
	//
	// When true, a leading block comment is rendered on the same line
	// as its element. Matches legacy `buf format` behavior.
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
// literal, RPC signature) decides between flat and broken layout.
type LayoutStrategy int

const (
	// LayoutDynamic preserves source intent as the baseline and lets
	// the dom layer enforce the width budget on top:
	//
	//   - If the source had this scope flat (no newline between open
	//     and close), the printer keeps it flat -- unless its flat
	//     width exceeds MaxWidth, in which case dom breaks it.
	//
	//   - If the source had this scope broken (any newline between
	//     open and close), the printer keeps it broken regardless of
	//     width.
	//
	// Whitespace is tightened within whichever shape applies, but
	// the flat/broken decision respects what the user wrote.
	LayoutDynamic LayoutStrategy = iota

	// LayoutStrict applies hardcoded shape rules per scope-type,
	// regardless of source structure or width. Matches legacy
	// `buf format` behavior.
	LayoutStrict
)

// LegacyBufFormat returns Formatting options that match the legacy
// `buf format` formatter's behavior. Use as:
//
//	printer.PrintFile(printer.Options{
//	    Format:     true,
//	    Formatting: printer.LegacyBufFormat(),
//	}, file)
//
// This is the migration target for buf format replacement.
func LegacyBufFormat() Formatting {
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

// Default returns the recommended Formatting options for new callers.
// Use as:
//
//	printer.PrintFile(printer.Options{
//	    Format:     true,
//	    Formatting: printer.Default(),
//	}, file)
//
// Compared to [LegacyBufFormat], Default uses [LayoutDynamic] for body
// and literal scopes (preserving source intent for flat-vs-broken
// decisions) and turns off the comment-rewriting/repositioning knobs
// that legacy enabled. MaxWidth remains [math.MaxInt] until
// width-aware layout strategies land.
func Default() Formatting {
	return Formatting{
		MaxWidth:                           math.MaxInt,
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
// applied to int fields. Bool fields are not auto-defaulted (Go's
// zero value cannot be distinguished from an explicit false override);
// callers should start from a preset such as [LegacyBufFormat] when
// bool defaults matter.
func (f Formatting) withDefaults() Formatting {
	if f.MaxWidth == 0 {
		f.MaxWidth = math.MaxInt
	}
	if f.TabstopWidth == 0 {
		f.TabstopWidth = 2
	}
	// Delegate any remaining defaulting to dom (shouldn't change
	// anything for these two fields after the explicit assignments
	// above, but keeps the printer in sync with dom's contract).
	dd := f.domOptions().WithDefaults()
	f.MaxWidth = dd.MaxWidth
	f.TabstopWidth = dd.TabstopWidth
	return f
}

// domOptions converts formatting options to dom.Options for the
// width-aware fields. Used by [PrintFile] and [Print] when invoking
// dom.Render.
func (f Formatting) domOptions() dom.Options {
	return dom.Options{
		MaxWidth:     f.MaxWidth,
		TabstopWidth: f.TabstopWidth,
	}
}

// domOptions converts printer options to dom.Options.
func (opts Options) domOptions() dom.Options {
	return opts.Formatting.domOptions()
}
