// Copyright 2020-2022 Buf Technologies, Inc.
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

package sourceinfo_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/bufbuild/protocompile"
	"github.com/bufbuild/protocompile/linker"
)

// If true, re-generates the golden output file.
const regenerateMode = false

func TestSourceCodeInfo(t *testing.T) {
	compiler := protocompile.Compiler{
		Resolver: protocompile.WithStandardImports(&protocompile.SourceResolver{
			ImportPaths: []string{"../internal/testprotos"},
		}),
		IncludeSourceInfo: true,
	}
	fds, err := compiler.Compile(context.Background(), "desc_test_comments.proto", "desc_test_complex.proto")
	if !assert.Nil(t, err) {
		return
	}
	// also test that imported files have source code info
	// (desc_test_comments.proto imports desc_test_options.proto)
	importedFd := fds[0].FindImportByPath("desc_test_options.proto")
	if !assert.NotNil(t, importedFd) {
		return
	}

	// create description of source code info
	// (human readable so diffs in source control are comprehensible)
	var buf bytes.Buffer
	for _, fd := range fds {
		printSourceCodeInfo(fd, &buf)
	}
	printSourceCodeInfo(importedFd, &buf)
	actual := buf.String()

	if regenerateMode {
		// re-generate the file
		err = os.WriteFile("test-source-info.txt", buf.Bytes(), 0666)
		if !assert.Nil(t, err) {
			return
		}
	}

	b, err := os.ReadFile("test-source-info.txt")
	if !assert.Nil(t, err) {
		return
	}
	golden := string(b)

	assert.Equal(t, golden, actual, "wrong source code info")
}

// NB: this function can be used to manually inspect the source code info for a
// descriptor, in a manner that is much easier to read and check than raw
// descriptor form.
func printSourceCodeInfo(fd linker.File, out io.Writer) {
	fmt.Fprintf(out, "---- %s ----\n", fd.Path())

	var fdMsg *descriptorpb.FileDescriptorProto
	if r, ok := fd.(linker.Result); ok {
		fdMsg = r.Proto()
	} else {
		fdMsg = protodesc.ToFileDescriptorProto(fd)
	}

	for i := 0; i < fd.SourceLocations().Len(); i++ {
		loc := fd.SourceLocations().Get(i)
		var buf bytes.Buffer
		findLocation(linker.ResolverFromFile(fd), fdMsg.ProtoReflect(), fdMsg.ProtoReflect().Descriptor(), loc.Path, &buf)
		fmt.Fprintf(out, "\n\n%s:\n", buf.String())
		fmt.Fprintf(out, "%s:%d:%d\n", fd.Path(), loc.StartLine+1, loc.StartColumn+1)
		fmt.Fprintf(out, "%s:%d:%d\n", fd.Path(), loc.EndLine+1, loc.EndColumn+1)
		if len(loc.LeadingDetachedComments) > 0 {
			for i, comment := range loc.LeadingDetachedComments {
				fmt.Fprintf(out, "    Leading detached comment [%d]:\n%s\n", i, comment)
			}
		}
		if loc.LeadingComments != "" {
			fmt.Fprintf(out, "    Leading comments:\n%s\n", loc.LeadingComments)
		}
		if loc.TrailingComments != "" {
			fmt.Fprintf(out, "    Trailing comments:\n%s\n", loc.TrailingComments)
		}
	}
}

func findLocation(res protoregistry.ExtensionTypeResolver, msg protoreflect.Message, md protoreflect.MessageDescriptor, path []int32, buf *bytes.Buffer) {
	if len(path) == 0 {
		return
	}

	tag := protoreflect.FieldNumber(path[0])
	fld := md.Fields().ByNumber(tag)
	if fld == nil {
		ext, err := res.FindExtensionByNumber(md.FullName(), tag)
		if err != nil {
			panic(fmt.Sprintf("could not find field with tag %d in message of type %s", path[0], msg.Descriptor().FullName()))
		}
		fld = ext.TypeDescriptor()
	}

	fmt.Fprintf(buf, " > %s", fld.Name())
	path = path[1:]
	idx := -1
	if fld.Cardinality() == protoreflect.Repeated && len(path) > 0 {
		idx = int(path[0])
		fmt.Fprintf(buf, "[%d]", path[0])
		path = path[1:]
	}

	if len(path) > 0 {
		var next protoreflect.Message
		if msg != nil {
			fldVal := msg.Get(fld)
			if idx >= 0 {
				l := fldVal.List()
				if idx < l.Len() {
					next = l.Get(idx).Message()
				}
			} else {
				next = fldVal.Message()
			}
		}

		if next == nil && msg != nil {
			buf.WriteString(" !!! ")
		}

		findLocation(res, next, fld.Message(), path, buf)
	}
}
