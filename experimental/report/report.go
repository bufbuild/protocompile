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
	"errors"
	"fmt"
	"runtime"
	runtimedebug "runtime/debug"
	"slices"
	"strings"

	"google.golang.org/protobuf/proto"

	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/internal/ext/cmpx"
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

	// If set, [Report.Sort] will not discard duplicate diagnostics, as defined
	// in that function's contract.
	KeepDuplicates bool

	// If set, all diagnostics of severity at most Warning (i.e., >= Warning
	// as integers) are suppressed.
	SuppressWarnings bool
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

// SoftError pushes a diagnostic with the onto this report, making it a warning
// if hard is false.
func (r *Report) SoftError(hard bool, err Diagnose) *Diagnostic {
	level := Warning
	if hard {
		level = Error
	}

	d := r.push(1, level)
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

// Level pushes a diagnostic with the given level onto this report.
func (r *Report) Level(level Level, err Diagnose) *Diagnostic {
	d := r.push(1, level)
	err.Diagnose(d)
	return d
}

// Fatalf creates an ad-hoc [ICE] diagnostic with the given message; analogous to
// [fmt.Errorf].
func (r *Report) Fatalf(format string, args ...any) *Diagnostic {
	return r.push(1, ICE).Apply(Message(format, args...))
}

// Errorf creates an ad-hoc error diagnostic with the given message; analogous to
// [fmt.Errorf].
func (r *Report) Errorf(format string, args ...any) *Diagnostic {
	return r.push(1, Error).Apply(Message(format, args...))
}

// SoftError pushes an ad-hoc soft error diagnostic with the given message; analogous to
// [fmt.Errorf].
func (r *Report) SoftErrorf(hard bool, format string, args ...any) *Diagnostic {
	level := Warning
	if hard {
		level = Error
	}
	return r.push(1, level).Apply(Message(format, args...))
}

// Warnf creates an ad-hoc warning diagnostic with the given message; analogous to
// [fmt.Errorf].
func (r *Report) Warnf(format string, args ...any) *Diagnostic {
	return r.push(1, Warning).Apply(Message(format, args...))
}

// Remarkf creates an ad-hoc remark diagnostic with the given message; analogous to
// [fmt.Errorf].
func (r *Report) Remarkf(format string, args ...any) *Diagnostic {
	return r.push(1, Remark).Apply(Message(format, args...))
}

// Levelf creates an ad-hoc diagnostic with the given level and message; analogous to
// [fmt.Errorf].
func (r *Report) Levelf(level Level, format string, args ...any) *Diagnostic {
	return r.push(1, level).Apply(Message(format, args...))
}

// SaveOptions calls the given function and, upon its completion, restores
// r.Options to the value it had before it was called.
func (r *Report) SaveOptions(body func()) {
	prev := r.Options
	body()
	r.Options = prev
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
	diagnostic := r.push(1, ICE).Apply(
		Message("unexpected panic; this is a bug"),
		Notef("%v", panicked),
	)
	r.Tracing = tracing

	if diagnose != nil {
		diagnose(diagnostic)
	}

	var ice *icePanic
	if err, _ := panicked.(error); errors.As(err, &ice) {
		diagnostic.Apply(ice.options...)
	}

	// Append a stack trace but only after any user-provided diagnostic
	// information.
	stack := strings.Split(strings.TrimSpace(string(runtimedebug.Stack())), "\n")
	// Remove the goroutine number and the first two frames (debug.Stack and
	// Report.CatchICE).
	stack = stack[5:]

	diagnostic.debug = append(diagnostic.debug, "", "stack trace:")
	diagnostic.debug = append(diagnostic.debug, stack...)

	if resume {
		panic(panicked)
	}
}

type icePanic struct {
	error
	options []DiagnosticOption
}

func (e *icePanic) Unwrap() error { return e.error }

// AnnotatePanic will recover a panic and annotate it such that when [CatchICE]
// recovers it, it can extract this information and display it in the
// diagnostic.
func (r *Report) AnnotateICE(options ...DiagnosticOption) {
	panicked := recover()
	if panicked == nil {
		return
	}

	err, _ := panicked.(error)
	if err == nil {
		err = fmt.Errorf("%v", err)
	}

	var ice *icePanic
	if errors.As(err, &ice) {
		ice.options = append(ice.options, options...)
	} else {
		ice = &icePanic{err, options}
	}

	panic(ice)
}

// Canonicalize sorts this report's diagnostics according to an specific
// ordering criteria. Diagnostics are sorted by, in order:
//
// 1. File name of primary span.
// 2. SortOrder value.
// 3. Start offset of primary snippet.
// 4. End offset of primary snippet.
// 5. Diagnostic tag.
// 6. Textual content of error message.
//
// Where diagnostics have no primary span, the file is treated as empty and the
// offsets are treated as zero.
//
// These criteria ensure that diagnostics for the same file go together,
// diagnostics for the same sort order (lex, parse, etc) go together, and they
// are otherwise ordered by where they occur in the file.
//
// Canonicalize will deduplicate diagnostics whose primary span and (nonempty)
// diagnostic tags are equal, selecting the diagnostic that sorts as greatest
// as the canonical value. This allows later diagnostics to replace earlier
// diagnostics, so long as they cooperate by using the same tag. Deduplication
// can be suppressed using [Options].KeepDuplicates.
func (r *Report) Canonicalize() {
	slices.SortFunc(r.Diagnostics, cmpx.Join(
		cmpx.Key(func(d Diagnostic) string { return d.Primary().Path() }),
		cmpx.Key(func(d Diagnostic) int { return d.sortOrder }),
		cmpx.Key(func(d Diagnostic) int { return d.Primary().Start }),
		cmpx.Key(func(d Diagnostic) int { return d.Primary().End }),
		cmpx.Key(func(d Diagnostic) string { return d.tag }),
		cmpx.Key(func(d Diagnostic) string { return d.message }),
	))

	if r.KeepDuplicates {
		return
	}

	type key struct {
		span source.Span
		tag  string
	}
	var cur key
	slices.Backward(r.Diagnostics)(func(i int, d Diagnostic) bool {
		if d.tag == "" {
			return true
		}

		key := key{d.Primary().Span(), d.tag}
		if cur.tag != "" && cur == key {
			r.Diagnostics[i].level = -1 // Use this to mark which diagnostics to delete.
		} else {
			cur = key
		}

		return true
	})

	r.Diagnostics = slices.DeleteFunc(r.Diagnostics, func(d Diagnostic) bool { return d.level == -1 })
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

			snippet := &compilerpb.Diagnostic_Annotation{
				File:      file,
				Start:     uint32(snip.Start),
				End:       uint32(snip.End),
				Message:   snip.message,
				Primary:   snip.primary,
				PageBreak: snip.pageBreak,
			}
			for _, edit := range snip.edits {
				snippet.Edits = append(snippet.Edits, &compilerpb.Diagnostic_Edit{
					Start:   uint32(edit.Start),
					End:     uint32(edit.End),
					Replace: edit.Replace,
				})
			}

			dProto.Annotations = append(dProto.Annotations, snippet)
		}

		proto.Diagnostics = append(proto.Diagnostics, dProto)
	}

	return proto
}

// AppendFromProto appends diagnostics from a Protobuf message to this report.
//
// deserialize will be called with an empty message that should be deserialized
// onto, which this function will then convert into [Diagnostic]s to populate the
// report with.
func (r *Report) AppendFromProto(deserialize func(proto.Message) error) error {
	proto := new(compilerpb.Report)
	if err := deserialize(proto); err != nil {
		return err
	}

	files := make([]*source.File, len(proto.Files))
	for i, fProto := range proto.Files {
		files[i] = source.NewFile(fProto.Path, string(fProto.Text))
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

			snippet := snippet{
				Span: source.Span{
					File:  file,
					Start: int(snip.Start),
					End:   int(snip.End),
				},
				message:   snip.Message,
				primary:   snip.Primary,
				pageBreak: snip.PageBreak,
			}
			for _, edit := range snip.Edits {
				snippet.edits = append(snippet.edits, Edit{
					Start:   int(edit.Start),
					End:     int(edit.End),
					Replace: edit.Replace,
				})
			}

			d.snippets = append(d.snippets, snippet)
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

	if level >= Warning && r.SuppressWarnings {
		return &Diagnostic{}
	}

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
		for range r.Tracing {
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
