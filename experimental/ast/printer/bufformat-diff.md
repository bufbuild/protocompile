# Bufformat Golden Test Differences

This document summarizes the intentional differences between the
protocompile printer (`LegacyBufFormat` preset) and the legacy
`buf format` golden files in `TestBufFormat`. It also lists the
remaining mechanical issues that still cause test failures.

## Skipped Tests

### editions/2024
Parser error test, not a printer test. Not applicable.

### deprecate/*
These tests require AST transforms (adding `deprecated` options)
performed by buf's `FormatModuleSet`, not by the printer. The printer
only formats what the AST contains.

### all/v1/all.proto, customoptions/options.proto
Our formatter keeps detached comments at section boundaries during
declaration sorting rather than permuting them with declarations. When
declarations are reordered, comments that were between two
declarations stay at the section boundary rather than moving with the
following declaration. This prevents comments from being silently
associated with the wrong declaration.

### service/v1/service.proto, service/v1/service_options.proto
Our formatter always inserts a space before trailing block comments:
`M /* comment */` vs the golden's `M/* comment */`. Consistent spacing
before trailing comments is more readable and matches the convention
used everywhere else in our output.

## Remaining Failing Tests (4)

The four remaining failures are placement-only differences. The
printer output is valid Protobuf in every case.

### A. Trailing block comment on value in compact-options not on own line

Compact options inside an expanded `[...]` scope want the trailing
block comment on the value to land on its own line (legacy preset
honors `Formatting.TrailingBlockCommentsOnNewLine = true`). It does
for most scopes, but not when the value is a path: `printPath`
resets `trailingBlockOnNewLine` to keep paths tight.

Appears in: `message_options.proto`, `option_complex_array_literal.proto`.

```
golden:  packed = false
         /* trailing comment */

ours:    packed = false /* trailing comment */
```

**Cause:** `printPath` resets `trailingBlockOnNewLine(false)` to
prevent breaks mid-path (avoids a different idempotency regression
documented in Step 2.6). A path used as a *value* should still get
the on-own-line treatment, but the printer can't distinguish path-
as-value from path-as-key without more plumbing.

### B. Blank lines lost in dicts with newline separators or trailing comments

When dict fields aren't comma-separated (or have trailing comments
near commas), pass1 preserves blank lines but pass2 doesn't.

Appears in: `option_complex_array_literal.proto` (idempotency only),
`option_message_field.proto`.

```
pass1:   ]

         values: [
           ...
         ]

pass2:   ]
         values: [
           ...
         ]
```

**Cause:** The trivia walker (literal mode) breaks decls at `,`. For
dicts that use newline-only separators it doesn't break, so
per-element `blankBefore` isn't recorded. Even for comma-separated
fields, post-comma trailing comments shift trivia between pass1 and
pass2 enough to lose blank-line tracking. The simple comma case
(most fixtures) was fixed and now preserves blank lines correctly;
the messy edge cases remain.

### C. `//` vs `/* */` after `}` / `]`

Compact options or dict values with a trailing `//` line comment that
terminates on a newline stay as `//`; the golden rewrites to `/* */`.

Appears in: `literal_comments.proto`.

```
golden:  additional_rules: [] /* comment on , */
ours:    additional_rules: [] // comment on ,
```

**Cause:** Our printer keeps `//` line comments that already terminate
on a newline (e.g. after `}` or `]`) as `//`; the legacy formatter
rewrites them to `/* */`. Stylistic choice — keeping `//` is safe
when nothing else follows on the line and preserves the original
comment style.

### D. Trailing comment placement on commas in array literals

When a `,` between array elements has a trailing block comment
(`{rule: "child"}, /* child node */`), legacy keeps it inline on the
comma's line. Our `TrailingBlockCommentsOnNewLine` pushes it to its
own line.

Appears in: `literal_comments.proto`.

```
golden:  /* Before */ {rule: "child"}, /* child node */
         {

ours:    /* Before */ {rule: "child"},
         /* child node */
         {
```

**Cause:** With Step 9's walker change, post-comma trailing comments
attach to the comma. Step 2.6's `TrailingBlockCommentsOnNewLine`
flag fires for those, putting the comment on its own line. Legacy
buf format treats trailing-on-comma differently from trailing-on-
value: only the latter gets the own-line treatment.

## Summary

| Category | Tests affected | Status |
|----------|---------------|--------|
| (A) Trailing block on value-position path | `message_options`, `option_complex_array_literal` | path-context limitation |
| (B) Dict blank-lines around trailing comments / newline separators | `option_complex_array_literal` (idempotency), `option_message_field` | walker limitation |
| (C) `//` vs `/* */` after `}`/`]` | `literal_comments` | stylistic; keep `//` when safe |
| (D) Trailing-on-comma block placement | `literal_comments` | own-line treatment over-applies to commas |

No comments are dropped, no syntax is broken, and all passing tests
are idempotent. The remaining diffs are placement-only.

## Recently Fixed

- **Trailing block comment placement in expanded brackets** —
  the `Formatting.TrailingBlockCommentsOnNewLine` knob (legacy `true`)
  now drives placement: trailing block comments inside expanded
  bracket scopes go on their own line under the legacy preset. (Step
  2.6.)
- **Missing space before `:` after bracketed extension key with a
  trailing block comment** — `printExprField` peeks at the colon's
  leading trivia and uses `gapSpace` instead of `gapInline` when the
  trivia ends in an inline block comment. (Step 10.)
- **Missing blank lines between comma-separated dict/array entries**
  — the trivia walker in literal scope mode now breaks at `,`,
  producing per-element trivia slots. `printArray`/`printDict`/
  `printCompactOptions` consult `slots.hasBlankBefore(i)` to choose
  `gapBlankline` vs `gapNewline`. (Step 9. Newline-only-separator
  dicts remain — see Remaining (C).)
- **Leading block comment paired with array element** — the
  `Formatting.PairLeadingBlockComments` knob (legacy `true`) inlines
  leading block comments with their array element. (Step 2.7,
  narrowed to `printArray` only.) Dict fields, compact options, and
  compound-string parts now keep their leading block comments on
  their own line, matching legacy.
- **Compound-string interior block comments paired inline** —
  resolved by resetting `pairLeadingBlock` in `printCompoundString`
  so the surrounding scope's flag doesn't leak between parts.
- **Empty `;` decl** (e.g. `message M {};`) no longer collides with
  blank-line preservation; the trivia walker recognizes `;` after a
  body as a separate empty decl.
- **Single-option compact options with a leading `/* */` comment on
  the value** (e.g. `[(opt) = /* Before */ nan /* Trailing */]`) now
  stay inline rather than expanding.
- **Single-option compact options with a leading comment on the key**
  (e.g. `[/* leading */ packed = false]`) now expand to multi-line
  correctly instead of emitting a broken half-expanded form.
- **Single-element dicts** stay inline when they fit.

## Correctness Improvements Over Legacy

These are cases where our output is intentionally not byte-equivalent
to legacy `buf format` because the legacy output is broken protobuf.

- **Inline `//` comment containing `*/` in its body**
  (e.g. `// foo */ bar`) is escaped to `/* foo * / bar */` when
  converting to a block comment. Legacy `bufformat/formatter.go` does
  not check for embedded `*/` and produces `/* foo */ bar */`, which
  terminates the synthesized block comment prematurely and leaks
  `bar */` as text. Our escape preserves syntactic validity at the
  cost of a one-character visual difference in the body.

[pair]: ./options.go
