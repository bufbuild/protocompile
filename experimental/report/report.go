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
	"runtime"
	runtimedebug "runtime/debug"
	"slices"
	"strings"

	"google.golang.org/protobuf/proto"

	compilerpb "github.com/bufbuild/protocompile/internal/gen/buf/compiler/v1alpha1"
)

// Report is a collection of diagnostics.
//
// Report is not thread-safe (in the sense that distinct goroutines should not
// all write to Report at the same time). Instead, the recommendation is to create
// multiple reports and then merge them, using [Report.Sort] to canonicalize the result.
type Report struct {
	Options

	// The actual diagnostics on this report. Generally, you'll want to use one of
	// the helpers like [Report.Error] instead of appending directly.
	Diagnostics []Diagnostic
}

// Options for how a report should be constructed.
type Options struct {
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

// Diagnose is a type that can be rendered as a diagnostic.
type Diagnose interface {
	Diagnose(*Diagnostic)
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
	return r.push(1, Error).Apply(Message(format, args...))
}

// Warnf creates an ad-hoc warning diagnostic with the given message; analogous to
// [fmt.Errorf].
func (r *Report) Warnf(format string, args ...any) *Diagnostic {
	return r.push(1, Warning).Apply(Message(format, args...))
}

// Remarkf creates an ad-hoc remark diagnostic with an the given message; analogous to
// [fmt.Errorf].
func (r *Report) Remarkf(format string, args ...any) *Diagnostic {
	return r.push(1, Remark).Apply(Message(format, args...))
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
	diagnostic := r.push(1, Error).Apply(Message("%v", panicked))
	r.Tracing = tracing
	diagnostic.level = ICE

	if diagnose != nil {
		diagnose(diagnostic)
	}

	// Append a stack trace but only after any user-provided diagnostic
	// information.
	stack := strings.Split(strings.TrimSpace(string(runtimedebug.Stack())), "\n")
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
			Tag:     d.tag,
			Level:   compilerpb.Diagnostic_Level(d.level),
			InFile:  d.inFile,
			Notes:   d.notes,
			Help:    d.help,
			Debug:   d.debug,
		}

		for _, snip := range d.snippets {
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
				Message: snip.message,
				Primary: snip.primary,
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
			tag:     dProto.Tag,
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

			d.snippets = append(d.snippets, snippet{
				Span: Span{
					File:  file,
					Start: int(snip.Start),
					End:   int(snip.End),
				},
				message: snip.Message,
				primary: snip.Primary,
			})
			havePrimary = havePrimary || snip.Primary
		}

		if !havePrimary && len(d.snippets) > 0 {
			d.snippets[0].primary = true
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
		d.Apply(Debugf("%s", buf.String()))
	}

	return d
}
