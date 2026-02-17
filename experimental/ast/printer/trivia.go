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

package printer

import (
	"strings"

	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/experimental/token/keyword"
)

// attachedTrivia holds trivia tightly bound to a single semantic token.
type attachedTrivia struct {
	// leading contains all skippable tokens before this token
	// (since the previous non-skippable token in the same scope),
	// with any trailing comment already extracted into trailing.
	leading []token.Token

	// trailing contains the trailing comment on the same line
	// after this token (if any).
	trailing []token.Token
}

// detachedTrivia holds a set of trivia runs as slots within a scope.
type detachedTrivia struct {
	slots [][]token.Token
}

func (t detachedTrivia) isEmpty() bool {
	return len(t.slots) == 0 || len(t.slots) == 1 && len(t.slots[0]) == 0
}

// triviaIndex is the complete trivia decomposition for one file.
//
// Every natural non-skippable token gets an entry in attached, even if
// both leading and trailing are empty. This distinguishes natural tokens
// (use leading trivia) from synthetic tokens (use gap fallback).
type triviaIndex struct {
	attached map[token.ID]attachedTrivia
	detached map[token.ID]detachedTrivia
}

// scopeTrivia returns the detached trivia for a tokens scope.
func (idx *triviaIndex) scopeTrivia(scopeID token.ID) detachedTrivia {
	if idx == nil {
		return detachedTrivia{}
	}
	return idx.detached[scopeID]
}

// tokenTrivia returns the attached trivia for a token.
// Returns true for all natural tokens, false for synthetic tokens.
func (idx *triviaIndex) tokenTrivia(id token.ID) (attachedTrivia, bool) {
	if idx == nil {
		return attachedTrivia{}, false
	}
	att, ok := idx.attached[id]
	return att, ok
}

// buildTriviaIndex walks the entire token stream and builds the trivia index.
//
// For each non-skippable token, it collects all preceding skippable tokens
// as the token's "leading" trivia. Trailing comments (line comments on the
// same line as the previous token) are extracted and stored separately.
//
// Declaration boundaries (`;` and `}`) are tracked to build scope slots.
// When trivia between two declarations contains a blank line, the portion
// before the last blank line is stored as detached trivia in a slot,
// enabling it to survive if the following declaration is deleted.
func buildTriviaIndex(stream *token.Stream) *triviaIndex {
	idx := &triviaIndex{
		attached: make(map[token.ID]attachedTrivia),
		detached: make(map[token.ID]detachedTrivia),
	}
	idx.walkScope(stream.Cursor(), 0)
	return idx
}

// walkScope processes all tokens within one scope.
//
// scopeID is 0 for the file-level scope, or the fused bracket's
// token.ID for bracket-interior scopes.
//
// It builds both per-token attached trivia and per-scope slot arrays.
// Slot boundaries are detected by tracking `;` and `}` tokens, which
// mark the end of declarations in protobuf syntax.
func (idx *triviaIndex) walkScope(cursor *token.Cursor, scopeID token.ID) {
	var pending []token.Token
	var trivia detachedTrivia
	for tok := cursor.NextSkippable(); !tok.IsZero(); tok = cursor.NextSkippable() {
		if tok.Kind().IsSkippable() {
			pending = append(pending, tok)
			continue
		}
		detached, attached := splitDetached(pending)
		trivia.slots = append(trivia.slots, detached)
		idx.attached[tok.ID()] = attachedTrivia{leading: attached}
		pending = nil

		idx.walkDecl(cursor, tok)
	}
	// Append any remaining tokens at the end of scope.
	trivia.slots = append(trivia.slots, pending)
	idx.detached[scopeID] = trivia
}

func (idx *triviaIndex) walkFused(leafToken token.Token) token.Token {
	openToken, closeToken := leafToken.StartEnd()
	idx.walkScope(leafToken.Children(), openToken.ID())

	trivia := idx.scopeTrivia(openToken.ID())
	endTokens := trivia.slots[len(trivia.slots)-1]
	detached, attached := splitDetached(endTokens)
	trivia.slots[len(trivia.slots)-1] = detached
	idx.detached[openToken.ID()] = trivia
	idx.attached[closeToken.ID()] = attachedTrivia{
		leading: attached,
	}
	return closeToken
}

// walkDecl processes a declaration.
func (idx *triviaIndex) walkDecl(cursor *token.Cursor, startToken token.Token) {
	endToken := startToken
	var pending []token.Token
	for tok := startToken; !tok.IsZero(); tok = cursor.NextSkippable() {
		if tok != startToken && tok.Kind().IsSkippable() {
			pending = append(pending, tok)
			continue
		}

		// Register leading trivia for every non-skippable token after the
		// first (the first token's trivia is already set by walkScope).
		if tok != startToken {
			idx.attached[tok.ID()] = attachedTrivia{leading: pending}
			pending = nil
		}

		endToken = tok
		if !tok.IsLeaf() {
			// Recurse into fused tokens (non-leaf tokens).
			endToken = idx.walkFused(tok)
		}
		atDeclBoundary := tok.Keyword() == keyword.Semi || tok.Keyword().IsBrackets()
		if atDeclBoundary {
			break
		}
	}
	// Capture trailing trivia for end of declaration. This includes comments on
	// the last line and all blank lines beneath it, up until the last newline.
	afterNewline := false
	atEndOfScope := true
	var trailing []token.Token
	for tok := cursor.NextSkippable(); !tok.IsZero(); tok = cursor.NextSkippable() {
		isNewline := tok.Kind() == token.Space && strings.Count(tok.Text(), "\n") > 0
		isSpace := tok.Kind() == token.Space && !isNewline
		isComment := tok.Kind() == token.Comment
		if !afterNewline && !isNewline && !isSpace && !isComment {
			cursor.PrevSkippable()
			atEndOfScope = false
			break
		}
		if afterNewline && !isNewline && !isSpace {
			detached, attached := splitDetached(trailing)
			trailing = detached

			cursor.PrevSkippable()
			for range attached {
				cursor.PrevSkippable()
			}
			atEndOfScope = false
			break
		}
		afterNewline = afterNewline || isNewline
		trailing = append(trailing, tok)
	}
	// At end of scope, keep only inline content (before the first newline) as
	// trailing. Put back newlines and beyond so they flow to the scope's last
	// slot and become the close token's leading trivia via walkFused.
	if atEndOfScope && afterNewline {
		firstNewline := len(trailing)
		for i, tok := range trailing {
			if tok.Kind() == token.Space && strings.Count(tok.Text(), "\n") > 0 {
				firstNewline = i
				break
			}
		}
		for range len(trailing) - firstNewline {
			cursor.PrevSkippable()
		}
		trailing = trailing[:firstNewline]
	}
	if len(trailing) > 0 {
		att := idx.attached[endToken.ID()]
		att.trailing = trailing
		idx.attached[endToken.ID()] = att
	}
}

// splitDetached splits a trivia token slice at the last blank line boundary.
// A blank line boundary consists 2+ newlines within a set of only Space tokens.
func splitDetached(tokens []token.Token) (detached, attached []token.Token) {
	lastBlankEnd := -1
	for index := len(tokens) - 1; index >= 0; index-- {
		tok := tokens[index]
		if tok.Kind() != token.Space {
			lastBlankEnd = -1
		} else if n := strings.Count(tok.Text(), "\n"); n > 0 {
			if lastBlankEnd != -1 {
				break
			}
			lastBlankEnd = index
		}
	}
	if lastBlankEnd == -1 {
		return nil, tokens
	}
	return tokens[:lastBlankEnd], tokens[lastBlankEnd:]
}
