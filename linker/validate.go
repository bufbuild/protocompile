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

package linker

import (
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/bufbuild/protocompile/internal"
	"github.com/bufbuild/protocompile/reporter"
)

// ValidateOptions runs some validation checks on the result that can only
// be done after options are interpreted.
func (r *result) ValidateOptions(handler *reporter.Handler) error {
	return r.validateExtensions(r, handler)
}

func (r *result) validateExtensions(d hasExtensionsAndMessages, handler *reporter.Handler) error {
	for i := 0; i < d.Extensions().Len(); i++ {
		if err := r.validateExtension(d.Extensions().Get(i), handler); err != nil {
			return err
		}
	}
	for i := 0; i < d.Messages().Len(); i++ {
		if err := r.validateExtensions(d.Messages().Get(i), handler); err != nil {
			return err
		}
	}
	return nil
}

func (r *result) validateExtension(fld protoreflect.FieldDescriptor, handler *reporter.Handler) error {
	// NB: It's a little gross that we don't enforce these in validateBasic().
	// But it requires linking to resolve the extendee, so we can interrogate
	// its descriptor.
	if xtd, ok := fld.(protoreflect.ExtensionTypeDescriptor); ok {
		fld = xtd.Descriptor()
	}
	fd := fld.(*fldDescriptor)
	if fld.ContainingMessage().Options().(*descriptorpb.MessageOptions).GetMessageSetWireFormat() {
		// Message set wire format requires that all extensions be messages
		// themselves (no scalar extensions)
		if fld.Kind() != protoreflect.MessageKind {
			file := r.FileNode()
			pos := file.NodeInfo(r.FieldNode(fd.proto).FieldType()).Start()
			return handler.HandleErrorf(pos, "messages with message-set wire format cannot contain scalar extensions, only messages")
		}
		if fld.Cardinality() == protoreflect.Repeated {
			file := r.FileNode()
			pos := file.NodeInfo(r.FieldNode(fd.proto).FieldLabel()).Start()
			return handler.HandleErrorf(pos, "messages with message-set wire format cannot contain repeated extensions, only optional")
		}
	} else {
		// In validateBasic() we just made sure these were within bounds for any message. But
		// now that things are linked, we can check if the extendee is messageset wire format
		// and, if not, enforce tighter limit.
		if fld.Number() > internal.MaxNormalTag {
			file := r.FileNode()
			pos := file.NodeInfo(r.FieldNode(fd.proto).FieldTag()).Start()
			return handler.HandleErrorf(pos, "tag number %d is higher than max allowed tag number (%d)", fld.Number(), internal.MaxNormalTag)
		}
	}

	return nil
}
