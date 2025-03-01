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

// Package dom provides a DOM-like tree structure for formatting text. A [Dom] is a tree of
// [Chunk]s, and each [Chunk] represents a line of text with formatting information for
// indentation and splitting (what whitespace should follow the text, if any).
//
// A [Dom] can be formatted with a line limit and indent string. The line limit will be used
// to determining splitting behaviour for each [Chunk] and the indent string will be used to
// measure and output the formatted string.
//
// If [Dom.Format] is not called, then the output of the Dom will be unformatted. Callers
// can check the state of the formatting with [Dom.Formatting].
package dom

//go:generate go run github.com/bufbuild/protocompile/internal/enum split.yaml
