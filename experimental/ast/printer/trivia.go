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

// triviaRun is a contiguous sequence of skippable tokens (whitespace and comments).
type triviaRun struct {
	tokens []token.Token
}

// attachedTrivia holds trivia tightly bound to a single semantic token.
type attachedTrivia struct {
	// leading contains all skippable tokens before this token
	// (since the previous non-skippable token in the same scope),
	// with any trailing comment already extracted into trailing.
	leading triviaRun

	// trailing contains the trailing comment on the same line
	// after this token (if any).
	trailing triviaRun
}

// slot is one positional bucket of detached trivia within a scope.
type slot struct {
	runs []triviaRun
}

// triviaIndex is the complete trivia decomposition for one file.
//
// Every natural non-skippable token gets an entry in attached, even if
// both leading and trailing are empty. This distinguishes natural tokens
// (use leading trivia) from synthetic tokens (use gap fallback).
//
// The scopeEnd map holds skippable tokens at the end of each scope
// (after the last non-skippable token). For bracket scopes, the
// scopeEnd is consumed during building and stored as the close
// bracket's leading trivia. For the file scope (ID 0), it is
// emitted by printFile after all declarations.
type triviaIndex struct {
	attached map[token.ID]attachedTrivia
	detached map[token.ID][]slot
	scopeEnd map[token.ID]triviaRun
}

// scopeSlots returns the slot array for a scope, or nil if none.
func (idx *triviaIndex) scopeSlots(scopeID token.ID) []slot {
	if idx == nil {
		return nil
	}
	return idx.detached[scopeID]
}

// tokenTrivia returns the attached trivia for a token.
// The bool indicates whether the token was seen during building
// (true for all natural tokens, false for synthetic tokens).
func (idx *triviaIndex) tokenTrivia(id token.ID) (attachedTrivia, bool) {
	if idx == nil {
		return attachedTrivia{}, false
	}
	att, ok := idx.attached[id]
	return att, ok
}

// getScopeEnd returns the end-of-scope trivia for a scope.
func (idx *triviaIndex) getScopeEnd(scopeID token.ID) triviaRun {
	if idx == nil {
		return triviaRun{}
	}
	return idx.scopeEnd[scopeID]
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
		detached: make(map[token.ID][]slot),
		scopeEnd: make(map[token.ID]triviaRun),
	}

	walkScope(stream.Cursor(), 0, idx)
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
func walkScope(cursor *token.Cursor, scopeID token.ID, idx *triviaIndex) {
	var pending []token.Token
	var prev token.ID
	atDeclBoundary := true // true at start of scope for slot[0]
	slotIdx := 0
	var slots []slot

	t := cursor.NextSkippable()
	for !t.IsZero() {
		if t.Kind().IsSkippable() {
			pending = append(pending, t)
			t = cursor.NextSkippable()
			continue
		}

		// Extract trailing comment for the previous token.
		trailing, rest := extractTrailing(prev, pending)
		if len(trailing) > 0 {
			setTrailing(prev, trailing, idx)
		}

		// At a declaration boundary, split trivia into detached (slot)
		// and attached (token leading). This preserves detached comments
		// when declarations are deleted from the AST.
		if atDeclBoundary && len(rest) > 0 {
			detached, attached := splitDetached(rest)
			if len(detached) > 0 {
				for slotIdx >= len(slots) {
					slots = append(slots, slot{})
				}
				slots[slotIdx] = slot{runs: []triviaRun{{tokens: detached}}}
			}
			rest = attached
		}

		idx.attached[t.ID()] = attachedTrivia{
			leading: triviaRun{tokens: rest},
		}
		pending = nil
		prev = t.ID()

		// Track declaration boundaries: `;` ends simple declarations,
		// `}` (close brace) ends body declarations.
		atDeclBoundary = (t.Keyword() == keyword.Semi)

		// Recurse into fused brackets (non-leaf tokens).
		if !t.IsLeaf() {
			walkScope(t.Children(), t.ID(), idx)

			// The close bracket's leading trivia comes from the
			// inner scope's end-of-scope data.
			endRun := idx.scopeEnd[t.ID()]
			delete(idx.scopeEnd, t.ID())

			_, closeTok := t.StartEnd()
			processToken(closeTok, prev, endRun.tokens, idx)
			prev = closeTok.ID()

			// A close brace ends a declaration (message, enum, etc.).
			if closeTok.Keyword() == keyword.RBrace {
				atDeclBoundary = true
			}
		}

		if atDeclBoundary {
			slotIdx++
		}

		t = cursor.NextSkippable()
	}

	// Handle end of scope: extract trailing comment for the last
	// non-skippable token, store remaining as scope end.
	eofTrailing, rest := extractTrailing(prev, pending)
	if len(eofTrailing) > 0 {
		setTrailing(prev, eofTrailing, idx)
	}
	idx.scopeEnd[scopeID] = triviaRun{tokens: rest}

	if len(slots) > 0 {
		idx.detached[scopeID] = slots
	}
}

// processToken stores the leading trivia for tok and extracts any
// trailing comment for the previous token.
func processToken(tok token.Token, prevID token.ID, pending []token.Token, idx *triviaIndex) {
	trailing, rest := extractTrailing(prevID, pending)
	if len(trailing) > 0 {
		setTrailing(prevID, trailing, idx)
	}
	idx.attached[tok.ID()] = attachedTrivia{
		leading: triviaRun{tokens: rest},
	}
}

// setTrailing stores a trailing comment on the given token.
func setTrailing(prevID token.ID, trailing []token.Token, idx *triviaIndex) {
	if prevID == 0 {
		return
	}
	att := idx.attached[prevID]
	att.trailing = triviaRun{tokens: trailing}
	idx.attached[prevID] = att
}

// extractTrailing checks if the beginning of pending tokens forms a
// trailing comment on the previous non-skippable token. A trailing
// comment is a line comment (//) on the same line as the previous token,
// optionally preceded by non-newline whitespace.
//
// Returns (trailing tokens, remaining tokens).
func extractTrailing(prevID token.ID, pending []token.Token) (trailing, rest []token.Token) {
	if prevID == 0 || len(pending) == 0 {
		return nil, pending
	}

	idx := 0

	// Skip leading whitespace that does not contain a newline.
	if idx < len(pending) && pending[idx].Kind() == token.Space {
		if strings.Contains(pending[idx].Text(), "\n") {
			// Newline found before any comment: no trailing comment.
			return nil, pending
		}
		idx++
	}

	// Check for a line comment.
	if idx < len(pending) && pending[idx].Kind() == token.Comment &&
		strings.HasPrefix(pending[idx].Text(), "//") {
		end := idx + 1
		return pending[:end], pending[end:]
	}

	return nil, pending
}

// splitDetached splits a trivia token slice at the last blank line boundary.
// A blank line is 2+ consecutive newline-only Space tokens.
// Everything before the last blank line is detached; the blank line and
// everything after is attached (stays on the token).
func splitDetached(tokens []token.Token) (detached, attached []token.Token) {
	lastBlankStart := -1
	i := 0
	for i < len(tokens) {
		start := i
		for i < len(tokens) && tokens[i].Kind() == token.Space && tokens[i].Text() == "\n" {
			i++
		}
		if i-start >= 2 {
			lastBlankStart = start
		}
		if i == start {
			i++
		}
	}

	if lastBlankStart <= 0 {
		return nil, tokens
	}

	return tokens[:lastBlankStart], tokens[lastBlankStart:]
}
