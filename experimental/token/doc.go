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

// Package token provides a memory-efficient representation of a token tree
// stream for the Protobuf IDL.
//
// # Token Trees
//
// Tokens in Protocompile are trees: a token may "contain" a range of other
// tokens. For example, the tokens between matched braces { } are contained
// within the braces, accessible via [Token.Children]. This simplifies parsing,
// since it moves the tricky work of matching braces out of the parser and into
// the lexer. It is also use for other sequences of tokens that are better
// manipulated as a single token, such as juxtaposed strings that are
// automatically concatenated.
//
// # Synthetic Tokens
//
// To support post-parse modification, this library distinguishes between
// natural [Token]s (those created by a parse operation) and synthetic [Token]s
// (those created programmatically to modify/build an AST).
//
// Synthetic tokens have a few important differences from ordinary tokens, the
// most important of which is that they do not appear in the token stream (so
// [Stream.Cursor] won't find them) and they do not have [source.Span]s, so they
// cannot be used in diagnostics (Span() will return the zero [source.Span]).
package token

//go:generate go run github.com/bufbuild/protocompile/internal/enum kind.yaml
