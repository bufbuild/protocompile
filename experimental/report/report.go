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

// Diagnose is an error that can be rendered as a diagnostic.
type Diagnose interface {
	error

	// Diagnose writes out this error to the given diagnostic.
	//
	// This function should not set Level nor Err; those are set by the
	// diagnostics framework.
	Diagnose(*Diagnostic)
}

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

	// The file this diagnostic occurs in, if it has no associated Annotations. This
	// is used for errors like "file too big" that cannot be given a snippet.
	InFile string

	// A list of annotated source code spans in the diagnostic.
	Annotations []Annotation

	// Notes and help messages to include at the end of the diagnostic, after the
	// Annotations.
	Notes, Help, Debug []string

	// Stack trace information for the diagnostic, for use in debugging
	// the compiler. Only populated when the env var PROTOCOMPILE_DEBUG is set.
	trace []runtime.Frame
}

// Annotation is an annotated source code snippet within a [Diagnostic].
type Annotation struct {
	// The file this snippet is from. Note that Annotations with the same file name
	// are treated as being part of the same file, regardless of that file's contents.
	File File
	// Start and end positions for this snippet, within the above file.
	Start, End Location
	// A message to show under this snippet. May be empty.
	Message string
	// Whether this is a "primary" snippet, which is used for deciding whether or not
	// to mark the snippet with the same color as the overall diagnostic.
	Primary bool
}

// Primary returns this diagnostic's primary snippet, if it has one.
//
// If it doesn't have one, it returns a dummy annotation referring to InFile.
func (d *Diagnostic) Primary() Annotation {
	for _, annotation := range d.Annotations {
		if annotation.Primary {
			return annotation
		}
	}

	return Annotation{
		File:    File{Path: d.InFile},
		Primary: true,
	}
}

// With applies the given options to this diagnostic.
func (d *Diagnostic) With(options ...DiagnosticOption) {
	for _, option := range options {
		option(d)
	}
}

// DiagnosticOption is an option that can be applied to a [Diagnostic].
type DiagnosticOption func(*Diagnostic)

// InFile returns a DiagnosticOption that causes a diagnostic without a primary
// span to mention the given file.
func InFile(path string) DiagnosticOption {
	return func(d *Diagnostic) { d.InFile = path }
}

// Snippetf returns a DiagnosticOption that adds a new snippet to a diagnostic.
//
// The first annotation added is the "primary" annotation, and will be rendered
// differently from the others.
func Snippet[Spanner interface{ Span() S }, S Span](at Spanner) DiagnosticOption {
	return Snippetf(at, "")
}

// Snippetf returns a DiagnosticOption that adds a new snippet to a diagnostic with the given message.
//
// The first annotation added is the "primary" annotation, and will be rendered
// differently from the others.
func Snippetf[Spanner interface{ Span() S }, S Span](at Spanner, format string, args ...any) DiagnosticOption {
	return SnippetAtf(at.Span(), format, args...)
}

// SnippetAtf is like [Snippet], but takes a span rather than something with a Span() method.
func SnippetAt(span Span) DiagnosticOption {
	return SnippetAtf(span, "")
}

// SnippetAtf is like [Snippetf], but takes a span rather than something with a Span() method.
func SnippetAtf(span Span, format string, args ...any) DiagnosticOption {
	// This is hoisted out to improve stack traces when something goes awry in the
	// argument to With(). By hoisting, it correctly blames the right invocation to Snippet().
	annotation := Annotation{
		File:    span.File(),
		Start:   span.Start(),
		End:     span.End(),
		Message: fmt.Sprintf(format, args...),
	}
	return func(d *Diagnostic) {
		annotation.Primary = len(d.Annotations) == 0
		d.Annotations = append(d.Annotations, annotation)
	}
}

// Note returns a DiagnosticOption that provides the user with context about the
// diagnostic, after the annotations.
func Note(format string, args ...any) DiagnosticOption {
	return func(d *Diagnostic) {
		d.Notes = append(d.Notes, fmt.Sprintf(format, args...))
	}
}

// Help returns a DiagnosticOption that provides the user with a helpful prose
// suggestion for resolving the diagnostic.
func Help(format string, args ...any) DiagnosticOption {
	return func(d *Diagnostic) {
		d.Help = append(d.Help, fmt.Sprintf(format, args...))
	}
}

// Report is a collection of diagnostics.
type Report []Diagnostic

// Error pushes an error diagnostic onto this report.
func (r *Report) Error(err Diagnose) {
	err.Diagnose(r.push(1, err, Error))
}

// Warn pushes a warning diagnostic onto this report.
func (r *Report) Warn(err Diagnose) {
	err.Diagnose(r.push(1, err, Warning))
}

// Remark pushes a remark diagnostic onto this report.
func (r *Report) Remark(err Diagnose) {
	err.Diagnose(r.push(1, err, Remark))
}

// Errorf creates a new error diagnostic with an unspecified error type; analogous to
// [fmt.Errorf].
func (r *Report) Errorf(format string, args ...any) *Diagnostic {
	return r.push(1, fmt.Errorf(format, args...), Error)
}

// Warnf creates a new warning diagnostic with an unspecified error type; analogous to
// [fmt.Errorf].
func (r *Report) Warnf(format string, args ...any) *Diagnostic {
	return r.push(1, fmt.Errorf(format, args...), Warning)
}

// Remarkf creates a new remark diagnostic with an unspecified error type; analogous to
// [fmt.Errorf].
func (r *Report) Remarkf(format string, args ...any) *Diagnostic {
	return r.push(1, fmt.Errorf(format, args...), Remark)
}

// push is the core "make me a diagnostic" function.
func (r *Report) push(skip int, err error, level Level) *Diagnostic {
	*r = append(*r, Diagnostic{Err: err, Level: level})
	d := &(*r)[len(*r)-1]

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
	return d
}
