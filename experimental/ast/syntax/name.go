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

package syntax

import (
	"fmt"
	"strconv"
)

var names = func() map[Syntax]string {
	names := make(map[Syntax]string)
	for syntax := range All() {
		if syntax.IsEdition() {
			names[syntax] = fmt.Sprintf("Edition %s", syntax)
		} else {
			names[syntax] = strconv.Quote(syntax.String())
		}
	}
	return names
}()

// Name returns the name of this syntax as it should appear in diagnostics.
func (s Syntax) Name() string {
	name, ok := names[s]
	if !ok {
		return "Edition <?>"
	}
	return name
}
