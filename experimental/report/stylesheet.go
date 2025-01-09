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

package report

// styleSheet is the colors used for pretty-rendering diagnostics.
type styleSheet struct {
	r Renderer

	reset string
	// Normal colors.
	nError, nWarning, nRemark, nAccent, nAdd, nDelete string
	// Bold colors.
	bError, bWarning, bRemark, bAccent, bAdd, bDelete string
}

func newStyleSheet(r Renderer) styleSheet {
	if !r.Colorize {
		return styleSheet{r: r}
	}

	return styleSheet{
		r:     r,
		reset: "\033[0m",
		// Red.
		nError: "\033[0;31m",
		bError: "\033[1;31m",

		// Yellow.
		nWarning: "\033[0;33m",
		bWarning: "\033[1;33m",

		// Cyan.
		nRemark: "\033[0;36m",
		bRemark: "\033[1;36m",

		// Blue. Used for "accents" such as non-primary span underlines, line
		// numbers, and other rendering details to clearly separate them from
		// the source code (which appears in white).
		nAccent: "\033[0;34m",
		bAccent: "\033[1;34m",

		// Green.
		nAdd: "\033[0;32m",
		bAdd: "\033[1;32m",
		// Red.
		nDelete: "\033[0;31m",
		bDelete: "\033[1;31m",
	}
}

// ColorForLevel returns the escape sequence for the non-bold color to use for
// the given level.
func (c styleSheet) ColorForLevel(l Level) string {
	switch l {
	case Error, ICE:
		return c.nError
	case Warning:
		if c.r.WarningsAreErrors {
			return c.nError
		}
		return c.nWarning
	case Remark:
		return c.nRemark
	case noteLevel:
		return c.nAccent
	default:
		return ""
	}
}

// BoldForLevel returns the escape sequence for the bold color to use for
// the given level.
func (c styleSheet) BoldForLevel(l Level) string {
	switch l {
	case Error, ICE:
		return c.bError
	case Warning:
		if c.r.WarningsAreErrors {
			return c.nError
		}
		return c.bWarning
	case Remark:
		return c.bRemark
	case noteLevel:
		return c.bAccent
	default:
		return ""
	}
}
