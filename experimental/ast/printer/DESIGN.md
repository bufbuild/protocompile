# Printer Architecture & Configuration Design

This document captures the design direction for the `experimental/ast/printer`
package: how the printer is structured, where formatting decisions live, what
configuration surface looks like, and the migration path from the current
hardcoded design to a configurable formatter that can replace legacy
`buf format`.

This doc is the design output of an architectural discussion that ran
alongside a code-review sweep of the package; it complements
`bufformat-diff.md` (the catalogue of intentional behavioral divergences
from legacy `buf format`).

## Goals

1. **Round-trip fidelity** — printing a parsed AST without formatting should
   reproduce the source byte-for-byte.
2. **Configurable formatting** — formatting opinions are exposed as options;
   the printer itself contains no hardcoded style decisions.
3. **Buf format replacement** — a preset configuration (`LegacyBufFormat`)
   replicates the legacy formatter's behavior, enabling migration without
   churn for existing users.
4. **Clear separation of concerns** — printing (AST traversal + dom emission)
   is distinct from formatting (decisions about layout, sort, normalization).

## Non-goals

- **Strict package separation.** "Printer" and "formatter" are conceptual
  layers, not separate Go packages. Splitting them apart yields more
  package-boundary friction than benefit; the separation is achieved by
  organizing decisions through `Options`, not through imports.
- **Pluggable layout engines.** The dom package handles layout primitives;
  there is no plan to support multiple layout backends.

## Pipeline

The printer operates in three sequential phases. Each phase has a distinct
responsibility, and each later phase depends only on the output of the
previous phase, not on the configuration that produced it.

```
   ┌──────────────────┐    ┌──────────────────┐    ┌──────────────────┐
   │      Edits       │ -> │  AST Transforms  │ -> │ Layout & Emission │
   │ (user mutations) │    │ (sort, normalize)│    │ (dom tag stream)  │
   └──────────────────┘    └──────────────────┘    └──────────────────┘
                                                            │
                                                            v
                                                   ┌─────────────────┐
                                                   │   dom render    │
                                                   │  (string output)│
                                                   └─────────────────┘
```

### Phase 1: Edits

User-driven mutations to the AST tree (insert, delete, move declarations).
Edits produce a possibly-modified AST. Edits run **before** sort because
sort needs to operate on the final set of declarations, including those
inserted or moved by Edit.

Edits are not yet implemented in the printer package; the existing AST-edit
helpers in `printer_test.go` are the reference implementation that will be
lifted into the package proper.

### Phase 2: AST Transforms

Pre-emission transformations of the AST that depend only on the AST and
options. Each transform takes an AST and returns a possibly-rewritten AST.

Currently the only transform is `sortFileDeclsForFormat`, which canonically
orders top-level declarations (syntax/edition, package, imports, file
options, then everything else in source order). Future transforms could
include: deduplication of sorted entries, normalization of equivalent forms,
etc.

This phase is opt-in via `Formatting.CanonicalizeFileOrder` (and any
future per-transform options).

### Phase 3: Layout & Emission

Walks the (possibly-transformed) AST and pushes dom tags into a sink. Layout
decisions — when to break, when to indent, when to convert `//` to `/* */`,
etc. — are made here, driven by `Formatting` options.

Some decisions delegate to dom: `dom.Group` decides flat-vs-broken based on
`Formatting.MaxWidth`. Other decisions are made by the printer directly
(comment style conversion, structural expansion thresholds). The choice of
which decisions delegate to dom is explicit in the code: the dispatch site
names the decision and selects between strategies (e.g., width-aware via
dom vs. structural via hardcoded shape).

After this phase, dom renders the tag stream to a string. dom's behavior is
controlled by `dom.Options`, derived from `Formatting.MaxWidth` and
`Formatting.TabstopWidth`.

## Options surface

