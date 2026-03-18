// Package sourceinfo contains decoding operations for Protobuf descriptor
// SourceCodeInfo paths.
//
// The resulting output is intended primarily for diagnostics.
package sourceinfo

import (
	"fmt"
	"strings"

	"github.com/bufbuild/protocompile/internal/ext/slicesx"
	"github.com/bufbuild/protocompile/internal/ext/stringsx"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

// Location is a decoded location within a SourceCodeInfo message.
type Location struct {
	Path       Path
	Start, End Position

	Leading, Trailing string
	Detached          []string
}

// Decode decodes all of the source locations within a FileDescriptorProto.
func Decode(fdp *descriptorpb.FileDescriptorProto) []Location {
	if fdp.SourceCodeInfo == nil {
		return nil
	}

	return slicesx.Transform(
		fdp.SourceCodeInfo.Location,
		func(scil *descriptorpb.SourceCodeInfo_Location) Location {
			return DecodeLocation(scil, fdp)
		},
	)
}

// DecodeLocation decodes a SourceCodeInfo location.
//
// If fdp is not nil, it is used for obtaining the names of entities within the
// path.
func DecodeLocation(scil *descriptorpb.SourceCodeInfo_Location, fdp *descriptorpb.FileDescriptorProto) Location {
	loc := Location{Path: DecodePath(scil.Path, fdp)}
	switch len(scil.Span) {
	case 0:
	case 1:
		loc.Start.Line = int(scil.Span[0])
		loc.End.Line = int(scil.Span[0])
	case 2:
		loc.Start.Line = int(scil.Span[0])
		loc.End.Line = int(scil.Span[0])
		loc.Start.Column = int(scil.Span[1])
		loc.End.Column = int(scil.Span[1])
	case 3:
		loc.Start.Line = int(scil.Span[0])
		loc.End.Line = int(scil.Span[0])
		loc.Start.Column = int(scil.Span[1])
		loc.End.Column = int(scil.Span[2])
	default:
		loc.Start.Line = int(scil.Span[0])
		loc.End.Line = int(scil.Span[2])
		loc.Start.Column = int(scil.Span[1])
		loc.End.Column = int(scil.Span[3])
	}

	loc.Leading = scil.GetLeadingComments()
	loc.Trailing = scil.GetTrailingComments()
	loc.Detached = scil.LeadingDetachedComments

	return loc
}

// Position is a line/column-based position within a file.
//
// SourceCodeInfo only provides line/column information, and not byte offset
// information. This package does not attempt to reconstruct this information,
// because the source code of the file itself is not available.
type Position struct {
	Line, Column int
}

// Path is a SourceCodeInfo path.
type Path []Component

// Component is a piece of a [Path].
//
// A Component corresponds to a field of a message within a descriptor, rooted
// at a FileDescriptorProto.
type Component struct {
	Number int32  // The field number od the message.
	Name   string // The name of the field. Empty if not known.

	Repeated bool // Whether this is a repeated field.
	Index    int  // If this is a repeated, the index within it.

	// For fields which correspond to an entity with a name, such
	// FileDescriptorProto.message or DescriptorProto.field, this is that
	// entity's name.
	IndexName string
}

// Decode decodes a SourceCodeInfo path.
//
// If fdp is not nil, it is used for obtaining the names of entities within the
// path.
func DecodePath(path []int32, fdp *descriptorpb.FileDescriptorProto) Path {
	var out Path

	msg := fdp.ProtoReflect()
	ty := (*descriptorpb.FileDescriptorProto)(nil).ProtoReflect().Descriptor()
	for i := 0; i < len(path); i++ {
		out = append(out, Component{Number: path[i]})
		if ty == nil {
			continue
		}

		c := slicesx.LastPointer(out)
		field := ty.Fields().ByNumber(protoreflect.FieldNumber(c.Number))
		if field == nil {
			msg = nil
			continue
		}

		c.Name = string(field.Name())
		ty = field.Message()
		if ty == nil {
			msg = nil
			continue
		}

		if field.Cardinality() != protoreflect.Repeated {
			msg = msg.Get(field).Message()
			continue
		}

		if i == len(path)-1 {
			break
		}

		i++
		c.Repeated = true
		c.Index = int(path[i])
		if msg != nil {
			msg = msg.Get(field).List().Get(int(c.Index)).Message()

			name := msg.Descriptor().Fields().ByName("name")
			if name == nil || name.Kind() != protoreflect.StringKind {
				continue
			}

			_, c.IndexName, _ = stringsx.CutLast(msg.Get(name).String(), ".")
		}
	}

	return out
}

// String implements [fmt.Stringer].
func (p Path) String() string {
	buf := new(strings.Builder)
	for _, c := range p {
		buf.WriteByte('.')

		if c.Name != "" {
			buf.WriteString(c.Name)
		} else {
			fmt.Fprintf(buf, "%v", c.Index)
		}

		if c.Repeated {
			if c.IndexName != "" {
				fmt.Fprintf(buf, "[%v]", c.IndexName)
			} else {
				fmt.Fprintf(buf, "[%v]", c.Index)
			}
		}
	}

	return buf.String()
}
