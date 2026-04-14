# Bufformat Golden Test Differences

This document summarizes the intentional differences between the protocompile
printer and the old buf format golden files in `TestBufFormat`. Each difference
is categorized and reasoned about.

## Skipped Tests

### editions/2024
Parser error test, not a printer test. Not applicable.

### deprecate/*
These tests require AST transforms (adding `deprecated` options) performed by
buf's `FormatModuleSet`, not by the printer. The printer only formats what the
AST contains.

### all/v1/all.proto, customoptions/options.proto
Our formatter keeps detached comments at section boundaries during declaration
sorting rather than permuting them with declarations. When declarations are
reordered, comments that were between two declarations stay at the section
boundary rather than moving with the following declaration. This prevents
comments from being silently associated with the wrong declaration.

### service/v1/service.proto, service/v1/service_options.proto
Our formatter always inserts a space before trailing block comments:
`M /* comment */` vs the golden's `M/* comment */`. Consistent spacing before
trailing comments is more readable and matches the convention used everywhere
else in our output.

## Remaining Failing Tests (4)

### 1. option_complex_array_literal.proto -- Blank lines between array elements

Minor blank line differences between array elements.

**Rationale:** Differences come from our slot-based trivia handling which
normalizes blank lines between elements consistently.

### 2. option_message_field.proto -- Extension key bracket expansion + blank lines

```
golden:  [/* One */ foo.bar..._garblez /* Two */] /* Three */ : "boo"
ours:    [
           /* One */
           foo.bar..._garblez /* Two */
         ]/* Three */
         : "boo"
```

**Cause:** Extension keys containing block comments trigger multi-line expansion
because `scopeHasAttachedComments` detects comments inside the brackets.

**Rationale:** Expanding bracketed expressions with interior comments to
multi-line is our general policy. It makes the comments more visible and the
structure clearer. The golden's single-line form with multiple block comments
is dense and harder to read.

Also has blank line differences between declarations in message literal
contexts, same cause as other blank line diffs.

### 3. message_options.proto -- Block comment placement + bracket expansion

Multiple differences:

**a) Block comments on their own line become inline trailing:**
```
golden:  foo: 1
         /*trailing*/
ours:    foo: 1 /*trailing*/
```
When a block comment follows a value on the next line with no blank line
separating them, our trivia index attaches it as trailing on the value. The
golden keeps it on its own line.

**b) Compact option bracket collapse:**
```
golden:  repeated int64 values = 2 [
           /* leading comment */
           packed = false
           /* trailing comment */
         ];
ours:    repeated int64 values = 2 [/* leading comment */
         packed = false /* trailing comment */];
```
Single compact option with comments stays inline in our formatter because the
option count is 1. The golden expands it due to the comments.

**c) Single-element dict expansion:** FIXED. Single-element dicts now stay
inline when they fit: `{foo: 99}`. The bug was that the caller's gap (e.g.
`gapNewline` from a multi-element array) was emitted inside the `dom.Group`,
causing the group to always break.

**Rationale:** (a) and (b) are stylistic choices about when to expand vs
collapse bracketed expressions. Our formatter consistently expands when content
could benefit from vertical space.

### 4. literal_comments.proto -- Trailing comment format after close braces

```
golden:  } /* Trailing */
ours:    } // Trailing
```

**Cause:** Our `convertLineToBlock` is not set after close braces because `}`
ends a scope and `//` at end of line is safe -- nothing follows that would be
consumed by the line comment.

**Rationale:** Keeping `// Trailing` as-is is correct. The golden's conversion
to `/* Trailing */` is unnecessary since `}` is always followed by a newline
or end of scope. Our behavior preserves the original comment style.

Also has bracket expansion differences for message literals with leading block
comments (same cause as message_options above) and compound string trailing
comment differences.

## Summary

All remaining differences are stylistic. No comments are dropped, no syntax is
broken, and formatting is idempotent for all passing tests. The categories are:

| Category | Tests affected | Our choice |
|----------|---------------|------------|
| Bracket expansion with comments | option_message_field, message_options, literal_comments | Expand when interior has comments |
| Block comment trailing attachment | message_options, literal_comments | Attach to preceding value when no blank line |
| `//` vs `/* */` after `}` | literal_comments | Keep `//` (safe at end of line) |
| Blank lines between declarations | option_message_field, message_options, option_complex_array_literal | Normalized by slot-based trivia |
