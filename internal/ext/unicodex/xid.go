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

package unicodex

import "unicode"

// IsASCIIIdent checks if s is a "traditional" ASCII-only identifier.
func IsASCIIIdent(s string) bool {
	for i, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
			if i == 0 {
				return false
			}
		case r == '_':
		default:
			return false
		}
	}
	return len(s) > 0
}

// IsXIDStart returns whether r has the XID_Start property.
func IsXIDStart(r rune) bool {
	// ASCII fast path.
	if r == '_' ||
		(r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') {
		return true
	}

	return unicode.In(r,
		unicode.Letter,
		unicode.Nl, // Number, letter.
		unicode.Other_ID_Start,
	) && !unicode.In(r,
		unicode.Pattern_Syntax,
		unicode.Pattern_White_Space,
	)
}

// IsXIDContinue returns whether r has the XID_Continue property.
func IsXIDContinue(r rune) bool {
	// ASCII fast path.
	if r == '_' ||
		(r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') {
		return true
	}

	return unicode.In(r,
		unicode.Letter,
		unicode.Cf, // Other, format. This includes some joiners.
		unicode.Mn, // Mark, nonspacing.
		unicode.Mc, // Mark, combining. Handled above.
		unicode.Nl, // Number, letter.
		unicode.Nd, // Number, digit.
		unicode.Pc, // Punctuation, connector.
		unicode.Other_ID_Start,
	) && !unicode.In(r,
		unicode.Pattern_Syntax,
		unicode.Pattern_White_Space,
	)
}
