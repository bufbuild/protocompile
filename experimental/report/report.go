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
	"runtime/debug"
	"slices"
	"strings"

	"google.golang.org/protobuf/proto"

	"github.com/bufbuild/protocompile/experimental/internal"
	compilerpb "github.com/bufbuild/protocompile/internal/gen/buf/compiler/v1alpha1"
)

const (
	// Internal compiler error. Indicates a panic within the compiler.
	ICE Level = 1 + iota
	// Red. Indicates a semantic constraint violation.
	Error
	// Yellow. Indicates something that probably should not be ignored.
	Warning
	// Cyan. This is the diagnostics version of "info".
	Remark

	note // Used internally within the diagnostic renderer.
)

// Level represents the severity of a diagnostic message.
type Level int8

// Tag is a diagnostic tag: a machine-readable identification for a diagnostic.
//
// Tags should be lowercase identifiers separated by dashes, e.g. my-error-tag.
// If a package generates diagnostics with tags, it should expose those tags as
// constants.
type Tag string

// Option returns a DiagnosticOption that sets a diagnostic's tag to this one.
func (t Tag) Option() DiagnosticOption {
	return func(d *Diagnostic) {
		d.tag = t
	}
}

// Diagnostic is a type of error that can be rendered as a rich diagnostic.
//
// Not all Diagnostics are "errors", even though Diagnostic does embed error;
// some represent warnings, or perhaps debugging remarks.
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
	Message string

	// Whether this is a "primary" snippet, which is used for deciding whether or not
	// to mark the snippet with the same color as the overall diagnostic.
	Primary bool
}

// Primary returns this diagnostic's primary span, if it has one.
//
// If it doesn't have one, it returns the zero span.
func (d *Diagnostic) Primary() Span {
	for _, annotation := range d.annotations {
		if annotation.Primary {
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
			option(d)
		}
	}
	return d
}

// Diagnose is a type that can be rendered as a diagnostic.
type Diagnose interface {
	Diagnose(*Diagnostic)
}

// DiagnosticOption is an option that can be applied to a [Diagnostic].
//
// Nil values passed to [Diagnostic.With] are ignored.
type DiagnosticOption func(*Diagnostic)

// Message returns a Diagnostic option that sets the main diagnostic message.
func Message(format string, args ...any) DiagnosticOption {
	return func(d *Diagnostic) {
		if d.message != "" {
			panic("protocompile/report: set diagnostic message more than once")
		}

		d.message = fmt.Sprintf(format, args...)
	}
}

// InFile returns a DiagnosticOption that causes a diagnostic without a primary
// span to mention the given file.
func InFile(path string) DiagnosticOption {
	return func(d *Diagnostic) {
		if d.inFile != "" {
			panic("protocompile/report: set diagnostic path more than once")
		}

		d.inFile = path
	}
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

	// This is hoisted out to improve stack traces when something goes awry in
	// the argument to With(). By hoisting, it correctly blames the right
	// invocation to Snippet().
	annotation := annotation{Span: span}
	if len(args) > 0 {
		format, ok := args[0].(string)
		if !ok {
			panic("protocompile/report: expected string as first Snippet argument")
		}

		annotation.Message = fmt.Sprintf(format, args[1:]...)
	}

	return func(d *Diagnostic) {
		annotation.Primary = len(d.annotations) == 0
		d.annotations = append(d.annotations, annotation)
	}
}

// Note returns a DiagnosticOption that provides the user with context about the
// diagnostic, after the annotations.
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

