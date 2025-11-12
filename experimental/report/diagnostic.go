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

import (
	"fmt"

	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/internal/ext/slicesx"
)

// Level represents the severity of a diagnostic message.
type Level int8

const (
	// Internal compiler error. Indicates a panic within the compiler.
	ICE Level = 1 + iota
	// Red. Indicates a semantic constraint violation.
	Error
	// Yellow. Indicates something that probably should not be ignored.
	Warning
	// Cyan. This is the diagnostics version of "info".
	Remark

	noteLevel // Used internally within the diagnostic renderer.
)

// Diagnostic is a type of error that can be rendered as a rich diagnostic.
//
// Not all Diagnostics are "errors", even though Diagnostic does embed error;
// some represent warnings, or perhaps debugging remarks.
//
// To construct a diagnostic, create one using a function like [Report.Error].
// Then, call [Diagnostic.Apply] to apply options to it. You should at minimum
// apply [Message] and either [InFile] or at least one [Snippetf].
type Diagnostic struct {
	tag, message string

	level Level

	// sortOrder is used to force diagnostics to sort before or after each other
	// in groups. See [Report.Sort].
	sortOrder int
	// The file this diagnostic occurs in, if it has no associated Annotations. This
	// is used for errors like "file too big" that cannot be given a snippet.
	inFile string

	// A list of annotated source code spans in the diagnostic.
	snippets           []snippet
	notes, help, debug []string
}

// Edit is an edit to suggest on a snippet.
//
// See [SuggestEdits].
type Edit struct {
	// The start and end offsets of the edit, relative the span of the snippet
	// this edit is applied to (so, Start == 0 means the edit starts at the
	// start of the span).
	//
	// An insertion without deletion is modeled by Start == End.
	Start, End int

	// Text to replace the content between Start and End with.
	//
	// A pure deletion is modeled by Replace == "".
	Replace string
}

// IsDeletion returns whether this edit involves deleting part of the source
// text.
func (e Edit) IsDeletion() bool {
	return e.Start < e.End
}

// IsInsertion returns whether this edit involves inserting new text.
func (e Edit) IsInsertion() bool {
	return e.Replace != ""
}

// DiagnosticOption is an option that can be applied to a [Diagnostic].
//
// IsZero values passed to [Diagnostic.Apply] are ignored.
type DiagnosticOption interface {
	apply(*Diagnostic)
}

// Primary returns this diagnostic's primary span, if it has one.
//
// If it doesn't have one, it returns the zero span.
func (d *Diagnostic) Primary() source.Span {
	for _, annotation := range d.snippets {
		if annotation.primary {
			return annotation.Span
		}
	}

	return source.Span{}
}

// Level returns this diagnostic's level.
func (d *Diagnostic) Level() Level {
	return d.level
}

// Message returns this diagnostic's message, set using [Message].
func (d *Diagnostic) Message() string {
	return d.message
}

// Tag returns this diagnostic's tag, set using [Tag].
func (d *Diagnostic) Tag() string {
	return d.tag
}

// InFile returns the path of the file set using [InFile]. This path can be used if this
// diagnostic does not have a primary span to mention the given file and/or no annotations.
func (d *Diagnostic) InFile() string {
	return d.inFile
}

// Notes returns this diagnostic's notes, set using [Notef].
func (d *Diagnostic) Notes() []string {
	return d.notes
}

// Help returns this diagnostic's suggestions, set using [Helpf].
func (d *Diagnostic) Help() []string {
	return d.help
}

// Debug returns this diagnostic's debugging information, set using [Debugf].
func (d *Diagnostic) Debug() []string {
	return d.debug
}

// Is checks whether this diagnostic has a particular tag.
func (d *Diagnostic) Is(tag string) bool {
	return d.tag == tag
}

// Apply applies the given options to this diagnostic.
//
// Nil values are ignored; does nothing if d is nil.
func (d *Diagnostic) Apply(options ...DiagnosticOption) *Diagnostic {
	if d != nil {
		for _, option := range options {
			if option != nil {
				option.apply(d)
			}
		}
	}
	return d
}

// Tag returns a DiagnosticOption that sets a diagnostic's tag.
//
// Tags are machine-readable identifiers for diagnostics. Tags should be
// lowercase identifiers separated by dashes, e.g. my-error-tag. If a package
// generates diagnostics with tags, it should expose those tags as constants.
func Tag(t string) DiagnosticOption {
	return tag(t)
}

// Message returns a DiagnosticOption that sets the main diagnostic message.
func Message(format string, args ...any) DiagnosticOption {
	return message{format, args}
}

// InFile returns a DiagnosticOption that causes a diagnostic without a primary
// span to mention the given file.
func InFile(path string) DiagnosticOption {
	return inFile(path)
}

// Snippet is like [Snippetf], but it attaches no message to the snippet.
//
// The first annotation added is the "primary" annotation, and will be rendered
// differently from the others.
//
// If at is nil or returns the zero span, the returned DiagnosticOption is a no-op.
func Snippet(at source.Spanner) DiagnosticOption {
	return Snippetf(at, "")
}

