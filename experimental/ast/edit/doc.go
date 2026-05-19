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

// Package edit applies declaration-level mutations to a Protobuf AST
// before it is rendered.
//
// The package exposes a single entry point, [ApplyEdits], which
// applies a list of [Edit] values to an [ast.File] in order. Edits
// cover adding, deleting, and moving declarations within decl-
// bearing bodies (file, message, enum, service, oneof, extend, and
// RPC method bodies).
//
// After applying edits, the file is typically rendered with the
// [github.com/bufbuild/protocompile/experimental/ast/printer]
// package:
//
//	if err := edit.ApplyEdits(file, edits); err != nil {
//	    return err
//	}
//	out, err := printer.PrintFile(printer.Options{
//	    Format:     true,
//	    Formatting: printer.Default(),
//	}, file)
package edit
