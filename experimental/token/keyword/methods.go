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

package keyword

import (
	"iter"

	"github.com/bufbuild/protocompile/internal/ext/iterx"
	"github.com/bufbuild/protocompile/internal/trie"
)

var kwTrie = func() *trie.Trie[Keyword] {
	trie := new(trie.Trie[Keyword])
	for kw := range All() {
		if !kw.IsBrackets() {
			trie.Insert(kw.String(), kw)
		}
	}
	return trie
}()

// Prefix returns the longest prefix of text which matches any of the keywords
// that can be returned by [Lookup].
func Prefix(text string) Keyword {
	_, kw := kwTrie.Get(text)
	return kw
}

// Prefix returns an iterator over the keywords that can be returned by [Lookup]
// which are prefixes of text, in ascending order of length.
func Prefixes(text string) iter.Seq[Keyword] {
	return iterx.Right(kwTrie.Prefixes(text))
}

// Brackets returns the open and close brackets if k is a bracket keyword.
func (k Keyword) Brackets() (left, right, joined Keyword) {
	if int(k) >= len(braces) {
		return Unknown, Unknown, Unknown
	}
	return braces[k][0], braces[k][1], braces[k][2]
}

// IsValid returns whether this is a valid keyword value (not including
// [Unknown]).
func (k Keyword) IsValid() bool {
	return k.properties()&valid != 0
}

// IsProtobuf returns whether this keyword is used in Protobuf.
func (k Keyword) IsProtobuf() bool {
	return k.properties()&protobuf != 0
}

// IsCEL returns whether this keyword is used in CEL.
func (k Keyword) IsCEL() bool {
	return k.properties()&cel != 0
}

// IsPunctuation returns whether this keyword is punctuation (i.e., not a word).
func (k Keyword) IsPunctuation() bool {
	return k.properties()&punct != 0
}

// IsReservedWord returns whether this keyword is a known reserved word (i.e.,
// not punctuation).
func (k Keyword) IsReservedWord() bool {
	return k.properties()&word != 0
}

// IsBrackets returns whether this is "paired brackets" keyword.
func (k Keyword) IsBrackets() bool {
	return k.properties()&brackets != 0
}

// IsModifier returns whether this keyword is any kind of modifier.
func (k Keyword) IsModifier() bool {
	return k.IsMethodTypeModifier() ||
		k.IsTypeModifier() ||
		k.IsImportModifier() ||
		k.IsMethodTypeModifier()
}

// IsFieldTypeModifier returns whether this is a modifier for a field type
// in Protobuf.
func (k Keyword) IsFieldTypeModifier() bool {
	return k.properties()&modField != 0
}

// IsTypeModifier returns whether this is a modifier for a type declaration
// in Protobuf.
func (k Keyword) IsTypeModifier() bool {
	return k.properties()&modType != 0
}

// IsImportModifier returns whether this is a modifier for an import declaration
// in Protobuf.
func (k Keyword) IsImportModifier() bool {
	return k.properties()&modImport != 0
}

// IsMethodTypeModifier returns whether this is a modifier for a method
// declaration in Protobuf.
func (k Keyword) IsMethodTypeModifier() bool {
	return k.properties()&modMethodType != 0
}

// IsPseudoOption returns whether this is a Protobuf pseudo-option name
// ([default = "..."] and [json_name = "..."]).
func (k Keyword) IsPseudoOption() bool {
	return k.properties()&pseudoOption != 0
}
