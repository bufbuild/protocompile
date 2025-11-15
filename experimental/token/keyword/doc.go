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

// Package keyword provides [Keyword], an enum of all special "grammar particles"
// (i.e., literal tokens with special meaning in the grammar such as identifier
// keywords and punctuation) recognized by Protocompile.
package keyword

//go:generate go run github.com/bufbuild/protocompile/internal/enum keyword.yaml
