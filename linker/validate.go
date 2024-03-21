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

package linker

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/bufbuild/protocompile/internal"
	"github.com/bufbuild/protocompile/reporter"
	"github.com/bufbuild/protocompile/walk"
)

// ValidateOptions runs some validation checks on the result that can only
// be done after options are interpreted.
func (r *result) ValidateOptions(handler *reporter.Handler) error {
	return walk.Descriptors(r, func(d protoreflect.Descriptor) error {
		switch d := d.(type) {
		case protoreflect.FieldDescriptor:
			if err := r.validateField(d, handler); err != nil {
				return err
			}
		case protoreflect.MessageDescriptor:
			if err := r.validateMessage(d, handler); err != nil {
				return err
			}
		case protoreflect.EnumDescriptor:
			if err := r.validateEnum(d, handler); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *result) validateField(fld protoreflect.FieldDescriptor, handler *reporter.Handler) error {
	if xtd, ok := fld.(protoreflect.ExtensionTypeDescriptor); ok {
		fld = xtd.Descriptor()
	}
	fd, ok := fld.(*fldDescriptor)
	if !ok {
		// should not be possible
		return fmt.Errorf("field descriptor is wrong type: expecting %T, got %T", (*fldDescriptor)(nil), fld)
	}
	if fld.IsExtension() {
		if err := r.validateExtension(fd, handler); err != nil {
			return err
		}
	}
	if err := r.validatePacked(fd, handler); err != nil {
		return err
	}
	if fd.Kind() == protoreflect.EnumKind {
		requiresOpen := !fd.IsList() && !fd.HasPresence()
		if requiresOpen && fd.Enum().IsClosed() {
			// Fields in a proto3 message cannot refer to proto2 enums.
			// In editions, this translates to implicit presence fields
			// not being able to refer to closed enums.
			// TODO: This really should be based solely on whether the enum's first
			//       value is zero, NOT based on if it's open vs closed.
			//       https://github.com/protocolbuffers/protobuf/issues/16249
			file := r.FileNode()
			info := file.NodeInfo(r.FieldNode(fd.proto).FieldType())
			if err := handler.HandleErrorf(info, "cannot use closed enum %s in a field with implicit presence", fd.Enum().FullName()); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *result) validateExtension(fd *fldDescriptor, handler *reporter.Handler) error {
	// NB: It's a little gross that we don't enforce these in validateBasic().
	// But it requires linking to resolve the extendee, so we can interrogate
	// its descriptor.
	if fd.ContainingMessage().Options().(*descriptorpb.MessageOptions).GetMessageSetWireFormat() {
		// Message set wire format requires that all extensions be messages
		// themselves (no scalar extensions)
		if fd.Kind() != protoreflect.MessageKind {
			file := r.FileNode()
			info := file.NodeInfo(r.FieldNode(fd.proto).FieldType())
			return handler.HandleErrorf(info, "messages with message-set wire format cannot contain scalar extensions, only messages")
		}
		if fd.Cardinality() == protoreflect.Repeated {
			file := r.FileNode()
			info := file.NodeInfo(r.FieldNode(fd.proto).FieldLabel())
			return handler.HandleErrorf(info, "messages with message-set wire format cannot contain repeated extensions, only optional")
		}
	} else if fd.Number() > internal.MaxNormalTag {
		// In validateBasic() we just made sure these were within bounds for any message. But
		// now that things are linked, we can check if the extendee is messageset wire format
		// and, if not, enforce tighter limit.
		file := r.FileNode()
		info := file.NodeInfo(r.FieldNode(fd.proto).FieldTag())
		return handler.HandleErrorf(info, "tag number %d is higher than max allowed tag number (%d)", fd.Number(), internal.MaxNormalTag)
	}

	return nil
}

func (r *result) validatePacked(fd *fldDescriptor, handler *reporter.Handler) error {
	if !fd.proto.GetOptions().GetPacked() {
		// if packed isn't true, nothing to validate
		return nil
	}
	if fd.proto.GetLabel() != descriptorpb.FieldDescriptorProto_LABEL_REPEATED {
		file := r.FileNode()
		info := file.NodeInfo(r.FieldNode(fd.proto).FieldLabel())
		err := handler.HandleErrorf(info, "packed option is only allowed on repeated fields")
		if err != nil {
			return err
		}
	}
	switch fd.proto.GetType() {
	case descriptorpb.FieldDescriptorProto_TYPE_STRING, descriptorpb.FieldDescriptorProto_TYPE_BYTES,
		descriptorpb.FieldDescriptorProto_TYPE_MESSAGE, descriptorpb.FieldDescriptorProto_TYPE_GROUP:
		file := r.FileNode()
		info := file.NodeInfo(r.FieldNode(fd.proto).FieldType())
		return handler.HandleErrorf(info, "packed option is only allowed on numeric, boolean, and enum fields")
	}
	return nil
}

func (r *result) validateMessage(d protoreflect.MessageDescriptor, handler *reporter.Handler) error {
	md, ok := d.(*msgDescriptor)
	if !ok {
		// should not be possible
		return fmt.Errorf("message descriptor is wrong type: expecting %T, got %T", (*msgDescriptor)(nil), d)
	}

	if err := r.validateJSONNamesInMessage(md, handler); err != nil {
		return err
	}

	return nil
}

func (r *result) validateJSONNamesInMessage(md *msgDescriptor, handler *reporter.Handler) error {
	if err := r.validateFieldJSONNames(md, false, handler); err != nil {
		return err
	}
	if err := r.validateFieldJSONNames(md, true, handler); err != nil {
		return err
	}
	return nil
}

func (r *result) validateEnum(d protoreflect.EnumDescriptor, handler *reporter.Handler) error {
	ed, ok := d.(*enumDescriptor)
	if !ok {
		// should not be possible
		return fmt.Errorf("enum descriptor is wrong type: expecting %T, got %T", (*enumDescriptor)(nil), d)
	}

	firstValue := ed.Values().Get(0)
	if !ed.IsClosed() && firstValue.Number() != 0 {
		// TODO: This check doesn't really belong here. Whether the
		//       first value is zero s/b orthogonal to whether the
		//       allowed values are open or closed.
		//       https://github.com/protocolbuffers/protobuf/issues/16249
		file := r.FileNode()
		evd, ok := firstValue.(*enValDescriptor)
		if !ok {
			// should not be possible
			return fmt.Errorf("enum value descriptor is wrong type: expecting %T, got %T", (*enValDescriptor)(nil), firstValue)
		}
		info := file.NodeInfo(r.EnumValueNode(evd.proto).GetNumber())
		if err := handler.HandleErrorf(info, "first value of open enum %s must be zero", ed.FullName()); err != nil {
			return err
		}
	}

	if err := r.validateJSONNamesInEnum(ed, handler); err != nil {
		return err
	}

	return nil
}

func (r *result) validateJSONNamesInEnum(ed *enumDescriptor, handler *reporter.Handler) error {
	seen := map[string]*descriptorpb.EnumValueDescriptorProto{}
	for _, evd := range ed.proto.GetValue() {
		scope := "enum value " + ed.proto.GetName() + "." + evd.GetName()

		name := canonicalEnumValueName(evd.GetName(), ed.proto.GetName())
		if existing, ok := seen[name]; ok && evd.GetNumber() != existing.GetNumber() {
			fldNode := r.EnumValueNode(evd)
			existingNode := r.EnumValueNode(existing)
			conflictErr := fmt.Errorf("%s: camel-case name (with optional enum name prefix removed) %q conflicts with camel-case name of enum value %s, defined at %v",
				scope, name, existing.GetName(), r.FileNode().NodeInfo(existingNode).Start())

			// Since proto2 did not originally have a JSON format, we report conflicts as just warnings.
			// With editions, not fully supporting JSON is allowed via feature: json_format == BEST_EFFORT
			if !isJSONCompliant(ed) {
				handler.HandleWarningWithPos(r.FileNode().NodeInfo(fldNode), conflictErr)
			} else if err := handler.HandleErrorf(r.FileNode().NodeInfo(fldNode), conflictErr.Error()); err != nil {
				return err
			}
		} else {
			seen[name] = evd
		}
	}
	return nil
}

func (r *result) validateFieldJSONNames(md *msgDescriptor, useCustom bool, handler *reporter.Handler) error {
	type jsonName struct {
		source *descriptorpb.FieldDescriptorProto
		// true if orig is a custom JSON name (vs. the field's default JSON name)
		custom bool
	}
	seen := map[string]jsonName{}

	for _, fd := range md.proto.GetField() {
		scope := "field " + md.proto.GetName() + "." + fd.GetName()
		defaultName := internal.JSONName(fd.GetName())
		name := defaultName
		custom := false
		if useCustom {
			n := fd.GetJsonName()
			if n != defaultName || r.hasCustomJSONName(fd) {
				name = n
				custom = true
			}
		}
		if existing, ok := seen[name]; ok {
			// When useCustom is true, we'll only report an issue when a conflict is
			// due to a custom name. That way, we don't double report conflicts on
			// non-custom names.
			if !useCustom || custom || existing.custom {
				fldNode := r.FieldNode(fd)
				customStr, srcCustomStr := "custom", "custom"
				if !custom {
					customStr = "default"
				}
				if !existing.custom {
					srcCustomStr = "default"
				}
				info := r.FileNode().NodeInfo(fldNode)
				conflictErr := reporter.Errorf(info, "%s: %s JSON name %q conflicts with %s JSON name of field %s, defined at %v",
					scope, customStr, name, srcCustomStr, existing.source.GetName(), r.FileNode().NodeInfo(r.FieldNode(existing.source)).Start())

				// Since proto2 did not originally have default JSON names, we report conflicts
				// between default names (neither is a custom name) as just warnings.
				// With editions, not fully supporting JSON is allowed via feature: json_format == BEST_EFFORT
				if !isJSONCompliant(md) && !custom && !existing.custom {
					handler.HandleWarning(conflictErr)
				} else if err := handler.HandleError(conflictErr); err != nil {
					return err
				}
			}
		} else {
			seen[name] = jsonName{source: fd, custom: custom}
		}
	}
	return nil
}

func (r *result) hasCustomJSONName(fdProto *descriptorpb.FieldDescriptorProto) bool {
	// if we have the AST, we can more precisely determine if there was a custom
	// JSON named defined, even if it is explicitly configured to tbe the same
	// as the default JSON name for the field.
	opts := r.FieldNode(fdProto).GetOptions()
	if opts == nil {
		return false
	}
	for _, opt := range opts.Options {
		if len(opt.Name.Parts) == 1 &&
			opt.Name.Parts[0].Name.AsIdentifier() == "json_name" &&
			!opt.Name.Parts[0].IsExtension() {
			return true
		}
	}
	return false
}

func canonicalEnumValueName(enumValueName, enumName string) string {
	return enumValCamelCase(removePrefix(enumValueName, enumName))
}

// removePrefix is used to remove the given prefix from the given str. It does not require
// an exact match and ignores case and underscores. If the all non-underscore characters
// would be removed from str, str is returned unchanged. If str does not have the given
// prefix (even with the very lenient matching, in regard to case and underscores), then
// str is returned unchanged.
//
// The algorithm is adapted from the protoc source:
//
//	https://github.com/protocolbuffers/protobuf/blob/v21.3/src/google/protobuf/descriptor.cc#L922
func removePrefix(str, prefix string) string {
	j := 0
	for i, r := range str {
		if r == '_' {
			// skip underscores in the input
			continue
		}

		p, sz := utf8.DecodeRuneInString(prefix[j:])
		for p == '_' {
			j += sz // consume/skip underscore
			p, sz = utf8.DecodeRuneInString(prefix[j:])
		}

		if j == len(prefix) {
			// matched entire prefix; return rest of str
			// but skipping any leading underscores
			result := strings.TrimLeft(str[i:], "_")
			if len(result) == 0 {
				// result can't be empty string
				return str
			}
			return result
		}
		if unicode.ToLower(r) != unicode.ToLower(p) {
			// does not match prefix
			return str
		}
		j += sz // consume matched rune of prefix
	}
	return str
}

// enumValCamelCase converts the given string to upper-camel-case.
//
// The algorithm is adapted from the protoc source:
//
//	https://github.com/protocolbuffers/protobuf/blob/v21.3/src/google/protobuf/descriptor.cc#L887
func enumValCamelCase(name string) string {
	var js []rune
	nextUpper := true
	for _, r := range name {
		if r == '_' {
			nextUpper = true
			continue
		}
		if nextUpper {
			nextUpper = false
			js = append(js, unicode.ToUpper(r))
		} else {
			js = append(js, unicode.ToLower(r))
		}
	}
	return string(js)
}
