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

package report2

import (
	"fmt"
	"runtime"
)

const (
	Error Level = 1 + iota
	Warning
	Remark
	note // Used internally within the diagnostic renderer.
)

const (
	Simple Style = 1 + iota
	Monochrome
	Colored
)

// Level represents the severity of a diagnostic message.
type Level int8

// Style indicates how a diagnostic should be rendered to show a user.
type Style int

// Diagnostic is a type of error that can be rendered as a rich diagnostic.
//
// Not all Diagnostics are "errors", even though Diagnostic does embed error;
// some represent warnings, or perhaps debugging remarks.
type Diagnostic struct {
	// The error that prompted this diagnostic. Its Error() return is used
	// as the diagnostic message.
	Err error

	// The kind of diagnostic this is, which affects how and whether it is shown
	// to users.
	Level Level

	mention     string
	snippets    []snippet
	notes, help []string

	// Stack trace information for the diagnostic, for use in debugging
	// the compiler. Only populated when the env var PROTOCOMPILE_DEBUG is set.
	trace []runtime.Frame
}

type snippet struct {
	file       File
	start, end Location
	message    string
	primary    bool
}

// Primary returns this diagnostic's primary diagnostic, if it has one.
func (d *Diagnostic) Primary() (file File, start, end Location) {
	if len(d.snippets) == 0 {
		file.Path = d.mention
		return
	}

	return d.snippets[0].file, d.snippets[0].start, d.snippets[0].end
}

// DiagnosticOption is an option that can be applied to a [Diagnostic].
type DiagnosticOption func(*Diagnostic)

// MentionFile returns a DiagnosticOption that causes a diagnostic without
// a primary span to mention the given file.
func MentionFile(path string) DiagnosticOption {
	return func(d *Diagnostic) { d.mention = path }
}

// Snippet returns a DiagnosticOption that adds a new snippet to the
// diagnostic.
//
// The first snippet added is the "primary" snippet, and will be rendered
// differently from the others.
func Snippet[Spanner interface{ Span() S }, S Span](at Spanner, format string, args ...any) DiagnosticOption {
	return SnippetAt(at.Span(), format, args...)
}

// SnippetAt is like Snippet, but takes a span rather than something with a Span() method.
func SnippetAt(span Span, format string, args ...any) DiagnosticOption {
	return func(d *Diagnostic) {
		d.snippets = append(d.snippets, snippet{
			file:    span.File(),
			start:   span.Start(),
			end:     span.End(),
			message: fmt.Sprintf(format, args...),
			primary: len(d.snippets) == 0,
		})
	}
}

// Note returns a DiagnosticOption that provides the user with context about the
// diagnostic, after the snippets.
func Note(format string, args ...any) DiagnosticOption {
	return func(d *Diagnostic) {
		d.notes = append(d.notes, fmt.Sprintf(format, args...))
	}
}

// Help returns a DiagnosticOption that provides the user with a helpful prose
// suggestion for resolving the diagnostic.
func Help(format string, args ...any) DiagnosticOption {
	return func(d *Diagnostic) {
		d.help = append(d.help, fmt.Sprintf(format, args...))
	}
}

// Report is a collection of diagnostics.
type Report []Diagnostic

// Error pushes an error diagnostic onto this report.
func (r *Report) Error(err error, opts ...DiagnosticOption) {
	r.push(1, err, Error, opts)
}

// Warn pushes a warning diagnostic onto this report.
func (r *Report) Warn(err error, opts ...DiagnosticOption) {
	r.push(1, err, Warning, opts)
}

// Remark pushes a remark diagnostic onto this report.
func (r *Report) Remark(err error, opts ...DiagnosticOption) {
	r.push(1, err, Remark, opts)
}

// push is the core "make me a diagnostic" function.
func (r *Report) push(skip int, err error, level Level, opts []DiagnosticOption) {
	*r = append(*r, Diagnostic{Err: err, Level: level})
	d := &(*r)[len(*r)-1]
	for _, opt := range opts {
		opt(d)
	}

	// If debugging is on, capture a stack trace.
	if debugMode > debugOff {
		// Unwind the stack to find program counter information.
		pc := make([]uintptr, 64)
		pc = pc[:runtime.Callers(skip+2, pc)]

		// Fill trace with the result.
		var zero runtime.Frame
		frames := runtime.CallersFrames(pc)
		for {
			next, more := frames.Next()
			if next != zero {
				d.trace = append(d.trace, next)
			}
			if !more {
				break
			}
		}
	}
}
