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

package fdp

import (
	"slices"

	descriptorv1 "buf.build/gen/go/bufbuild/protodescriptor/protocolbuffers/go/buf/descriptor/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/bufbuild/protocompile/experimental/ir"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/internal/ext/iterx"
)

// debug is a helper for building SourceCodeInfo.
type debug struct {
	file *ir.File

	path  []int32
	proto *descriptorpb.SourceCodeInfo
	extns *descriptorv1.SourceCodeInfoExtension

	commentTracker *commentTracker

	suppressed bool
}

// init initializes this debug for the given file.
func (d *debug) init(file *ir.File) {
	if d == nil {
		return
	}

	d.file = file

	d.proto = new(descriptorpb.SourceCodeInfo)
	d.extns = new(descriptorv1.SourceCodeInfoExtension)
	proto.SetExtension(
		d.proto,
		descriptorv1.E_BufSourceCodeInfoExtension, d.extns)

	d.commentTracker = new(commentTracker)
	d.commentTracker.attributeComments(file.AST().Stream().Cursor())
}

// in pushes a new path component for the duration of the function passed to
// the returned function.
func (d *debug) in(path ...int32) func(func()) {
	return func(body func()) {
		if d == nil {
			body()
			return
		}

		n := len(d.path)
		d.path = append(d.path, path...)
		body()
		d.path = d.path[:n]
	}
}

// span is like [debug.maybeComments] but it records no comments.
func (d *debug) span(s source.Spanner, path ...int32) {
	d.maybeComments(s, false, path...)
}

// comments is like [debug.maybeComments] but it always records comments.
func (d *debug) comments(s source.Spanner, path ...int32) {
	d.maybeComments(s, true, path...)
}

// maybeComments adds the source code info location based on the current path.
//
// If checking for comments, it looks at the first token of the given span for leading
// and detached leading comments, and the last token of the span for trailing comments.
//
// If the last token is fused token, for example, the closing brace of a body, }, based
// on protoc comment attribution semantics, it checks the opening brace for trailing comments.
func (d *debug) maybeComments(s source.Spanner, comments bool, path ...int32) {
	if d == nil || d.suppressed {
		return
	}

	span := source.GetSpan(s)
	if span.IsZero() {
		return
	}

	loc := new(descriptorpb.SourceCodeInfo_Location)
	d.proto.Location = append(d.proto.Location, loc)

	loc.Span = locationSpan(span)
	loc.Path = append(slices.Clone(d.path), path...)
	if !comments {
		return
	}

	stream := d.file.AST().Stream()

	_, start := stream.Around(span.Start)
	leading := d.commentTracker.attributed[start.ID()]
	if leading != nil {
		if leadingComment := leading.leadingComment(); leadingComment != "" {
			loc.LeadingComments = addr(leadingComment)
		}
		if detachedComments := leading.detachedComments(); len(detachedComments) > 0 {
			loc.LeadingDetachedComments = detachedComments
		}
	}

	end, _ := stream.Around(span.End)
	// Check the start of a fused token.
	end, _ = end.StartEnd()
	trailing := d.commentTracker.attributed[end.ID()]
	if trailing != nil {
		if trailingComment := trailing.trailingComment(); trailingComment != "" {
			loc.TrailingComments = addr(trailingComment)
		}
	}
}

// extensions calls do with the Buf-specific SourceCodeInfo extensions.
func (d *debug) extensions(do func(*descriptorv1.SourceCodeInfoExtension)) {
	if d != nil && !d.suppressed {
		do(d.extns)
	}
}

// done writes the result of recording SourceCodeInfo to out and resets this
// builder.
func (d *debug) done(out **descriptorpb.SourceCodeInfo) {
	if d == nil {
		return
	}

	if iterx.Empty2(d.extns.ProtoReflect().Range) {
		proto.ClearExtension(d.proto, descriptorv1.E_BufSourceCodeInfoExtension)
	}

	slices.SortStableFunc(d.proto.Location, func(a, b *descriptorpb.SourceCodeInfo_Location) int {
		return slices.Compare(a.Span, b.Span)
	})
	d.proto.Location = slices.Insert(d.proto.Location, 0, &descriptorpb.SourceCodeInfo_Location{
		Span: locationSpan(d.file.AST().Span()),
	})

	*out = d.proto
	*d = debug{}
}

// suppress disables recording of debug information until the returned function
// is called. Use like this:
//
//	defer d.suppress()()
func (d *debug) suppress() func() {
	if d == nil {
		return func() {}
	}
	prev := d.suppressed
	d.suppressed = true
	return func() { d.suppressed = prev }
}

// locationSpan is a helper function for returning the [descriptorpb.SourceCodeInfo_Location]
// span for the given [source.Span].
//
// The span for [descriptorpb.SourceCodeInfo_Location] always has exactly three or four:
// start line, start column, end line (optional, otherwise assumed same as start line),
// and end column. The line and column numbers are zero-based.
func locationSpan(span source.Span) []int32 {
	start, end := span.StartLoc(), span.EndLoc()
	if start.Line == end.Line {
		return []int32{
			int32(start.Line) - 1,
			int32(start.Column) - 1,
			int32(end.Column) - 1,
		}
	}
	return []int32{
		int32(start.Line) - 1,
		int32(start.Column) - 1,
		int32(end.Line) - 1,
		int32(end.Column) - 1,
	}
}