// Debug returns a DiagnosticOption appends debugging information to a diagnostic that
// is not intended to be shown to normal users.
func Debug(format string, args ...any) DiagnosticOption {
	return func(d *Diagnostic) {
		d.debug = append(d.debug, fmt.Sprintf(format, args...))
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
func (r *Report) Error(err Diagnose) *Diagnostic {
	d := r.push(1, Error)
	err.Diagnose(d)
	return d
}

// Warn pushes a warning diagnostic onto this report.
func (r *Report) Warn(err Diagnose) *Diagnostic {
	d := r.push(1, Warning)
	err.Diagnose(d)
	return d
}

// Remark pushes a remark diagnostic onto this report.
func (r *Report) Remark(err Diagnose) *Diagnostic {
	d := r.push(1, Remark)
	err.Diagnose(d)
	return d
}

// Errorf creates an ad-hoc error diagnostic with the given message; analogous to
// [fmt.Errorf].
func (r *Report) Errorf(format string, args ...any) *Diagnostic {
	return r.push(1, Error).With(Message(format, args...))
}

// Warnf creates an ad-hoc warning diagnostic with the given message; analogous to
// [fmt.Errorf].
func (r *Report) Warnf(format string, args ...any) *Diagnostic {
	return r.push(1, Warning).With(Message(format, args...))
}

// Remarkf creates an ad-hoc remark diagnostic with an the given message; analogous to
// [fmt.Errorf].
func (r *Report) Remarkf(format string, args ...any) *Diagnostic {
	return r.push(1, Remark).With(Message(format, args...))
}

// CatchICE will recover a panic (an internal compiler error, or ICE) and log it
// as an error diagnostic. This function should be called in a defer statement.
//
// When constructing the diagnostic, diagnose is called, to provide an
// opportunity to annotate further.
//
// If resume is true, resumes the recovered panic.
func (r *Report) CatchICE(resume bool, diagnose func(*Diagnostic)) {
	panicked := recover()
	if panicked == nil {
		return
	}

	// Instead of using the built-in tracing function, which causes the stack
	// trace to be hidden by default, use debug.Stack and convert it into notes
	// so that it is always visible.
	tracing := r.Tracing
	r.Tracing = 0 // Temporarily disable built-in tracing.
	diagnostic := r.push(1, Error).With(Message("%v", panicked))
	r.Tracing = tracing
	diagnostic.level = ICE

	if diagnose != nil {
		diagnose(diagnostic)
	}

	// Append a stack trace but only after any user-provided diagnostic
	// information.
	stack := strings.Split(strings.TrimSpace(string(debug.Stack())), "\n")
	// Remove the goroutine number and the first two frames (debug.Stack and
	// Report.CatchICE).
	stack = stack[5:]

	diagnostic.notes = append(diagnostic.notes, "", "stack trace:")
	diagnostic.notes = append(diagnostic.notes, stack...)

	if resume {
		panic(panicked)
	}
}

// Sort canonicalizes this report's diagnostic order according to an specific
// ordering criteria. Diagnostics are sorted by, in order;
//
// File name of primary span, SortOrder value, start offset of primary snippet,
// end offset of primary snippet, content of error message.
//
// Where diagnostics have no primary span, the file is treated as empty and the
// offsets are treated as zero.
//
// These criteria ensure that diagnostics for the same file go together,
// diagnostics for the same sort order (lex, parse, etc) go together, and they
// are otherwise ordered by where they occur in the file.
func (r *Report) Sort() {
	slices.SortFunc(r.Diagnostics, func(a, b Diagnostic) int {
		aPrime := a.Primary()
		bPrime := b.Primary()

		if diff := strings.Compare(aPrime.Path(), bPrime.Path()); diff != 0 {
			return diff
		}

		if diff := a.sortOrder - b.sortOrder; diff != 0 {
			return diff
		}

		if diff := aPrime.Start - bPrime.Start; diff != 0 {
			return diff
		}

		if diff := aPrime.End - bPrime.End; diff != 0 {
			return diff
		}

		return strings.Compare(a.message, b.message)
	})
}

// ToProto converts this report into a Protobuf message for serialization.
//
// This operation is lossy: only the Diagnostics slice is serialized. It also discards
// concrete types of Diagnostic.Err, replacing them with opaque [errors.New] values
// on deserialization.
//
// It will also deduplicate [File2] values based on their paths, paying no attention to
// their contents.
func (r *Report) ToProto() proto.Message {
	proto := new(compilerpb.Report)

	fileToIndex := map[string]uint32{}
	for _, d := range r.Diagnostics {
		dProto := &compilerpb.Diagnostic{
			Message: d.message,
			Tag:     string(d.tag),
			Level:   compilerpb.Diagnostic_Level(d.level),
			InFile:  d.inFile,
			Notes:   d.notes,
			Help:    d.help,
			Debug:   d.debug,
		}

		for _, snip := range d.annotations {
			file, ok := fileToIndex[snip.Path()]
			if !ok {
				file = uint32(len(proto.Files))
				fileToIndex[snip.Path()] = file

				proto.Files = append(proto.Files, &compilerpb.Report_File{
					Path: snip.Path(),
					Text: []byte(snip.Text()),
				})
			}

			dProto.Annotations = append(dProto.Annotations, &compilerpb.Diagnostic_Annotation{
				File:    file,
				Start:   uint32(snip.Start),
				End:     uint32(snip.End),
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

	files := make([]*File, len(proto.Files))
	for i, fProto := range proto.Files {
		files[i] = NewFile(fProto.Path, string(fProto.Text))
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
			tag:     Tag(dProto.Tag),
			message: dProto.Message,
			level:   level,
			inFile:  dProto.InFile,
			notes:   dProto.Notes,
			help:    dProto.Help,
			debug:   dProto.Debug,
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
			if int(snip.Start) >= len(file.Text()) ||
				int(snip.End) > len(file.Text()) ||
				snip.Start > snip.End {
				return fmt.Errorf(
					"protocompile/report: out-of-bounds span for diagnostic[%d].annotation[%d]: [%d:%d]",
					i, j, snip.Start, snip.End,
				)
			}

			d.annotations = append(d.annotations, annotation{
				Span: Span{
					File:  file,
					Start: int(snip.Start),
					End:   int(snip.End),
				},
				Message: snip.Message,
				Primary: snip.Primary,
			})
			havePrimary = havePrimary || snip.Primary
		}

		if !havePrimary && len(d.annotations) > 0 {
			d.annotations[0].Primary = true
		}

		r.Diagnostics = append(r.Diagnostics, d)
	}

	return nil
}

// push is the core "make me a diagnostic" function.
//
//nolint:unparam  // For skip, see the comment below.
func (r *Report) push(skip int, level Level) *Diagnostic {
	// The linter does not like that skip is statically a constant.
	// We provide it as an argument for documentation purposes, so
	// that callers of this function within this package can specify
	// can specify how deeply-nested they are, even if they all have
	// the same level of nesting right now.

	r.Diagnostics = append(r.Diagnostics, Diagnostic{
		level:     level,
		sortOrder: r.Stage,
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
		d.With(Debug("%s", buf.String()))
	}

	return d
}

// AsError wraps a [Report] as an [error].
type AsError struct {
	Report Report
}

// Error implements [error].
func (e *AsError) Error() string {
	text, _, _ := Renderer{Compact: true}.RenderString(&e.Report)
	return text
}

// ErrInFile wraps an [error] into a diagnostic on the given file.
type ErrInFile struct {
	Err  error
	Path string
}

var _ Diagnose = &ErrInFile{}

// Error implements [error].
func (e *ErrInFile) Error() string {
	return e.Err.Error()
}

// Diagnose implements [Diagnose].
func (e *ErrInFile) Diagnose(d *Diagnostic) {
	d.inFile = e.Path
}
