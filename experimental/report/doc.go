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

/*
Package report provides a robust diagnostics framework. It offers diagnostic
construction, interchange, and ASCII art rendering functionality.

Diagnostics are collected into a [Report], which is a helpful builder over
a slice of [Diagnostic]s. Each [Diagnostic] consists of a Go error plus
metadata for rendering, such as source code spans, notes, and suggestions.
This package takes after Rust's diagnostic philosophy: diagnostics should
be pleasant to read, provide rich information about the error, and come in
a standard, machine-readable format.

Reports can be rendered using a [Renderer], which provides several options
for how to render the result to the user.

A Report can be converted into a Protobuf using [Report.ToProto]. This can
be serialized to e.g. JSON as an alternative error output.

The [File] type is a generic utility for converting file offsets into
text editor coordinates. E.g., given a byte offset, what is the user-visible
line and column number? Package report expects the caller to construct this
information themselves, to avoid recomputing it unnecessarily.

# Defining Diagnostics

Generally, to define a diagnostic, you should define a new Go error type,
and then make it implement [Diagnose]. This has two benefits:

 1. When someone using your tool as a library looks through a Report, they
    can type assert Diagnostic.Err to programmatically determine the nature
    of a diagnostic.

 2. When emitting the diagnostic in different places you get the same UX.
    This means you should do this even if the error type will be unexported.

Sometimes, (2) is not enough of a benefit, in which case you can just use
Report.Errorf() and friends.

# Diagnostics Style Guide

Diagnostics created with package report expect to be written in a certain
way. The following guidelines are taken, mostly verbatim, from the [Rust
Project's diagnostics style guide].

The golden rule: Users will see diagnostics when they are frustrated. Do not
make them more frustrated. Do not make them feel like your tool does not
respect their intelligence.

 1. Errors are for semantic constraint violations, i.e., the compiler will
    not produce valid output. Warnings are for when the compiler notices
    something not strictly forbidden but probably bad. Remarks are
    essentially warnings that are not shown to the user by default.
    Diagnostic notes are for factual information that adds context to why the
    diagnostic was shown. Diagnostic help is for prose suggestions to the
    user. Diagnostic debugs are never shown to normal users, and are for
    compiler debugging only.

 2. Diagnostics should be written in plain, friendly English. Your message
    will appear on many surfaces, such as terminals and LSP plugin insets.
    The golden standard is that the error message should be readable and
    understandable by an inexperienced, hung-over programmer whose native
    language is not European, displayed on a dirty budget smartphone screen.

 3. Diagnostic messages do not begin with a capital letter and do not end in
    punctuation. The compiler does not ask questions. The words "error",
    "warning", "remark", "help", and "note" are NEVER capitalized. Never
    refer to "a diagnostic"; prefer something more specific, like "compiler
    error".

 4. Error messages should be succinct: short and sweet, keeping in mind (1).
    Users will see these messages many, many times.

 5. The word "illegal" is illegal. We use this term inside the compiler, but
    the word may have negative connotations for some people. "Forbidden" is
    also forbidden. Prefer "invalid", "not allowed", etc.

 6. The first span in a diagnostic (the primary span) should be precisely
    the code that resulted in the error. Try to avoid more than three spans
    in an error. Try to pick the smallest spans you can: instead of
    highlighting a whole type definition, try highlighting just its name.

 7. Try not to emit multiple diagnostics for the same error. This requires
    more work in the compiler, but it is worth it for the UX.

 8. If your tool does not have enough information to emit a good diagnostic,
    that is a bug in either your tool, or in the language your tool operates
    on (in both cases, it is the tool's job to acquire this information).

 9. When talking about your tool, call it "the compiler", "the linter", etc.
    Your tool is a machine, not a person; therefore it does not speak in
    first person. When referring to a programming language's semantics,
    rather than the compiler's, use that language's name. For example,
    "Go does not support...",  "... is not valid Protobuf", "this is a
    limitation of C++".

[Rust Project's diagnostics style guide]: https://github.com/rust-lang/rustc-dev-guide/blob/master/src/diagnostics.md
*/
package report