```go
// Options controls the printer's overall behavior.
type Options struct {
    // Format enables Phase 2 (AST transforms) and Phase 3 layout
    // decisions. When false, the printer round-trips the AST as-is:
    // no transforms, no layout decisions, source whitespace preserved
    // verbatim from token trivia. Formatting is ignored.
    Format bool

    // Formatting controls behavior in format mode. Honored only when
    // Format is true.
    Formatting Formatting
}

// Formatting groups all options that drive formatting behavior.
// Each field corresponds to a specific decision the printer makes
// during transforms or emission.
type Formatting struct {
    // --- Layout / dom-related ---

    // MaxWidth is the column budget passed to dom.Group for flat-vs-
    // broken layout decisions. dom breaks a group whose flat width
    // exceeds this value. Consulted wherever the printer wraps content
    // in dom.Group (e.g. RPC method signatures, width-aware layout
    // strategies for compact options / arrays / dicts).
    //
    // Zero is treated as unset and replaced by the default. Users
    // wanting "no width limit" set explicitly to math.MaxInt.
    //
    // Default: 100. Legacy: math.MaxInt (legacy buf format does not
    // make width-based layout decisions).
    MaxWidth int

    // TabstopWidth is the column count for one indentation level. Used
    // as the `by` argument when wrapping content in dom.Indent.
    //
    // Zero is treated as unset and replaced by the default.
    //
    // Default: 2. Legacy: 2.
    TabstopWidth int

    // BodyLayout selects how scopes that are decl-bearing bodies (`{ ... }`)
    // decide between flat and broken layout. This applies uniformly to:
    //   - message bodies
    //   - enum bodies
    //   - service bodies
    //   - oneof bodies
    //   - extend bodies
    //   - RPC method bodies
    //
    // Default: LayoutDynamic. Legacy: LayoutStrict.
    BodyLayout LayoutStrategy

    // LiteralLayout selects how literal expression scopes decide between
    // flat and broken layout. This applies uniformly to:
    //   - compact options:    [opt = val, ...]
    //   - array literals:     [a, b, c]
    //   - dict (message)
    //     literals:           {key: value, ...}
    //
    // Default: LayoutDynamic. Legacy: LayoutStrict.
    LiteralLayout LayoutStrategy

    // (Other scopes -- RPC signature parens, extension-path parens --
    // are not configurable. Their internal behavior is to be settled
    // when we inspect test sources during implementation: either
    // LayoutDynamic uniformly, or bespoke strict shapes per scope-type.
    // Both options are functionally equivalent for typical inputs;
    // the call depends on edge cases we surface from real protos.)

    // --- AST transforms ---

    // CanonicalizeFileOrder applies a canonical multi-tier ordering to
    // top-level file declarations:
    //   1. syntax / edition
    //   2. package
    //   3. imports (alphabetized within group; `import option` after
    //      regular imports per Edition 2024)
    //   4. file-level options (plain before extension, alphabetized
    //      within each subgroup)
    //   5. everything else, in source order
    // Default: true. Legacy: true.
    CanonicalizeFileOrder bool

    // --- Comment handling ---

    // RewriteTrailingLineCommentsToBlock controls how the printer
    // handles trailing `//` line comments.
    //
    // When true, every trailing `//` comment is rewritten as
    // `/* foo */` (with `*/` in the body escaped to `* /` to keep
    // the synthesized block comment sound). This matches legacy
    // `buf format` behavior, which uniformly converts all trailing
    // line comments regardless of whether the conversion is required
    // for syntactic correctness; it modifies user comment text as a
    // side effect.
    //
    // When false, trailing `//` comments are emitted verbatim. If
    // the layout would otherwise render a scope flat such that a
    // `//` trailing would consume following inline content (e.g. a
    // closing `]` or `;`), the layout falls back to broken for that
    // scope instead. Trailing `//` comments at end of a line where
    // nothing follows (e.g. a comment after `]` immediately before
    // a newline) are simply left alone.
    //
    // Default: false. Legacy: true.
    RewriteTrailingLineCommentsToBlock bool

    // NormalizeBlockComments rewrites the interior whitespace of
    // multi-line `/* */` comments to a canonical form.
    //
    // The algorithm:
    //
    //   - If every non-empty interior line begins with the same
    //     non-alphanumeric character (e.g. `*`, `=`, `#`), treat the
    //     comment as "prefix style": strip all leading whitespace
    //     per line and re-emit with one space before the prefix
    //     character.
    //
    //   - Otherwise, treat as "plain style": compute the minimum
    //     visual indent across non-empty interior lines, unindent
    //     by that amount, then re-indent every line with three
    //     spaces.
    //
    // When true, the algorithm runs on every multi-line block
    // comment. Matches legacy `buf format` behavior; modifies user
    // comment text as a side effect.
    //
    // When false, multi-line block comments are emitted with their
    // interior whitespace preserved verbatim from source.
    //
    // Default: false. Legacy: true.
    NormalizeBlockComments bool

    // --- Style (legacy-divergence items) ---

    // TrailingBlockCommentsOnNewLine controls placement of trailing
    // `/* */` comments when the surrounding bracket scope is rendered
    // multi-line (i.e., the layout has decided "broken").
    //
    // When true, trailing block comments are emitted on their own line
    // after the value, separated from the value vertically. Matches
    // legacy `buf format` behavior and also covers compound-string
    // interior block comments (which have the same underlying cause).
    //
    // When false, trailing block comments stay inline with their
    // value on the same line.
    //
    // Default: false. Legacy: true.
    TrailingBlockCommentsOnNewLine bool

    // PairLeadingBlockComments controls placement of leading `/* */`
    // comments on elements inside an expanded scope (e.g. an element
    // of an array or dict literal that's been broken multi-line).
    //
    // When true, a leading block comment is rendered on the same line
    // as its element: `/* Before */ {rule: "child"}`. Matches legacy
    // `buf format` behavior; visually pairs the comment with what it
    // is commenting on.
    //
    // When false, the comment is placed on its own line above the
    // element. Both readings are legitimate; this default favors
    // uniform per-line elements over per-element comment pairing.
    //
    // Default: false. Legacy: true.
    PairLeadingBlockComments bool
}

