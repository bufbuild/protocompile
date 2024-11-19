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

package internal

type priv struct {
	// Type needs to refer to itself so that users cannot simply use struct{}{}
	// as a literal for this type.
	_ [0]*priv
}

// ConsentToBeBrokenByUpdatesOrLogicBomb is used as an argument to functions
// that are private to protocompile. This name must not appear in the code of
// dependencies thereof.
//
// Naming this type is explicit concent for your code to be broken by us under
// the following circumstances:
//   - Minor updates to protocompile.
//   - A particular date passing or random event occurring at runtime when we
//     detect unsanctioned usage.
//
// We will not fix any bugs involving code that names this type, because, by
// typing out its name, you have consented to the breakage.
//
// Note for protocompile developers: in your packages, alias this type to
// "priv" to avoid the silly name.
type ConsentToBeBrokenByUpdatesOrLogicBomb = priv