// Snippetf returns a DiagnosticOption that adds a new snippet to a diagnostic.
//
// Any additional arguments to this function are passed to [fmt.Sprintf] to
// produce a message to go with the span.
//
// The first annotation added is the "primary" annotation, and will be rendered
// differently from the others.
//
// If at is nil or returns the zero span, the returned DiagnosticOption is a no-op.
func Snippetf(at source.Spanner, format string, args ...any) DiagnosticOption {
	return snippet{
		Span:    source.GetSpan(at),
		message: fmt.Sprintf(format, args...),
	}
}

// SuggestEdits is like [Snippet], but generates a snippet that contains
// machine-applicable suggestions.
//
// A snippet with suggestions will be displayed separately from other snippets.
// The message associated with the snippet will be prefixed with "help:" when
// rendered.
func SuggestEdits(at source.Spanner, message string, edits ...Edit) DiagnosticOption {
	span := source.GetSpan(at)
	text := span.Text()
	for _, edit := range edits {
		// Force a bounds check here to make it easier to debug, instead of
		// panicking in the renderer (or emitting an invalid report proto).
		_ = text[edit.Start:edit.End]
	}

	return snippet{
		Span:    span,
		message: message,
		edits:   edits,
	}
}

// SuggestEditsWithWidening is like [SuggestEdits], but it allows edits' starts and
// ends to not conform to the given span exactly (e.g., the end points are
// negative or greater than the length of the span).
//
// This will widen the span for the suggestion to fit the edits.
func SuggestEditsWithWidening(at source.Spanner, message string, edits ...Edit) DiagnosticOption {
	span := source.GetSpan(at)
	start := span.Start
	span = source.JoinSeq(slicesx.Map(edits, func(e Edit) source.Span {
		return span.File.Span(e.Start+start, e.End+start)
	}))
	delta := start - span.Start

	for i := range edits {
		edits[i].Start += delta
		edits[i].End += delta
	}

	return SuggestEdits(span, message, edits...)
}

// Notef returns a DiagnosticOption that provides the user with context about the
// diagnostic, after the annotations.
func Notef(format string, args ...any) DiagnosticOption {
	return note{format, args}
}

// Helpf returns a DiagnosticOption that provides the user with a helpful prose
// suggestion for resolving the diagnostic.
func Helpf(format string, args ...any) DiagnosticOption {
	return help{format, args}
}

// Debugf returns a DiagnosticOption appends debugging information to a diagnostic that
// is not intended to be shown to normal users.
func Debugf(format string, args ...any) DiagnosticOption {
	return debug{format, args}
}

// PageBreak is a DiagnosticOption that inserts a "page break", separating
// diagnostic snippets before and after it into separate windows.
var PageBreak pageBreak

// snippet is an annotated source code snippet within a [Diagnostic].
//
// Snippets will render as annotated source code spans that show the context
// around the annotated region. More literally, this is e.g. a red squiggly
// line under some code.
type snippet struct {
	// The span for this annotation.
	source.Span

	// A message to show under this snippet.
	//
	// May be empty, in which case it will simply render as the red/yellow/etc
	// squiggly line with no note attached to it. This is useful for cases where
	// the overall error message already explains what the problem is and there
	// is no additional context that would be useful to add to the error.
	message string

	// Whether this is a "primary" snippet, which is used for deciding whether or not
	// to mark the snippet with the same color as the overall diagnostic.
	primary bool

	// Whether this snippet ends in a page break, i.e., it should not be
	// rendered together with following snippets, even if they're in the same
	// file.
	pageBreak bool

	// Edits to include in this snippet. This causes this snippet to be rendered
	// in its own window when it is non-empty, and no underline will appear for
	// the overall span of the snippet. The message will still be used, as the
	// title of the window.
	edits []Edit
}

func (a snippet) apply(d *Diagnostic) {
	if a.Span.IsZero() {
		return
	}

	a.primary = len(d.snippets) == 0
	d.snippets = append(d.snippets, a)
}

type lazySprintf struct {
	format string
	args   []any
}

func (ls lazySprintf) String() string {
	return fmt.Sprintf(ls.format, ls.args...)
}

type tag string
type inFile string

type message lazySprintf
type note lazySprintf
type help lazySprintf
type debug lazySprintf

type pageBreak struct{}

func (t tag) apply(d *Diagnostic) {
	if d.tag != "" {
		panic("protocompile/report: set diagnostic tag more than once")
	}

	d.tag = string(t)
}

func (f inFile) apply(d *Diagnostic) {
	if d.inFile != "" {
		panic("protocompile/report: set diagnostic path more than once")
	}

	d.inFile = string(f)
}

func (m message) apply(d *Diagnostic) {
	if d.message != "" {
		panic("protocompile/report: set diagnostic message more than once")
	}

	d.message = lazySprintf(m).String()
}

func (n note) apply(d *Diagnostic)  { d.notes = append(d.notes, lazySprintf(n).String()) }
func (n help) apply(d *Diagnostic)  { d.help = append(d.help, lazySprintf(n).String()) }
func (n debug) apply(d *Diagnostic) { d.debug = append(d.debug, lazySprintf(n).String()) }

func (pageBreak) apply(d *Diagnostic) {
	if len(d.snippets) == 0 {
		return
	}
	d.snippets[len(d.snippets)-1].pageBreak = true
}