// LayoutStrategy controls how a scope (a syntactic construct that can
// be rendered flat on one line or broken across multiple lines, e.g.
// a message body, compact-options bracket, array literal, dict literal,
// RPC signature) decides between flat and broken layout.
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
    // Whitespace is tightened within whichever shape applies (runs
    // of spaces collapse to canonical), but the flat/broken decision
    // respects what the user wrote.
    LayoutDynamic LayoutStrategy = iota

    // LayoutStrict applies hardcoded shape rules per scope-type,
    // regardless of source structure or width. Examples:
    //
    //   - Compact options with >=2 entries: always expanded, one
    //     per line. With 1 entry: always inline.
    //
    //   - Body scopes ({decl, decl, ...}) with any content: always
    //     expanded. Empty bodies: always collapsed to "{}".
    //
    // This matches legacy `buf format` behavior.
    LayoutStrict
)
```

### Defaults

`Options{}` zero-value: `Format=false, Formatting=Formatting{}` — round-trip
mode. The Formatting struct is unused so its zero value is irrelevant.

When `Format=true` and `Formatting` is zero-valued (or partially set), the
printer applies a per-field defaulting pass before honoring the value —
analogous to `dom.Options.WithDefaults`. Each field's default represents
the printer's preferred / "modern" behavior; the per-field defaults are
documented on each field below. Callers may override any field by setting
it explicitly.

For "behave like legacy buf format" use the preset:

```go
// LegacyBufFormat returns a Formatting value that matches the legacy
// `buf format` formatter's behavior. Use as:
//
//   printer.PrintFile(printer.Options{
//       Format:     true,
//       Formatting: printer.LegacyBufFormat(),
//   }, file)
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
```

A future `Default()` (or `Modern()`) preset would surface the printer's
preferred modern defaults — e.g., width-aware layouts, configurable
comment handling.

### Schema summary

At-a-glance view of all `Formatting` fields, with their default values
and the values used by `LegacyBufFormat()`:

| Field                                | Type             | Default         | Legacy           |
|--------------------------------------|------------------|-----------------|------------------|
| `MaxWidth`                           | `int`            | `100`           | `math.MaxInt`    |
| `TabstopWidth`                       | `int`            | `2`             | `2`              |
| `BodyLayout`                         | `LayoutStrategy` | `LayoutDynamic` | `LayoutStrict`   |
| `LiteralLayout`                      | `LayoutStrategy` | `LayoutDynamic` | `LayoutStrict`   |
| `CanonicalizeFileOrder`              | `bool`           | `true`          | `true`           |
| `RewriteTrailingLineCommentsToBlock` | `bool`           | `false`         | `true`           |
| `NormalizeBlockComments`             | `bool`           | `false`         | `true`           |
| `TrailingBlockCommentsOnNewLine`     | `bool`           | `false`         | `true`           |
| `PairLeadingBlockComments`           | `bool`           | `false`         | `true`           |

`LayoutStrategy` enum values: `LayoutDynamic`, `LayoutStrict`.

## Decision dispatch in code

Layout and emission decisions consult `p.options.Formatting` at the point
of decision, with the strategy named explicitly:

```go
func (p *printer) printCompactOptions(co ast.CompactOptions) {
    // ... setup ...

    switch p.options.Formatting.LiteralLayout {
    case LayoutStrict:
        p.compactOptionsStrict(co, ...)
    case LayoutDynamic:
        p.compactOptionsDynamic(co, ...)
    }
}
```

This makes it visible to a reader that:
1. There's a decision being made here.
2. The decision is configurable.
3. The strategy is named, not buried in branching logic.

## Migration plan

The transition from the current code to this design is incremental: each
option lands as a self-contained change. At every step, default behavior
preserves the existing output (the `LegacyBufFormat` preset captures
"act like today").

### Step 0: groundwork (after current sweep)
- Land this design doc.
- No code changes yet.

### Step 1: introduce `Formatting` struct, deprecate single `Format` boolean
- Add `Formatting` field to `Options`.
- `Format=true` now requires explicit Formatting (or defaults to a
  no-op Formatting). LegacyBufFormat preset replicates current behavior.
- Update internal usage to consult `Formatting.X` instead of
  `Options.Format` for specific decisions.
- Tests pass with no golden updates (LegacyBufFormat is the default-or-
  near-default).

### Step 2: per-knob migrations
For each option in the inventory, in roughly this order:
1. `CanonicalizeFileOrder` — easiest; lift `sortFileDeclsForFormat` call
   to consult the option.
2. `LiteralLayout` — introduce the strategy enum, implement
   `LayoutDynamic` for compact options, array literals, and dict
   literals. Default stays `LayoutStrict` initially; flipping to
   `LayoutDynamic` happens in a later step once non-conformance with
   legacy is acceptable.
3. `BodyLayout` — same pattern, applied uniformly to message/enum/
   service/oneof/extend/RPC-method bodies.
4. `RewriteTrailingLineCommentsToBlock` — gate the existing `lineToBlock`
   rewrite. Modern default (`false`) requires implementing the layout
   fallback for scopes containing `//` trailings.
