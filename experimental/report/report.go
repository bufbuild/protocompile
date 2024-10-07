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
	"errors"
	"fmt"
	"runtime"
	"slices"
	"strings"

	"google.golang.org/protobuf/proto"

	compilerpb "github.com/bufbuild/protocompile/internal/gen/buf/compiler/v1"
)

const (
	// Red. Indicates a semantic constraint violation.
	Error Level = 1 + iota
	// Yellow. Indicates something that probably should not be ignored.
	Warning
	// Cyan. This is the diagnostics version of "info".
	Remark

	note // Used internally within the diagnostic renderer.
)

// Level represents the severity of a diagnostic message.
type Level int8

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

	// Stage is an opaque identifier for the "stage" that a diagnostic occurred in.
	// See [Report.Sort].
	Stage int

	// The file this diagnostic occurs in, if it has no associated Annotations. This
	// is used for errors like "file too big" that cannot be given a snippet.
	InFile string

	// A list of annotated source code spans in the diagnostic.
	Annotations []Annotation

	// Notes and help messages to include at the end of the diagnostic, after the
	// Annotations.
	Notes, Help, Debug []string
}

// Annotation is an annotated source code snippet within a [Diagnostic].
type Annotation struct {
	// A message to show under this snippet. May be empty.
	Message string

	// Whether this is a "primary" snippet, which is used for deciding whether or not
	// to mark the snippet with the same color as the overall diagnostic.
	Primary bool

	// The file this snippet is from. Note that Annotations with the same file name
	// are treated as being part of the same file, regardless of that file's contents.
	File File
	// Start and end positions for this snippet, within the above file.
	Start, End Location
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
//
// The arguments are stringified with [fmt.Sprint].
func Note(args ...any) DiagnosticOption {
	return func(d *Diagnostic) {
		d.Notes = append(d.Notes, fmt.Sprint(args...))
	}
}

// Notef is like [Note], but it calls [fmt.Sprintf] internally for you.
func Notef(format string, args ...any) DiagnosticOption {
	return func(d *Diagnostic) {
		d.Notes = append(d.Notes, fmt.Sprintf(format, args...))
	}
}

// Help returns a DiagnosticOption that provides the user with a helpful prose
// suggestion for resolving the diagnostic.
//
// The arguments are stringified with [fmt.Sprint].
func Help(args ...any) DiagnosticOption {
	return func(d *Diagnostic) {
		d.Help = append(d.Help, fmt.Sprint(args...))
	}
}

// Helpf is like [Help], but it calls [fmt.Sprintf] internally for you.
func Helpf(format string, args ...any) DiagnosticOption {
	return func(d *Diagnostic) {
		d.Help = append(d.Help, fmt.Sprintf(format, args...))
	}
}

// Debug returns a DiagnosticOption appends debugging information to a diagnostic that
// is not intended to be shown to normal users.
//
// The arguments are stringified with [fmt.Sprint].
func Debug(args ...any) DiagnosticOption {
	return func(d *Diagnostic) {
		d.Help = append(d.Help, fmt.Sprint(args...))
	}
}

// Debugf is like [Debug], but it calls [fmt.Sprintf] internally for you.
func Debugf(format string, args ...any) DiagnosticOption {
	return func(d *Diagnostic) {
		d.Help = append(d.Help, fmt.Sprintf(format, args...))
	}
}

// Report is a collection of diagnostics.
//
// Report is not thread-safe (in the sense that distinct goroutines should not
// all write to Report at the same time). Instead, the recommendation is to create
// multiple reports and then merge them, using [Report.Sort] to canonicalize the result.
type Report struct {
	// The actual diagnostics on this report. Generally, you'll want to use one of
	// the helpers like [Report.Error] instead of appending directly.
	Diagnostics []Diagnostic

	// The stage to apply to any new diagnostics created with this report.
	//
	// Diagnostics with the same stage will sort together. See [Report.Sort].
	Stage int

	// When greater than zero, this will capture debugging information at the
	// site of each call to Error() etc. This will make diagnostic construction
	// orders of magnitude slower; it is intended to help tool writers to debug
	// their diagnostics.
	//
	// Higher values mean more debugging information. What debugging information
	// is actually provided is subject to change.
	Tracing int
}

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

// Sort canonicalizes this report's diagnostic order according to an specific
// ordering criteria. Diagnostics are sorted by, in order;
//
// File name of primary span, stage, start offset of primary snippet, end offset
// of primary snippet, content of error message.
//
// Where diagnostics have no primary span, the file is treated as empty and the
// offsets are treated as zero.
//
// These criteria ensure that diagnostics for the same file go together,
// diagnostics for the same stage (lex, parse, etc) go together, and they are
// otherwise ordered by where they occur in the file.
func (r *Report) Sort() {
	slices.SortFunc(r.Diagnostics, func(a, b Diagnostic) int {
		aPrime := a.Primary()
		bPrime := b.Primary()

		if diff := strings.Compare(aPrime.File.Path, bPrime.File.Path); diff != 0 {
			return diff
		}

		if diff := a.Stage - b.Stage; diff != 0 {
			return diff
		}

		if diff := aPrime.Start.Offset - bPrime.Start.Offset; diff != 0 {
			return diff
		}

		if diff := aPrime.End.Offset - bPrime.End.Offset; diff != 0 {
			return diff
		}

		return strings.Compare(a.Err.Error(), b.Err.Error())
	})
}

// ToProto converts this report into a Protobuf message for serialization.
//
// This operation is lossy: only the Diagnostics slice is serialized. It also discards
// concrete types of Diagnostic.Err, replacing them with opaque [errors.New] values
// on deserialization.
//
// It will also deduplicate [File] values based on their paths, paying no attention to
// their contents.
func (r *Report) ToProto() proto.Message {
	proto := new(compilerpb.Report)

	fileToIndex := map[string]uint32{}
	for _, d := range r.Diagnostics {
		dProto := &compilerpb.Diagnostic{
			Message: d.Err.Error(),
			Level:   compilerpb.Diagnostic_Level(d.Level),
			InFile:  d.InFile,
			Notes:   d.Notes,
			Help:    d.Help,
			Debug:   d.Debug,
		}

		for _, snip := range d.Annotations {
			file, ok := fileToIndex[snip.File.Path]
			if !ok {
				file = uint32(len(proto.Files))
				fileToIndex[snip.File.Path] = file

				proto.Files = append(proto.Files, &compilerpb.Report_File{
					Path: snip.File.Path,
					Text: []byte(snip.File.Text),
				})
			}

			dProto.Annotations = append(dProto.Annotations, &compilerpb.Diagnostic_Annotation{
				File:    file,
				Start:   uint32(snip.Start.Offset),
				End:     uint32(snip.End.Offset),
				Message: snip.Message,
				Primary: snip.Primary,
			})
		}

		proto.Diagnostics = append(proto.Diagnostics, dProto)
	}

	return proto
}

// FromProto appends diagnostics from a Protobuf message to this report.
//
// deserialize will be called with an empty message that should be deserialized
// onto, which this function will then convert into [Diagnostic]s to populate the
// report with.
func (r *Report) AppendFromProto(deserialize func(proto.Message) error) error {
	proto := new(compilerpb.Report)
	if err := deserialize(proto); err != nil {
		return err
	}

	files := make([]*IndexedFile, len(proto.Files))
	for i, fProto := range proto.Files {
		files[i] = NewIndexedFile(File{
			Path: fProto.Path,
			Text: string(fProto.Text),
		})
	}

	for i, dProto := range proto.Diagnostics {
		if dProto.Message == "" {
			return fmt.Errorf("protocompile/report: missing message for diagnostic[%d]", i)
		}
		level := Level(dProto.Level)
		switch level {
		case Error, Warning, Remark:
		default:
			return fmt.Errorf("protocompile/report: invalid value for Diagnostic.level: %d", int(level))
		}

		d := Diagnostic{
			Err:    errors.New(dProto.Message),
			Level:  level,
			InFile: dProto.InFile,
			Notes:  dProto.Notes,
			Help:   dProto.Help,
			Debug:  dProto.Debug,
		}

		var havePrimary bool
		for j, snip := range dProto.Annotations {
			if int(snip.File) >= len(proto.Files) {
				return fmt.Errorf(
					"protocompile/report: invalid file index for diagnostic[%d].annotation[%d]: %d",
					i, j, snip.File,
				)
			}

			file := files[snip.File]
			if int(snip.Start) >= len(file.File().Text) ||
				int(snip.End) > len(file.File().Text) ||
				snip.Start > snip.End {
				return fmt.Errorf(
					"protocompile/report: out-of-bounds span for diagnostic[%d].annotation[%d]: [%d:%d]",
					i, j, snip.Start, snip.End,
				)
			}

			d.Annotations = append(d.Annotations, Annotation{
				File:    file.File(),
				Start:   file.Search(int(snip.Start)),
				End:     file.Search(int(snip.End)),
				Message: snip.Message,
				Primary: snip.Primary,
			})
			havePrimary = havePrimary || snip.Primary
		}

		if !havePrimary && len(d.Annotations) > 0 {
			d.Annotations[0].Primary = true
		}

		r.Diagnostics = append(r.Diagnostics, d)
	}

	return nil
}

// push is the core "make me a diagnostic" function.
//
//nolint:unparam
func (r *Report) push(skip int, err error, level Level) *Diagnostic {
	r.Diagnostics = append(r.Diagnostics, Diagnostic{
		Err:   err,
		Level: level,
		Stage: r.Stage,
	})
	d := &(r.Diagnostics)[len(r.Diagnostics)-1]

	// If debugging is on, capture a stack trace.
	if r.Tracing > 0 {
		// Unwind the stack to find program counter information.
		pc := make([]uintptr, 64)
		pc = pc[:runtime.Callers(skip+2, pc)]

		// Fill trace with the result.
		var (
			zero runtime.Frame
			buf  strings.Builder
		)
		frames := runtime.CallersFrames(pc)
		for i := 0; i < r.Tracing; i++ {
			frame, more := frames.Next()
			if frame == zero || !more {
				break
			}
			fmt.Fprintf(&buf, "at %s\n  %s:%d\n", frame.Function, frame.File, frame.Line)
		}
		d.With(Debug(buf.String()))
	}

	return d
}
