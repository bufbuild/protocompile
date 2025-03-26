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

// Package queries provides specific [incremental.Query] types for various parts
// of Protocompile. It is separate from package incremental itself because it is
// Protocompile-specific.
package queries

// Values for [report.Report].SortOrder, which determine how diagnostics
// generated across parts of the compiler are sorted.
const (
	stageFile int = iota * 10
	stageAST
	stageIR
)
