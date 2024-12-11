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

// Tag is a diagnostic tag: a machine-readable identification for a diagnostic.
//
// Tags should be lowercase identifiers separated by dashes, e.g. my-error-tag.
// If a package generates diagnostics with tags, it should expose those tags as
// constants.
type Tag string

// Apply implements [DiagnosticOption].
func (t Tag) Apply(d *Diagnostic) {
	if d.tag != "" {
		panic("protocompile/report: set diagnostic tag more than once")
	}

	d.tag = t
}

// Diagnostic is a type of error that can be rendered as a rich diagnostic.
//
// Not all Diagnostics are "errors", even though Diagnostic does embed error;
// some represent warnings, or perhaps debugging remarks.
//
// To construct a diagnostic, create one using a function like [Report.Error].
// Then, call [Diagnostic.With] to apply options to it. You should at minimum
// apply [Message] and either [InFile] or at least one [Snippet].
type Diagnostic struct {
	tag     Tag
	message string

	level Level

	// sortOrder is used to force diagnostics to sort before or after each other
	// in groups. See [Report.Sort].
	sortOrder int
	// The file this diagnostic occurs in, if it has no associated Annotations. This
	// is used for errors like "file too big" that cannot be given a snippet.
	inFile string

	// A list of annotated source code spans in the diagnostic.
	annotations        []annotation
	notes, help, debug []string
}

// DiagnosticOption is an option that can be applied to a [Diagnostic].
//
// Nil values passed to [Diagnostic.With] are ignored.
type DiagnosticOption interface {
	Apply(*Diagnostic)
}

// Primary returns this diagnostic's primary span, if it has one.
//
// If it doesn't have one, it returns the zero span.
func (d *Diagnostic) Primary() Span {
	for _, annotation := range d.annotations {
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
func (d *Diagnostic) Is(tag Tag) bool {
	return d.tag == tag
}

// With applies the given options to this diagnostic.
//
// Nil values are ignored.
func (d *Diagnostic) With(options ...DiagnosticOption) *Diagnostic {
	for _, option := range options {
		if option != nil {
			option.Apply(d)
		}
	}
	return d
}

// Message returns a Diagnostic option that sets the main diagnostic message.
func Message(format string, args ...any) DiagnosticOption {
	return message(fmt.Sprintf(format, args...))
}

// InFile is a DiagnosticOption that causes a diagnostic without a primary
// span to mention the given file.
type InFile string

// Apply implements [DiagnosticOption]
func (f InFile) Apply(d *Diagnostic) {
	if d.inFile != "" {
		panic("protocompile/report: set diagnostic path more than once")
	}

	d.inFile = string(f)
}

// Snippetf returns a DiagnosticOption that adds a new snippet to a diagnostic.
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

	annotation := annotation{Span: span}
	if len(args) > 0 {
		format, ok := args[0].(string)
		if !ok {
			panic("protocompile/report: expected string as first Snippet argument")
		}

		annotation.message = fmt.Sprintf(format, args[1:]...)
	}

	return annotation
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

// annotation is an annotated source code snippet within a [Diagnostic].
//
// Snippets will render as annotated source code spans that show the context
// around the annotated region. More literally, this is e.g. a red squiggly
// line under some code.
type annotation struct {
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

func (a annotation) Apply(d *Diagnostic) {
	a.primary = len(d.annotations) == 0
	d.annotations = append(d.annotations, a)
}

type message string
type note string
type help string
type debug string

func (m message) Apply(d *Diagnostic) {
	if d.message != "" {
		panic("protocompile/report: set diagnostic message more than once")
	}

	d.message = string(m)
}

func (n note) Apply(d *Diagnostic)  { d.notes = append(d.notes, string(n)) }
func (n help) Apply(d *Diagnostic)  { d.help = append(d.help, string(n)) }
func (n debug) Apply(d *Diagnostic) { d.debug = append(d.debug, string(n)) }
