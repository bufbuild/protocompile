// Copyright 2020-2024 Buf Technologies, Inc.
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

// package ast is a high-performance implementation of a Protobuf IDL abstract
// syntax tree. It is intended to be end-all, be-all AST for the following
// use-cases:
//
// 1. Parse target for a Protobuf compiler.
//
// 2. Incremental, fault-tolerant parsing for a Protobuf language server.
//
// 3. In-place rewriting of the AST to implement refactoring tools.
//
// 4. Formatting, even of partially-invalid code.
//
// "High-performance" means that the AST is optimized to minimize resident
// memory use, pointer nesting (to minimize GC pause contribution), and maximize
// locality. This AST is suitable for hydrating large Buf modules in a
// long-lived process without exhausting memory or introducing unreasonable GC
// latency.
//
// In general, if an API tradeoff is necessary for satisfying any of the above
// goals, we make the tradeoff, but we attempt to maintain a high degree of ease
// of use.
//
// # AST Context
//
// Virtually all operations in this package involve a [Context] (no, not a
// [context.Context]). This struct acts as an arena that enables the highly
// compressed, memory-friendly representation this package uses for the AST.
//
// Using a [Context], you can create new tokens and new AST nodes. Types that
// represent AST nodes, such as [DeclMessage], are thin wrappers over a pointer
// to a [Context] and index into one of its tables. They are intended to be
// passed by value, because they are essentially pointers (and, in fact,
// expose a IsZero function for checking if they refer to a nil Context pointer).
//
// # Pointer-like types
//
// Virtually all AST nodes in this library are "pointer-like" types, in that
// although they are not Go pointers, they do refer to something stored in a
// [Context] somewhere. Internally, they contain a pointer to a [Context] and
// a pointer to the compressed node representation inside the [Context]. This is
// done so that we can avoid spending an extra eight or twelve bytes per
// node-at-rest, and to minimize GC churn by avoiding pointer cycles in the
// in-memory representation of the AST.
//
// All pointer-like types have a bool-returning IsZero method, which checks for
// the zero value. Pointer-like types should generally be passed by value, not
// by pointer; all of them have value receivers.
//
// In some places, the zero value of a pointer-like type is (incorrectly)
// referred to as nil. This is a documentation bug; instead, it should say zero.
//
// # Coming Soon
//
// This library will replace the existing [github.com/bufbuild/protocompile/ast]
// library. Outside of this file, documentation is written assuming this has
// already happened.
package ast
