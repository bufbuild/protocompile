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

// package predeclared provides all of the identifiers with a special meaning
// in Protobuf.
//
// These are not keywords, but are rather special names injected into scope in
// places where any user-defined path is allowed. For example, the identifier
// string overrides the meaning of a path with a single identifier called string,
// (such as a reference to a message named string in the current package) and as
// such counts as a predeclared identifier.
package predeclared

//go:generate go run github.com/bufbuild/protocompile/internal/enum predeclared.yaml
