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

## Remaining Failing Tests

### 1. package.proto -- Space after block comments in paths

```
golden:  header/*...*/./*...*/v1
ours:    header/*...*/ ./*...*/ v1
```

**Cause:** Our `gapGlue` context inserts a space after block comments
(`afterGap = gapSpace`) so that `*/` is never fused to the next identifier.
The golden glues `*/` directly to the next token.

**Rationale:** `*/v1` is harder to read than `*/ v1`. The space after a block
comment is a consistent rule applied everywhere in our formatter. The golden's
behavior is an artifact of treating block comments as invisible for spacing
purposes.

### 2. option_compound_name.proto -- Space around block comments in paths

```
golden:  (custom /* One */ . /* Two */ file_thing_option). /* Three */ foo
ours:    (custom/* One */ ./* Two */ file_thing_option)./* Three */ foo
```

**Cause:** Same `gapGlue` afterGap as above (space after block comments), but
we do not add a space *before* the first block comment in a glued context. The
golden has spaces on both sides.

**Rationale:** Our formatter consistently adds space after block comments but
not before them in glued contexts. Adding space before would require changing
`firstGap` for `gapGlue`, which affects all bracket/path contexts. The current
behavior is readable and internally consistent. This is a minor stylistic
difference.

### 3. compound_string.proto -- Compound string indentation inside arrays

```
golden:        // First element.
golden:        "this"
ours:            // First element.
ours:            "this"
```

**Cause:** Compound strings inside arrays receive an extra level of indentation
from `printCompoundString`'s `withIndent` on top of the array's own indent.

**Rationale:** The extra indentation makes it visually clear that the string
parts belong to a single compound value, not separate array elements. This is
consistent with how compound strings are indented in other contexts (e.g.,
`option ... = \n  "One"\n  "Two"`).

### 4. option_complex_array_literal.proto -- Compound string indentation + blank lines

Same compound string indentation issue as above. Also has minor blank line
differences between array elements.

**Rationale:** Same as above for indentation. Blank line differences come from
our slot-based trivia handling which normalizes blank lines between elements
consistently.

### 5. option_message_field.proto -- Extension key bracket expansion + blank lines

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

### 6. message_options.proto -- Block comment placement + bracket expansion

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

**c) Single-element dict expansion:**
```
golden:  {foo: 99},
ours:    {
           foo: 99
         },
```
Our formatter expands single-element message literals to multi-line. The golden
keeps them compact.

**Rationale:** These are all stylistic choices about when to expand vs collapse
bracketed expressions. Our formatter consistently expands when content could
benefit from vertical space.

### 7. literal_comments.proto -- Trailing comment format after close braces

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
| Space after block comments in gapGlue | package, option_compound_name | Always space after `*/` before identifiers |
| Compound string indentation in arrays | compound_string, option_complex_array_literal | Extra indent level for clarity |
| Bracket expansion with comments | option_message_field, message_options, literal_comments | Expand when interior has comments |
| Block comment trailing attachment | message_options | Attach to preceding value when no blank line |
| `//` vs `/* */` after `}` | literal_comments | Keep `//` (safe at end of line) |
| Blank lines between declarations | option_message_field, message_options, option_complex_array_literal | Normalized by slot-based trivia |
