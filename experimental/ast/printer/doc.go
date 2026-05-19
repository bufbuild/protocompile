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

// Package printer renders an [ast.File] (or an individual [ast.DeclAny]) back
// to protobuf source text. It is designed for two distinct uses:
//
//   - Round-trip fidelity: replay parsed source verbatim, preserving the
//     original whitespace, comments, and structure exactly as written.
//
//   - Formatting: apply layout decisions and optional AST transforms to
//     produce canonicalized output.
//
// # Entry points
//
// [PrintFile] renders an entire file; this should be the usual entry point.
// [Print] renders a single declaration as a snippet with no trailing
// newline — useful for LSP edit previews or for building output
// incrementally one decl at a time.
//
// Both entry points take an [Options] value that configures the printed output:
//
//   - With Options.Format = false (the zero value, round-trip mode), the
//     file is emitted as-is. Source whitespace and comments are preserved
//     verbatim from the token trivia attached during parsing. Options.Formatting
//     is ignored in this case.
//
//   - With Options.Format = true (format mode), [Options.Formatting]
//     drives layout decisions and formats the printed ouptut.
//
// # Formatting presets
//
// Format mode is configured via [Formatting]. Two ready-made presets are
// provided:
//
//   - [Default] is the recommended preset: dynamic layout that respects
//     source intent (e.g. a scope written flat stays flat; width-aware
//     breaking kicks in at 100 columns), and no rewriting of comment text.
//
//   - [Legacy] is the set of configurations that conforms to the legacy
//     protobuf formatter.
//
// See the [Formatting] type for the individual knobs.
//
// # Editing the AST
//
// To mutate the AST before rendering (insert, delete, or move
// declarations), use the companion
// [github.com/bufbuild/protocompile/experimental/ast/edit] package.
// Apply edits first, then call [PrintFile] on the resulting file.
package printer
