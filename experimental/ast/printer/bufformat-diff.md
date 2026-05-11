# Bufformat Golden Test Differences

This document summarizes the intentional differences between the
protocompile printer (`LegacyBufFormat` preset) and the legacy
`buf format` golden files in `TestBufFormat`. With the work tracked
here, `TestBufFormat` passes — every divergence is either fixed or
explicitly skipped with a documented reason.

## Skipped Tests

### editions/2024
Parser error test, not a printer test. Not applicable.

### deprecate/*
These tests require AST transforms (adding `deprecated` options)
performed by buf's `FormatModuleSet`, not by the printer. The printer
only formats what the AST contains.

### all/v1/all.proto, customoptions/options.proto (TestBufFormat) and ordering_section_comments.proto (TestFormat)
Our formatter keeps detached comments at section boundaries during
declaration sorting rather than permuting them with declarations. When
declarations are reordered, comments that were between two
declarations stay at the section boundary rather than moving with the
following declaration. This prevents comments from being silently
associated with the wrong declaration.

Example from `all/v1/all.proto` — the source has comments like
`// between-package-and-import comment` BEFORE the imports section.
Legacy `buf format` attaches the comment to the *first* import as a
leading comment so it travels with that import when imports are
sorted; we keep it at the section boundary instead:

```diff
 package all.v1;
+// between-package-and-import comment
+
 import ".../a.proto";
-
-// between-package-and-import comment
 import ".../b.proto";
```

(`+` = legacy golden, `-` = our output.)

`TestFormat/ordering_section_comments.proto` is the same divergence
on a fixture we own.

### service/v1/service.proto, service/v1/service_options.proto
Our formatter always inserts a space before trailing block comments:
`M /* comment */` vs the golden's `M/* comment */`. Consistent spacing
before trailing comments is more readable and matches the convention
used everywhere else in our output.

Example from `service/v1/service.proto`:

```diff
-  rpc Ping(/* Before Request */Message/* After Request */) returns ...
+  rpc Ping(/* Before Request */Message /* After Request */) returns ...
```

(`-` = legacy golden, `+` = our output — note the space before
`/* After Request */`.)

## Correctness Improvements Over Legacy

These are cases where our output is intentionally not byte-equivalent
to legacy `buf format` because the legacy output is broken or
unconventional. Each is exercised by both a `TestBufFormat` skip
(against buf's own testdata) and a `TestFormat` fixture we own.

- **Inline `//` comment containing `*/` in its body**
  (e.g. `// foo */ bar`) is escaped to `/* foo * / bar */` when
  converting to a block comment. Legacy `bufformat/formatter.go` does
  not check for embedded `*/` and produces `/* foo */ bar */`, which
  terminates the synthesized block comment prematurely and leaks
  `bar */` as text. Our escape preserves syntactic validity at the
  cost of a one-character visual difference in the body. Visible in
  `TestFormat/compact_options.proto` (`with_terminator` case).

- **Close `]` placement in broken array literals**: when source
  glues the close bracket to the last element on the same line
  (`{foo: 98}]`), legacy `buf format` preserves that gluing.
  We always emit `]` on its own line in broken layout, which is
  more conventional and easier to read. Visible in
  `TestFormat/message_literals.proto`.

## Recently Fixed

Changelog of legacy-divergence work landed in the printer (kept as a
running record to help track the migration). Items here are *not*
the durable spec — see the sections above for that.

- **Trailing block comment placement in expanded brackets** — the
  `Formatting.TrailingBlockCommentsOnNewLine` knob (legacy `true`)
  drives placement: trailing block comments inside expanded bracket
  scopes go on their own line under the legacy preset.
- **Missing space before `:` after bracketed extension key with a
  trailing block comment** — `printExprField` peeks at the colon's
  leading trivia and uses `gapSpace` instead of `gapInline` when the
  trivia ends in an inline block comment.
- **Missing blank lines between dict/array entries** — the trivia
  walker in literal scope mode breaks at `,`, producing per-element
  trivia slots with `blankBefore` indicators; `printArray`/
  `printDict`/`printCompactOptions` consult them. For the dict case
  where source elides commas (legacy buf format's emitted output
  drops separators), `printDict` falls back to a span-based
  blank-line check (`sourceBlankLineBetweenFields`) so idempotency
  passes.
- **Leading block comment paired with array element** — the
  `Formatting.PairLeadingBlockComments` knob (legacy `true`) inlines
  leading block comments with their array element when they were
  paired in source (`newlineRun == 0` AND `preCommentNewline`).
  Narrowed to `printArray` only; dict fields, compact options, and
  compound-string parts keep their leading block comments separate,
  matching legacy.
- **Compound-string interior block comments paired inline** —
  resolved by resetting `pairLeadingBlock` in `printCompoundString`
  so the surrounding scope's flag doesn't leak between parts.
- **Trailing block comment on a value-position path** — paths in
  value position (set via `pathInValueContext` in compactOptions
  value-emit, array elements, and `printExprField` value-emit) keep
  the surrounding scope's `trailingBlockOnNewLine` so a path's
  final-token trailing block comment respects the broken scope's
  policy. Other path uses (keys, decl names, extension names) reset
  the flag to keep paths tight.
- **Trailing-on-comma block placement** — comma-emit sites in
  literal scopes (`printArray`/`printDict`/`printCompactOptions`)
  suppress `trailingBlockOnNewLine` so a `*/` trailing on the comma
  stays inline. Legacy buf format only puts trailing-on-VALUE block
  comments on their own line, not trailing-on-comma.
- **Trailing `//` on dict-field scope value rewritten to `/* */`** —
  legacy buf format rewrites `// comment` to `/* comment */` when
  the comment trails a dict field whose value is itself an array or
  dict literal (i.e. ends in `]` or `}`), but keeps `//` for trailings
  on primitive values. `printDict`'s broken loop inspects each field's
  `Value().Kind()`; when it's `ExprKindArray`/`ExprKindDict`, it sets
  `lineToBlock(true)` so both the value's close-token trailing and the
  following comma's trailing rewrite. `printArray`/`printDict` capture
  the outer `lineToBlock` at entry (before the scope-reset defer
  fires) and apply `closeTrailingMods` around their close emit so the
  outer policy reaches the boundary token. The same helper forces
  `trailingBlockOnNewLine(false)` for that emit, keeping idempotency:
  on a re-parse where the comma was elided, the comment attaches to
  `]`/`}` directly and must still render inline with its bracket.
- **Source newline preservation between leading block comment and
  element** — `emitTrivia` now correctly counts newlines in
  multi-character space tokens (`"\n  "` is `\n` + indent), and
  preserves `newlineRun >= 1` in the post-comment gap when the outer
  gap was `gapNewline`. Without this, the element + leading block
  comment from source would always render inline regardless of
  layout.
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
