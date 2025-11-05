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

// Package source provides a standard interfaces for working with source files
// and spans within them.
//
// [File] is a source code file read from somewhere, which tracks its contents,
// its on-disk path, and book-keeping information for looking up offsets.
// [Span] is a region of some [File] which is used for diagnostics. [Opener]
// is a common interface for loading [File]s on disk.
package source
