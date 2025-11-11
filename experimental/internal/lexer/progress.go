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

package lexer

// mustProgress is a helper for ensuring that the lexer makes progress
// in each loop iteration. This is intended for turning infinite loops into
// panics.
type mustProgress struct {
	l    *lexer
	prev int
}

// check panics if the lexer has not advanced since the last call.
func (mp *mustProgress) check() {
	if mp.prev == mp.l.cursor {
		// NOTE: no need to annotate this panic; it will get wrapped in the
		// call to HandleICE for us.
		panic("lexer failed to make progress; this is a bug in protocompile")
	}
	mp.prev = mp.l.cursor
}
