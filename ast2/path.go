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

package ast2

// Path represents a multi-part identifier.
//
// This includes single identifiers like foo, references like foo.bar,
// and fully-qualified names like .foo.bar.
type Path struct {
	withContext

	raw rawPath
}

// Absolute returns whether this path starts with a dot.
func (p Path) Absolute() bool {
	start, _ := p.first()
	return start.Text() == "."
}

// AsIdent returns the single identifier that comprises this path, or
// the nil token.
func (p Path) AsIdent() Token {
	start, only := p.first()
	if !only || start.Kind() != TokenIdent {
		return Token{}
	}
	return start
}

// AsBuiltin returns the builtin that this path represents.
//
// If this path does not represent a builtin, returns [BuiltinUnknown].
func (p Path) AsBuiltin() Builtin {
	return BuiltinByName(p.AsIdent().Text())
}

// Span implements [Spanner] for Path.
func (p Path) Span() Span {
	return JoinSpans(p.raw[0].With(p), p.raw[1].With(p))
}

// Components is an [iter.Seq2] that ranges over each component in this path. Specifically,
// it yields the (nilable) dot that precedes the component, and the identifier token.
func (p Path) Components(yield func(dot, ident Token) bool) {
	if p.Nil() {
		return
	}

	var dot Token
	nextDot := p.Absolute()

	first := p.raw[0].With(p)
	if synth := first.synthetic(); synth != nil {
		if len(synth.children) == 0 {
			yield(Token{}, first)
			return
		}

		for _, tok := range synth.children {
			tok := tok.With(p)
			if nextDot {
				dot = tok
			} else if !yield(dot, tok) {
				break
			}
		}

		return
	}

	for i := p.raw[0]; i <= p.raw[1]; i++ {
		tok := i.With(p)
		if tok.Kind().IsSkippable() {
			continue
		}

		if nextDot {
			dot = tok
		} else if !yield(dot, tok) {
			break
		}
		nextDot = !nextDot
	}
}

// PathComponent is a piece of a path. This is either an identifier or a nested path
// (for an extension name).
type PathComponent struct {
	part Token
}

// IsExtension returns whether this path component is an extension component, i.e.
// (a.b.c).
func (p PathComponent) IsExtension() bool {
	return p.part.Kind() == TokenPunct
}

// AsExtension returns the Path inside of this path component, if it is an extension
// path component.
func (p PathComponent) AsExtension() Path {
	if !p.IsExtension() {
		return Path{}
	}

	// If this is a synthetic token, its children are already precisely a path,
	// so we can use the "synthetic with children" form of Path.
	if synth := p.part.synthetic(); synth != nil {
		return Path{withContext{p.part.Context()}, rawPath{p.part.id, 0}}
	}

	// Find the first and last non-skippable tokens to be the bounds.
	var first, last Token
	for token := range p.part.Children {
		if token.Kind().IsSkippable() {
			continue
		}

		if first.Nil() {
			first = token
		} else {
			// Only set last after seeing first, because then if we only
			// ever see one non-skippable token, it will leave last nil.
			last = token
		}
	}

	return Path{withContext{p.part.Context()}, rawPath{first.id, last.id}}
}

// AsIdent returns the single identifier that makes up this path component, if
// it is not an extension path component.
func (p PathComponent) AsIdent() Token {
	if p.IsExtension() {
		return Token{}
	}
	return p.part
}

// ** PRIVATE ** //

// start returns the starting token for this path, and whether it is the sole token.
func (p Path) first() (Token, bool) {
	start := p.raw[0].With(p)
	if synth := start.synthetic(); synth != nil && len(synth.children) > 0 {
		return synth.children[0].With(p), false
	}

	return start, p.raw[1].With(p).Nil()
}

// rawPath is the raw contents of a Path without its Context.
//
// This has one of the following configurations.
//
// 1. A single non-synthetic token and a nil token. In this case, this is a single component.
//
//  2. A pair of tokens. In this case, this is a path with at least one dot. The
//     path is absolute if the first token is a dot.
//
//  3. A single synthetic token and a nil token. If this token has children, those are
//     the path components. Otherwise, the token itself is the sole token.
//
// NOTE: Multiple compressed representations in this package depend on the fact that
// if raw[0] < 0, then raw[1] == 0 for all valid paths.
type rawPath [2]rawToken

// Wrap wraps this rawPath with a context to present to the user.
func (p rawPath) With(c Contextual) Path {
	if p[0] == 0 {
		return Path{}
	}

	return Path{withContext{c.Context()}, p}
}
