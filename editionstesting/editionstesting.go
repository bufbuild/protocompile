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

// Package editionstesting is a temporary package that allows users to test
// functionality related to Protobuf editions while that support is not yet
// complete. Once that support is complete, this package will be removed.
package editionstesting

import "github.com/bufbuild/protocompile/internal"

// AllowEditions can be called to opt into this repo's support for Protobuf
// editions. This is primarily intended for testing. This repo's support
// for editions is not yet complete.
//
// Once the implementation of editions is complete, this function will be
// REMOVED and editions will be allowed by all usages of this repo.
//
// The internal flag that this touches is not synchronized and calling this
// function is not thread-safe. So this must be called prior to using the
// compiler, ideally from an init() function or as one of the first things
// done from a main() function.
func AllowEditions() {
	internal.AllowEditions = true
}
