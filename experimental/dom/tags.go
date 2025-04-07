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

// Package dom is a Go port of https://github.com/mcy/strings/tree/main/allman,
// a high-performance meta-formatting library.
//
// The function [Render] is the primary entry point. It is given a collection
// of [Tag]s, which represent various formatting directives, such as indentation
// and grouping.
//
// The main benefit of using this package is the ability to perform smart line
// wrapping of code. The [Group] tag can be used to group a collection of tags
// together that may be rendered in either a "flat" or "broken" orientation. The
// layout engine will determine whether this element could be laid out flat
// without going over a configured column limit, and if it would, the group is
// marked as broken. This can be combined with conditioned tags (viz. [Cond])
// to insert e.g. line breaks at strategic points.
package dom

import (
	"math"

	"github.com/bufbuild/protocompile/internal/ext/stringsx"
)

// Render renders a document consisting of the given sequence of tags.
func Render(options Options, content func(push Sink)) string {
	d := new(dom)
	content(d.add)
	return render(options, d)
}

// Options specifies configuration for [Render].
type Options struct {
	// The maximum number of columns to render before triggering
	// a break. A value of zero implies an infinite width.
	MaxWidth int

	// The number of columns a tab character counts as. Defaults to 1.
	TabstopWidth int

	// If true, prints all of the tags in an HTML-like format. Intended for
	// debugging.
	HTML bool
}

// WithDefaults replaces any unset (read: zero value) fields of an Options which
// specify a default value with that default value.
func (o Options) WithDefaults() Options {
	if o.MaxWidth == 0 {
		o.MaxWidth = math.MaxInt
	}
	if o.TabstopWidth == 0 {
		o.TabstopWidth = 1
	}
	return o
}

// Tag is data passed to a rendering function.
//
// The various factory functions in this package can be used to construct tags.
// See their documentation for more information on what tags are available.
//
// The nil tag is equivalent to Text("").
type Tag func(*dom)

// Sink is a place to append tags. The given tags will be appended to whatever
// context the sink was created for.
//
// Many functions in this package take a func(push Sink) as an argument. This
// callback is executed in the context of that tag, and must not be used after
// the callback returns.
type Sink func(...Tag)

const (
	Always Cond = iota
	Flat        // Render only in a flat group.
	Broken      // Render only in a broken group.
)

// Cond is a condition for a tag.
//
// Tags can be conditioned on whether or not they are rendered if the group
// that they are being rendered in is flat or broken.
type Cond byte

// Text returns a tag that emits its text exactly.
//
// If text consists only of spaces (U+0020) or newlines (U+000A), it will be
// treated specially:
//
//   - Space tags adjacent to a newline will be deleted, so that lines do not
//     have trailing whitespace.
//
//   - If two space or newline tags of the same rune are adjacent, the shorter
//     one is deleted.
func Text(text string) Tag {
	return TextIf(Always, text)
}

// TextIf is like [Text], but with a condition attached.
//
// If the condition does not hold in the containing tag, this tag expands to
// nothing. The outermost level is treated as always broken.
func TextIf(cond Cond, text string) Tag {
	return func(d *dom) {
		if text == "" {
			return
		}

		var kind kind
		switch {
		case stringsx.Every(text, ' '):
			kind = kindSpace
		case stringsx.Every(text, '\n'):
			kind = kindBreak
		default:
			kind = kindText
		}

		d.push(tag{kind: kind, text: text, cond: cond}, nil)
	}
}

// Group returns a tag that groups together a collection of child tags.
//
// Each group in a document can be broken or flat. Flat groups contain only
// other flat groups. Groups are broken when:
//
// 1. They contain a tag that contains a newline.
//
// 2. They contain a broken group.
//
//  3. The width of the group when flat is greater than maxWidth (a value of
//     zero implies no limit).
//
//  4. If the group was laid out flat, the current line would exceed the maximum
//     configured column length in [Options].
func Group(maxWidth int, content func(push Sink)) Tag {
	return GroupIf(Always, maxWidth, content)
}

// GroupIf is like [Group], but with a condition attached.
//
// If the condition does not hold in the containing tag, this tag expands to
// nothing. The outermost level is treated as always broken.
func GroupIf(cond Cond, maxWidth int, content func(push Sink)) Tag {
	return func(d *dom) {
		if maxWidth == 0 {
			maxWidth = math.MaxInt
		}
		d.push(tag{kind: kindGroup, limit: maxWidth, cond: cond}, content)
	}
}

// Indent pushes by to the indentation stack for all of the given tags.
//
// The indentation stack consists of strings printed on each new line, if that
// line is otherwise not empty.
//
// Indent cannot be conditioned, because it already has no effect in a flat
// group.
func Indent(by string, content func(push Sink)) Tag {
	return func(d *dom) {
		if by == "" {
			content(d.add)
			return
		}
		d.push(tag{kind: kindIndent, text: by}, content)
	}
}

// Unindent pops the last [Indent] for all of the given tags.
//
// Unindent cannot be conditioned, because it already has no effect in a flat
// group.
func Unindent(content func(push Sink)) Tag {
	return func(d *dom) {
		d.push(tag{kind: kindUnindent}, content)
	}
}
