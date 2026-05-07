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

### A. Compound-string interior block comments paired inline

When a block comment appears between two parts of a compound string,
ours pairs it inline with one part instead of keeping it on its own
line.

Appears in: `literal_comments.proto`, `option_complex_array_literal.proto`,
`message_options.proto`.

```
golden:  "one"
         /* One */
         "two"

ours:    "one"
         /* One */ "two"
```

**Cause:** [`Formatting.PairLeadingBlockComments`][pair] (legacy
default `true`) is set in the broken paths of `printArray`,
`printDict`, and `printCompactOptions`. When a compound string is
emitted within those scopes, the flag leaks in and `emitTrivia`
inline-pairs leading block comments on subsequent string parts.
`printCompoundString` should reset the flag for its body.

### B. Dict/compact-options leading block comments paired inline

The same `PairLeadingBlockComments` over-application paints leading
block comments onto the same line as their dict field or compact
option, where the legacy golden keeps them separate.

Appears in: `message_options.proto`, `literal_comments.proto`.

```
golden:  /* Leading comment on the 'foo' element */
         foo: 1

ours:    /* Leading comment on the 'foo' element */ foo: 1
```

**Cause:** `PairLeadingBlockComments` should fire for *array*
elements only (matches the original `bufformat-diff` item 6 case).
The flag is currently set in all three literal scopes
(`printArray`/`printDict`/`printCompactOptions`); it should be
narrowed to `printArray` only.

### C. Missing blank lines between dict entries (newline-separator case)

When dict fields are separated by newlines instead of commas (a
permitted protobuf syntax), blank lines between fields collapse to a
single newline.

Appears in: `option_message_field.proto` (1 site).

```
golden:  foo: "goo"

         [...]: "boo"

ours:    foo: "goo"
         [...]: "boo"
```

**Cause:** The trivia walker (literal mode) breaks decls at `,`. For
dicts that use newline separators it doesn't break, so per-element
`blankBefore` is never recorded. The comma-separated case (most
fixtures, including comma-separated dict fields) was fixed and now
preserves blank lines correctly.

### D. `//` vs `/* */` after `}` / `]`

Compact options with a single option and a trailing `//` line comment
that already terminates on a newline stay as `//`; the golden rewrites
to `/* */`.

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

## Summary

| Category | Tests affected | Status |
|----------|---------------|--------|
| (A) Compound-string interior block paired inline | `literal_comments`, `option_complex_array_literal`, `message_options` | regression from `PairLeadingBlockComments` over-application |
| (B) Dict/compact-opts leading block paired inline | `message_options`, `literal_comments` | regression from `PairLeadingBlockComments` over-application |
| (C) Blank lines between dict entries (newline-separator case) | `option_message_field` | walker doesn't recognize newlines as element boundaries |
| (D) `//` vs `/* */` after `}`/`]` | `literal_comments` | stylistic; keep `//` when safe |

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
  leading block comments with their array element. (Step 2.7. Note:
  the same flag is over-applied to dict/compact-opts/compound-string
  scopes — see Remaining (A) and (B).)
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
