# Bufformat Golden Test Differences

This document summarizes the intentional differences between the
protocompile printer and the old `buf format` golden files in
`TestBufFormat`. It also lists the remaining mechanical issues that
still cause test failures.

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

The four remaining failures are all variations of the same small set
of stylistic differences. The printer output is valid Protobuf and
idempotent in every case; only whitespace placement around comments
differs.

### 1. Trailing block comment placement in expanded brackets

When a value has a trailing `/* */` comment and the surrounding
brackets are rendered multi-line, the golden puts the comment on its
own line; ours keeps it inline with the value.

Appears in: `message_options.proto` (3 sites), `literal_comments.proto`
(1 site), `option_complex_array_literal.proto` (1 site).

```
golden:  packed = false
         /* trailing comment */

ours:    packed = false /* trailing comment */
```

**Cause:** `emitTrailing` always emits trailing comments inline with a
leading space. It has no awareness that its containing bracket/brace
scope has been expanded multi-line.

**Rationale:** Our behavior preserves the relationship between the
value and its trailing comment (they were adjacent in the source and
stay adjacent in our output). The golden's policy of always splitting
trailing block comments in expanded scopes is stylistically arguable;
fixing it would require threading "scope is expanded" context down to
`emitTrailing`. Low priority.

### 2. Missing blank lines between dict/array entries

Inside message-literal dicts or array literals, blank lines between
entries are collapsed to a single newline.

Appears in: `option_complex_array_literal.proto` (2 sites),
`option_message_field.proto` (1 site).

```
golden:  ]

         values: [
           ...
         ]

         recursive: [

ours:    ]
         values: [
           ...
         ]
         recursive: [
```

**Cause:** `printDict` and `printArray` iterate elements with a fixed
`gapNewline`. Unlike `printScopeDecls` (file/body scopes), they do not
consult `trivia.hasBlankBefore(i)` to preserve blank lines. More
fundamentally, `walkDecl` does not break at `,` (or at the newline
that serves as an implicit separator in message literals), so no
per-element `blankBefore` is even recorded.

**Rationale:** Preserving blank lines inside message literals would
require treating each dict element as its own "sub-declaration" in the
trivia walker. Worth doing but non-trivial.

### 3. Single-compact-option trailing block comment expansion

Compact options with a single option and trailing/after-value block
comments stay inline in the golden but expand in ours (when the
expansion trigger also fires for other reasons).

Appears in: `literal_comments.proto` — bracket expansion on
`[]`+trailing-comment patterns.

```
golden:  additional_rules: [] /* comment on , */

ours:    additional_rules: [] // comment on ,    (line-to-block not applied)
```

**Cause:** Our printer keeps `//` line comments that already terminate
on a newline (e.g. after `}` or `]`) as `//` rather than rewriting
them to `/* */`. The golden rewrites.

**Rationale:** Keeping `//` is safe when nothing else follows on the
line, and preserves the original comment style. Stylistic choice.

### 4. Missing space before colon after bracketed extension key

When a message-literal key is an extension name `[...]` with a
trailing block comment, our output has no space between the comment
and the following `:`.

Appears in: `option_message_field.proto`.

```
golden:  [/* One */ foo.bar..._garblez /* Two */] /* Three */ : "boo"

ours:    [/* One */ foo.bar..._garblez /* Two */] /* Three */: "boo"
```

**Cause:** The colon uses `gapInline`, which produces no gap when
there's nothing in pending. The `/* Three */` comment was emitted as
trailing on the `]` via `emitTrailing` and is therefore not visible to
the subsequent `emitTrivia(gapInline)` call on the colon. The gap
logic cannot know the prior emission ended with a comment.

**Rationale:** Fixing would require tracking "last emit was a comment"
on the printer, which is a fair amount of plumbing for one edge case.
Low priority.

### 5. Compound-string trailing block comment attachment

A block comment between two adjacent string parts that in the source
sat on its own line attaches as trailing on the preceding part and
renders inline.

Appears in: `option_complex_array_literal.proto`,
`literal_comments.proto`, `message_options.proto`.

```
source:  "two"
         /* Two */
         "three"

golden:  "two"
         /* Two */
         "three"

ours:    "two" /* Two */
         "three"
```

**Cause:** The trivia walker assigns the post-"two" trivia `[\n, /*
Two */, \n]` to `"three"`'s leading, but the `splitDetached` logic
does not split (only one blank-line boundary is required; consecutive
single newlines don't count). So when `"three"` is printed, its
leading emits the comment with a gap that becomes inline in the flat
handling.

Really the same underlying issue as (1): block comment placement
between items in an expanded scope.

### 6. `/* Before */ {}` in array literal stays inline

Leading block comment on an array element is kept on the same line as
the element by the golden, but we break it onto its own line.

Appears in: `literal_comments.proto`.

```
golden:  [
           /* Before */ {}, // Trailing
           /* Before */ {rule: "child"}, /* child node */
           ...
         ]

ours:    [
           /* Before */
           {}, // Trailing
           /* Before */
           {rule: "child"}, /* child node */
           ...
         ]
```

**Cause:** The element's leading trivia has a comment; when printed
with `gapNewline`, the post-comment gap becomes a softbreak → newline
in the broken array context.

**Rationale:** Both readings are legitimate. The golden's compact form
pairs the comment visually with its element.

## Summary

| Category | Tests affected | Our choice |
|----------|---------------|------------|
| Trailing block comment in expanded bracket | `message_options`, `literal_comments`, `option_complex_array_literal` | Inline with value (same line) |
| Blank lines between dict/array entries | `option_complex_array_literal`, `option_message_field` | Collapsed to newline (trivia walker does not track per-element blank lines) |
| `//` vs `/* */` after `}`/`]` | `literal_comments` | Keep `//` (safe at end of line) |
| Space before `:` after `]` with trailing comment | `option_message_field` | No space (gap logic doesn't see prior trailing emission) |
| Compound-string comment on its own line | `option_complex_array_literal`, `literal_comments`, `message_options` | Attach as trailing → inline |
| Leading comment on array element inline pairing | `literal_comments` | Break onto own line |

No comments are dropped, no syntax is broken, and all passing tests
are idempotent. The remaining diffs are placement-only.

## Recently Fixed

- Empty `;` decl (e.g. `message M {};`) no longer collides with blank-
  line preservation. The trivia walker correctly recognizes `;` after
  a body as a separate empty decl.
- Single-option compact options with a leading `/* */` comment on the
  value (e.g. `[(opt) = /* Before */ nan /* Trailing */]`) now stay
  inline rather than expanding.
- Single-option compact options with a leading comment on the key
  (e.g. `[/* leading */ packed = false]`) now expand to multi-line
  correctly instead of emitting a broken half-expanded form.
- Single-element dicts stay inline when they fit.

## Correctness Improvements Over Legacy

These are cases where our output is intentionally not byte-equivalent
to legacy `buf format` because the legacy output is broken protobuf.

- Inline `//` comment containing `*/` in its body (e.g. `// foo */ bar`)
  is escaped to `/* foo * / bar */` when converting to a block comment.
  Legacy `bufformat/formatter.go` does not check for embedded `*/` and
  produces `/* foo */ bar */`, which terminates the synthesized block
  comment prematurely and leaks `bar */` as text. Our escape preserves
  syntactic validity at the cost of a one-character visual difference
  in the body.