5. `NormalizeBlockComments` — gate the prefix-style normalization.
6. `TrailingBlockCommentsOnNewLine` — implement the placement variant
   for trailing block comments inside expanded scopes (also covers
   compound-string interior block comments).
7. `PairLeadingBlockComments` — implement the placement variant for
   leading block comments on elements inside expanded scopes.

In parallel with these knob migrations, two non-knob behavioral
improvements (deferred from `bufformat-diff.md`) should land at some
point — these benefit both presets and don't need configuration:

- Walker support for blank lines between dict/array entries
  (`bufformat-diff.md` item 2).
- Gap logic awareness of "last emit was a trailing comment" so the
  space before a following `:` is correct (`bufformat-diff.md` item 4).

Each step:
- Adds a new option with default = current behavior.
- Adds a test case exercising the alternative behavior.
- Updates `LegacyBufFormat()` if a default differs from legacy.
- May add a section to `bufformat-diff.md` for new divergences.

### Step 3: implement Edits
- Phase 1 of the pipeline. Lift `applyEdit` and helpers from
  `printer_test.go` into the package; expose as a public API.
- Edits run before transforms; design ensures sort sees the final tree.

### Step 4: settle the modern Default preset
- Enumerate the printer's preferred defaults.
- Document the divergences from `LegacyBufFormat`.
- Provide as a preset alongside `LegacyBufFormat`.

## Open questions

1. **`Format` boolean's long-term role**: does it stay as the high-level
   "do anything formatting-y" toggle, or does it eventually go away in
   favor of nil-vs-set `Formatting`? Lean: keep as the toggle for clarity.

2. **Edit API shape**: separate `EditFile(file, edits) -> *ast.File`
   followed by `PrintFile(opts, file)`, or a combined
   `PrintFile(opts, file, edits)`? Lean: separate; keeps the pipeline
   phases independent and edits inspectable.

3. **Legacy-conformance test corpus**: do we vendor a curated subset of
   buf format's testdata into this repo (per earlier "TestBufFormat
   vendoring" discussion), and run it under
   `Options{Format: true, Formatting: LegacyBufFormat()}`? Lean: yes,
   once Step 1 lands; it becomes the conformance test for the legacy
   preset.

4. **Minor-scope layout** (RPC signature parens, extension-path parens,
   etc.): do these always behave `LayoutDynamic` internally, or do we
   apply bespoke strict shapes per scope-type? Functionally equivalent
   for typical input; defer the call until we have concrete test cases
   that exercise the difference.

## Relationship to other docs

- **`bufformat-diff.md`** — the *catalogue* of behavioral differences
  from legacy buf format. Stays as runtime behavior documentation;
  may shrink as legacy-conformance options absorb individual
  divergences.
- **`doc.go`** — the package's user-facing godoc. Will be expanded
  as the configurable surface lands; the *concepts* described here
  (pipeline, Format/Formatting, presets) become the package's public
  conceptual model.
