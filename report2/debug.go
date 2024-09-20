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

package report2

import (
	"os"
	"strings"
)

const (
	debugOff int = iota
	debugMinimal
	debugFull
)

// debugEnabled is the status of the PROTOCOMPILE_DEBUG environment variable at
// startup. This cannot be set in any way except by environment variable. It is
// used to enable diagnostic-debugging functionality.
var debugMode = func() int {
	switch strings.ToLower(os.Getenv("PROTOCOMPILE_DEBUG")) {
	case "", "0", "off", "false":
		return debugOff
	case "full":
		return debugFull
	default:
		return debugMinimal
	}
}()
