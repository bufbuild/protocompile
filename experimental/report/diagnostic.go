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

package report

import (
	"fmt"

	"github.com/bufbuild/protocompile/experimental/internal"
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
// apply [Message] and either [InFile] or at least one [Snippet].
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

// DiagnosticOption is an option that can be applied to a [Diagnostic].
//
// Nil values passed to [Diagnostic.Apply] are ignored.
type DiagnosticOption interface {
	apply(*Diagnostic)
}

// Primary returns this diagnostic's primary span, if it has one.
//
// If it doesn't have one, it returns the zero span.
func (d *Diagnostic) Primary() Span {
	for _, annotation := range d.snippets {
		if annotation.primary {
			return annotation.Span
		}
	}

	return Span{}
}

// Level returns this diagnostic's level.
func (d *Diagnostic) Level() Level {
	return d.level
}

// Is checks whether this diagnostic has a particular tag.
func (d *Diagnostic) Is(tag string) bool {
	return d.tag == tag
}

// Apply applies the given options to this diagnostic.
//
// Nil values are ignored.
func (d *Diagnostic) Apply(options ...DiagnosticOption) *Diagnostic {
	for _, option := range options {
		if option != nil {
			option.apply(d)
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

type tag string

func (t tag) apply(d *Diagnostic) {
	if d.tag != "" {
		panic("protocompile/report: set diagnostic tag more than once")
	}

	d.tag = string(t)
}

// Message returns a DiagnosticOption that sets the main diagnostic message.
func Message(format string, args ...any) DiagnosticOption {
	return message(fmt.Sprintf(format, args...))
}

// InFile returns a DiagnosticOption that causes a diagnostic without a primary
// span to mention the given file.
func InFile(path string) DiagnosticOption {
	return inFile(path)
}

// Snippet returns a DiagnosticOption that adds a new snippet to a diagnostic.
//
// Any additional arguments to this function are passed to [fmt.Sprintf] to
// produce a message to go with the span. Snippet(span) is equivalent to
// Snippet(span, "").
//
// The first annotation added is the "primary" annotation, and will be rendered
// differently from the others.
//
// If at is nil (be it a nil interface, or a value that has a Nil() function
// that returns true), or returns a nil span, this function will return nil.
func Snippet(at Spanner, args ...any) DiagnosticOption {
	if internal.Nil(at) {
		return nil
	}

	span := at.Span()
	if span.Nil() {
		return nil
	}

	snippet := snippet{Span: span}
	if len(args) > 0 {
		format, ok := args[0].(string)
		if !ok {
			panic("protocompile/report: expected string as first Snippet argument")
		}

		snippet.message = fmt.Sprintf(format, args[1:]...)
	}

	return snippet
}

// Note returns a DiagnosticOption that provides the user with context about the
// diagnostic, after the annotations.
func Note(format string, args ...any) DiagnosticOption {
	return note(fmt.Sprintf(format, args...))
}

// Help returns a DiagnosticOption that provides the user with a helpful prose
// suggestion for resolving the diagnostic.
func Help(format string, args ...any) DiagnosticOption {
	return help(fmt.Sprintf(format, args...))
}

// Debug returns a DiagnosticOption appends debugging information to a diagnostic that
// is not intended to be shown to normal users.
func Debug(format string, args ...any) DiagnosticOption {
	return debug(fmt.Sprintf(format, args...))
}

// snippet is an annotated source code snippet within a [Diagnostic].
//
// Snippets will render as annotated source code spans that show the context
// around the annotated region. More literally, this is e.g. a red squiggly
// line under some code.
type snippet struct {
	// The span for this annotation.
	Span

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
}

func (a snippet) apply(d *Diagnostic) {
	a.primary = len(d.snippets) == 0
	d.snippets = append(d.snippets, a)
}

type message string
type inFile string
type note string
type help string
type debug string

func (m message) apply(d *Diagnostic) {
	if d.message != "" {
		panic("protocompile/report: set diagnostic message more than once")
	}

	d.message = string(m)
}

func (f inFile) apply(d *Diagnostic) {
	if d.inFile != "" {
		panic("protocompile/report: set diagnostic path more than once")
	}

	d.inFile = string(f)
}

func (n note) apply(d *Diagnostic)  { d.notes = append(d.notes, string(n)) }
func (n help) apply(d *Diagnostic)  { d.help = append(d.help, string(n)) }
func (n debug) apply(d *Diagnostic) { d.debug = append(d.debug, string(n)) }
