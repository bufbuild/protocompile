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

// package ast2 is a high-performance implementation of a Protobuf IDL abstract
// syntax tree. It is intended to be end-all, be-all AST for the following use-cases:
//
// 1. Parse target for a Protobuf compiler.
//
// 2. Incremental, fault-tolerant parsing for a Protobuf language server.
//
// 3. In-place rewriting of the AST to implement refactoring tools.
//
// 4. Formatting, even of partially-invalid code.
//
// "High-performance" means that the AST is optimized to minimize resident memory use,
// pointer nesting (to minimize GC pause contribution), and maximize locality. This AST
// is suitable for hydrating large Buf modules in a long-lived process without exhausting
// memory or introducing unreasonable GC latency.
//
// In general, if an API tradeoff is necessary for satisfying any of the above goals, we
// make the tradeoff, but we attempt to maintain a high degree of ease of use.
//
// This library will replace the existing [github.com/bufbuild/protocompile/ast] library.
// Outside of this file, documentation is written assuming this has already happened.
package ast2
