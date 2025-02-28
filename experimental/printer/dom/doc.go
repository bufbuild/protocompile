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
// TODO
package dom

//go:generate go run github.com/bufbuild/protocompile/internal/enum split.yaml
