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
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"

	"github.com/bufbuild/protocompile/ast"
	"github.com/bufbuild/protocompile/internal"
	"github.com/bufbuild/protocompile/reporter"
	"github.com/bufbuild/protocompile/walk"
)

func (r *result) ResolveMessageType(name protoreflect.FullName) protoreflect.MessageDescriptor {
	d := r.resolveElement(name)
	if md, ok := d.(protoreflect.MessageDescriptor); ok {
		return md
	}
	return nil
}

func (r *result) ResolveEnumType(name protoreflect.FullName) protoreflect.EnumDescriptor {
	d := r.resolveElement(name)
	if ed, ok := d.(protoreflect.EnumDescriptor); ok {
		return ed
	}
	return nil
}

func (r *result) ResolveExtension(name protoreflect.FullName) protoreflect.ExtensionTypeDescriptor {
	d := r.resolveElement(name)
	if ed, ok := d.(protoreflect.ExtensionDescriptor); ok {
		if !ed.IsExtension() {
			return nil
		}
		if td, ok := ed.(protoreflect.ExtensionTypeDescriptor); ok {
			return td
		}
		return dynamicpb.NewExtensionType(ed).TypeDescriptor()
	}
	return nil
}

func (r *result) ResolveMessageLiteralExtensionName(node ast.IdentValueNode) string {
	return r.optionQualifiedNames[node]
}

func (r *result) resolveElement(name protoreflect.FullName) protoreflect.Descriptor {
	if len(name) > 0 && name[0] == '.' {
		name = name[1:]
	}
	importedFd, res := resolveElement(r, name, false, nil)
	if importedFd != nil {
		r.markUsed(importedFd.Path())
	}
	return res
}

func (r *result) markUsed(importPath string) {
	r.usedImports[importPath] = struct{}{}
}

func (r *result) CheckForUnusedImports(handler *reporter.Handler) {
	fd := r.FileDescriptorProto()
	file, _ := r.FileNode().(*ast.FileNode)
	for i, dep := range fd.Dependency {
		if _, ok := r.usedImports[dep]; !ok {
			isPublic := false
			// it's fine if it's a public import
			for _, j := range fd.PublicDependency {
				if i == int(j) {
					isPublic = true
					break
				}
			}
			if isPublic {
				continue
			}
			pos := ast.UnknownPos(fd.GetName())
			if file != nil {
				for _, decl := range file.Decls {
					imp, ok := decl.(*ast.ImportNode)
					if ok && imp.Name.AsString() == dep {
						pos = file.NodeInfo(imp).Start()
					}
				}
			}
			handler.HandleWarning(pos, errUnusedImport(dep))
		}
	}
}

func resolveElement(f File, fqn protoreflect.FullName, publicImportsOnly bool, checked []string) (imported File, d protoreflect.Descriptor) {
	path := f.Path()
	for _, str := range checked {
		if str == path {
			// already checked
			return nil, nil
		}
	}
	checked = append(checked, path)

	r := resolveElementInFile(fqn, f)
	if r != nil {
		// not imported, but present in f
		return nil, r
	}

	// When publicImportsOnly = false, we are searching only directly imported symbols. But
	// we also need to search transitive public imports due to semantics of public imports.
	for i := 0; i < f.Imports().Len(); i++ {
		dep := f.Imports().Get(i)
		if dep.IsPublic || !publicImportsOnly {
			depFile := f.FindImportByPath(dep.Path())
			_, d := resolveElement(depFile, fqn, true, checked)
			if d != nil {
				return depFile, d
			}
		}
	}

	return nil, nil
}

func (r *result) toDescriptor(fqn string, d proto.Message) protoreflect.Descriptor {
	if ret := r.descriptors[d]; ret != nil {
		// don't bother searching for parent if we don't need it...
		return ret
	}

	parent, index := r.findParent(fqn)
	switch d := d.(type) {
	case *descriptorpb.DescriptorProto:
		return r.asMessageDescriptor(d, r, parent, index, fqn)
	case *descriptorpb.FieldDescriptorProto:
		return r.asFieldDescriptor(d, r, parent, index, fqn)
	case *descriptorpb.OneofDescriptorProto:
		return r.asOneOfDescriptor(d, r, parent.(*msgDescriptor), index, fqn)
	case *descriptorpb.EnumDescriptorProto:
		return r.asEnumDescriptor(d, r, parent, index, fqn)
	case *descriptorpb.EnumValueDescriptorProto:
		return r.asEnumValueDescriptor(d, r, parent.(*enumDescriptor), index, fqn)
	case *descriptorpb.ServiceDescriptorProto:
		return r.asServiceDescriptor(d, r, index, fqn)
	case *descriptorpb.MethodDescriptorProto:
		return r.asMethodDescriptor(d, r, parent.(*svcDescriptor), index, fqn)
	default:
		// WTF? panic?
		return nil
	}
}

func (r *result) findParent(fqn string) (protoreflect.Descriptor, int) {
	names := strings.Split(strings.TrimPrefix(fqn, r.prefix), ".")
	if len(names) == 1 {
		for i, en := range r.FileDescriptorProto().EnumType {
			if en.GetName() == names[0] {
				return r, i
			}
			for j, env := range en.Value {
				if env.GetName() == names[0] {
					return r.asEnumDescriptor(en, r, r, i, r.prefix+en.GetName()), j
				}
			}
		}
		for i, ext := range r.FileDescriptorProto().Extension {
			if ext.GetName() == names[0] {
				return r, i
			}
		}
	}
	for i, svc := range r.FileDescriptorProto().Service {
		if svc.GetName() == names[0] {
			if len(names) == 1 {
				return r, i
			} else {
				if len(names) != 2 {
					return nil, 0
				}
				sd := r.asServiceDescriptor(svc, r, i, r.prefix+svc.GetName())
				for j, mtd := range svc.Method {
					if mtd.GetName() == names[1] {
						return sd, j
					}
				}
			}
		}
	}
	for i, msg := range r.FileDescriptorProto().MessageType {
		if msg.GetName() == names[0] {
			if len(names) == 1 {
				return r, i
			}
			md := r.asMessageDescriptor(msg, r, r, i, r.prefix+msg.GetName())
			return r.findParentInMessage(md, names[1:])
		}
	}
	return nil, 0
}

func (r *result) findParentInMessage(msg *msgDescriptor, names []string) (protoreflect.Descriptor, int) {
	if len(names) == 1 {
		for i, en := range msg.proto.EnumType {
			if en.GetName() == names[0] {
				return msg, i
			}
			for j, env := range en.Value {
				if env.GetName() == names[0] {
					return r.asEnumDescriptor(en, msg.file, msg, i, msg.fqn+"."+en.GetName()), j
				}
			}
		}
		for i, ext := range msg.proto.Extension {
			if ext.GetName() == names[0] {
				return msg, i
			}
		}
		for i, fld := range msg.proto.Field {
			if fld.GetName() == names[0] {
				return msg, i
			}
		}
		for i, ood := range msg.proto.OneofDecl {
			if ood.GetName() == names[0] {
				return msg, i
			}
		}
	}
	for i, nested := range msg.proto.NestedType {
		if nested.GetName() == names[0] {
			if len(names) == 1 {
				return msg, i
			}
			md := r.asMessageDescriptor(nested, msg.file, msg, i, msg.fqn+"."+nested.GetName())
			return r.findParentInMessage(md, names[1:])
		}
	}
	return nil, 0
}

func descriptorTypeWithArticle(d protoreflect.Descriptor) string {
	switch d := d.(type) {
	case protoreflect.MessageDescriptor:
		return "a message"
	case protoreflect.FieldDescriptor:
		if d.IsExtension() {
			return "an extension"
		}
		return "a field"
	case protoreflect.OneofDescriptor:
		return "a oneof"
	case protoreflect.EnumDescriptor:
		return "an enum"
	case protoreflect.EnumValueDescriptor:
		return "an enum value"
	case protoreflect.ServiceDescriptor:
		return "a service"
	case protoreflect.MethodDescriptor:
		return "a method"
	case protoreflect.FileDescriptor:
		return "a file"
	default:
		// shouldn't be possible
		return fmt.Sprintf("a %T", d)
	}
}

func (r *result) resolveReferences(handler *reporter.Handler, s *Symbols) error {
	fd := r.FileDescriptorProto()
	scopes := []scope{fileScope(r)}
	if fd.Options != nil {
		if err := r.resolveOptions(handler, "file", protoreflect.FullName(fd.GetName()), fd.Options.UninterpretedOption, scopes); err != nil {
			return err
		}
	}

	err := walk.DescriptorProtosEnterAndExit(fd,
		func(fqn protoreflect.FullName, d proto.Message) error {
			switch d := d.(type) {
			case *descriptorpb.DescriptorProto:
				// Strangely, when protoc resolves extension names, it uses the *enclosing* scope
				// instead of the message's scope. So if the message contains an extension named "i",
				// an option cannot refer to it as simply "i" but must qualify it (at a minimum "Msg.i").
				// So we don't add this messages scope to our scopes slice until *after* we do options.
				if d.Options != nil {
					if err := r.resolveOptions(handler, "message", fqn, d.Options.UninterpretedOption, scopes); err != nil {
						return err
					}
				}
				scopes = append(scopes, messageScope(r, fqn)) // push new scope on entry
				// walk only visits descriptors, so we need to loop over extension ranges ourselves
				for _, er := range d.ExtensionRange {
					if er.Options != nil {
						erName := protoreflect.FullName(fmt.Sprintf("%s:%d-%d", fqn, er.GetStart(), er.GetEnd()-1))
						if err := r.resolveOptions(handler, "extension range", erName, er.Options.UninterpretedOption, scopes); err != nil {
							return err
						}
					}
				}
			case *descriptorpb.FieldDescriptorProto:
				elemType := "field"
				if d.GetExtendee() != "" {
					elemType = "extension"
				}
				if d.Options != nil {
					if err := r.resolveOptions(handler, elemType, fqn, d.Options.UninterpretedOption, scopes); err != nil {
						return err
					}
				}
				if err := r.resolveFieldTypes(handler, s, fqn, d, scopes); err != nil {
					return err
				}
				if r.Syntax() == protoreflect.Proto3 && !allowedProto3Extendee(d.GetExtendee()) {
					file := r.FileNode()
					node := r.FieldNode(d).FieldExtendee()
					if err := handler.HandleErrorf(file.NodeInfo(node).Start(), "extend blocks in proto3 can only be used to define custom options"); err != nil {
						return err
					}
				}
			case *descriptorpb.OneofDescriptorProto:
				if d.Options != nil {
					if err := r.resolveOptions(handler, "one-of", fqn, d.Options.UninterpretedOption, scopes); err != nil {
						return err
					}
				}
			case *descriptorpb.EnumDescriptorProto:
				if d.Options != nil {
					if err := r.resolveOptions(handler, "enum", fqn, d.Options.UninterpretedOption, scopes); err != nil {
						return err
					}
				}
			case *descriptorpb.EnumValueDescriptorProto:
				if d.Options != nil {
					if err := r.resolveOptions(handler, "enum value", fqn, d.Options.UninterpretedOption, scopes); err != nil {
						return err
					}
				}
			case *descriptorpb.ServiceDescriptorProto:
				if d.Options != nil {
					if err := r.resolveOptions(handler, "service", fqn, d.Options.UninterpretedOption, scopes); err != nil {
						return err
					}
				}
				// not a message, but same scoping rules for nested elements as if it were
				scopes = append(scopes, messageScope(r, fqn)) // push new scope on entry
			case *descriptorpb.MethodDescriptorProto:
				if d.Options != nil {
					if err := r.resolveOptions(handler, "method", fqn, d.Options.UninterpretedOption, scopes); err != nil {
						return err
					}
				}
				if err := r.resolveMethodTypes(handler, fqn, d, scopes); err != nil {
					return err
				}
			}
			return nil
		},
		func(fqn protoreflect.FullName, d proto.Message) error {
			switch d.(type) {
			case *descriptorpb.DescriptorProto, *descriptorpb.ServiceDescriptorProto:
				// pop message scope on exit
				scopes = scopes[:len(scopes)-1]
			}
			return nil
		})

	if err == nil {
		// Because references can by cyclical (for example: message A can have
		// a field of type B and message B can have a field of type A), we can't
		// resolve all descriptors up-front in a straight-forward way. So we
		// instead resolve and cache them "just in time", so that order does not
		// matter.
		//
		// But lazy construction is not thread-safe: we now need to proactively
		// make sure they are all created so that no subsequent operation from some
		// other goroutine triggers it (which could result in unsafe concurrent
		// access and data races).
		_ = walk.Descriptors(r, func(d protoreflect.Descriptor) error {
			switch d := d.(type) {
			case protoreflect.FieldDescriptor:
				d.Message()
				d.Enum()
				d.ContainingMessage()
			case protoreflect.MethodDescriptor:
				d.Input()
				d.Output()
			}
			return nil
		})
	}
	return err
}

var allowedProto3Extendees = map[string]struct{}{
	".google.protobuf.FileOptions":           {},
	".google.protobuf.MessageOptions":        {},
	".google.protobuf.FieldOptions":          {},
	".google.protobuf.OneofOptions":          {},
	".google.protobuf.ExtensionRangeOptions": {},
	".google.protobuf.EnumOptions":           {},
	".google.protobuf.EnumValueOptions":      {},
	".google.protobuf.ServiceOptions":        {},
	".google.protobuf.MethodOptions":         {},
}

func allowedProto3Extendee(n string) bool {
	if n == "" {
		// not an extension, allowed
		return true
	}
	_, ok := allowedProto3Extendees[n]
	return ok
}

func (r *result) resolveFieldTypes(handler *reporter.Handler, s *Symbols, fqn protoreflect.FullName, fld *descriptorpb.FieldDescriptorProto, scopes []scope) error {
	file := r.FileNode()
	node := r.FieldNode(fld)
	elemType := "field"
	scope := fmt.Sprintf("field %s", fqn)
	if fld.GetExtendee() != "" {
		elemType = "extension"
		scope = fmt.Sprintf("extension %s", fqn)
		dsc := r.resolve(fld.GetExtendee(), false, scopes)
		if dsc == nil {
			return handler.HandleErrorf(file.NodeInfo(node.FieldExtendee()).Start(), "unknown extendee type %s", fld.GetExtendee())
		}
		if isSentinelDescriptor(dsc) {
			return handler.HandleErrorf(file.NodeInfo(node.FieldExtendee()).Start(), "unknown extendee type %s; resolved to %s which is not defined; consider using a leading dot", fld.GetExtendee(), dsc.FullName())
		}
		extd, ok := dsc.(protoreflect.MessageDescriptor)
		if !ok {
			return handler.HandleErrorf(file.NodeInfo(node.FieldExtendee()).Start(), "extendee is invalid: %s is %s, not a message", dsc.FullName(), descriptorTypeWithArticle(dsc))
		}
		fld.Extendee = proto.String("." + string(dsc.FullName()))
		// make sure the tag number is in range
		found := false
		tag := protoreflect.FieldNumber(fld.GetNumber())
		for i := 0; i < extd.ExtensionRanges().Len(); i++ {
			rng := extd.ExtensionRanges().Get(i)
			if tag >= rng[0] && tag < rng[1] {
				found = true
				break
			}
		}
		if !found {
			if err := handler.HandleErrorf(file.NodeInfo(node.FieldTag()).Start(), "%s: tag %d is not in valid range for extended type %s", scope, tag, dsc.FullName()); err != nil {
				return err
			}
		} else {
			// make sure tag is not a duplicate
			if err := s.addExtension(dsc.FullName(), tag, file.NodeInfo(node.FieldTag()).Start(), handler); err != nil {
				return err
			}
		}
	}

	if fld.Options != nil {
		if err := r.resolveOptions(handler, elemType, fqn, fld.Options.UninterpretedOption, scopes); err != nil {
			return err
		}
	}

	if fld.GetTypeName() == "" {
		// scalar type; no further resolution required
		return nil
	}

	dsc := r.resolve(fld.GetTypeName(), true, scopes)
	if dsc == nil {
		return handler.HandleErrorf(file.NodeInfo(node.FieldType()).Start(), "%s: unknown type %s", scope, fld.GetTypeName())
	}
	if isSentinelDescriptor(dsc) {
		return handler.HandleErrorf(file.NodeInfo(node.FieldType()).Start(), "%s: unknown type %s; resolved to %s which is not defined; consider using a leading dot", scope, fld.GetTypeName(), dsc.FullName())
	}
	switch dsc := dsc.(type) {
	case protoreflect.MessageDescriptor:
		if dsc.IsMapEntry() {
			isValid := false
			switch node.(type) {
			case *ast.MapFieldNode:
				// We have an AST for this file and can see this field is from a map declaration
				isValid = true
			case ast.NoSourceNode:
				// We don't have an AST for the file (it came from a provided descriptor). So we
				// need to validate that it's not an illegal reference. To be valid, the field
				// must be repeated and the entry type must be nested in the same enclosing
				// message as the field.
				isValid = dsc.FullName() == fqn && fld.GetLabel() == descriptorpb.FieldDescriptorProto_LABEL_REPEATED
			}
			if !isValid {
				return handler.HandleErrorf(file.NodeInfo(node.FieldType()).Start(), "%s: %s is a synthetic map entry and may not be referenced explicitly", scope, dsc.FullName())
			}
		}
		fld.TypeName = proto.String("." + string(dsc.FullName()))
		// if type was tentatively unset, we now know it's actually a message
		if fld.Type == nil {
			fld.Type = descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum()
		}
	case protoreflect.EnumDescriptor:
		proto3 := r.Syntax() == protoreflect.Proto3
		enumIsProto3 := dsc.ParentFile().Syntax() == protoreflect.Proto3
		if fld.GetExtendee() == "" && proto3 && !enumIsProto3 {
			// fields in a proto3 message cannot refer to proto2 enums
			return handler.HandleErrorf(file.NodeInfo(node.FieldType()).Start(), "%s: cannot use proto2 enum %s in a proto3 message", scope, fld.GetTypeName())
		}
		fld.TypeName = proto.String("." + string(dsc.FullName()))
		// the type was tentatively unset, but now we know it's actually an enum
		fld.Type = descriptorpb.FieldDescriptorProto_TYPE_ENUM.Enum()
	default:
		return handler.HandleErrorf(file.NodeInfo(node.FieldType()).Start(), "%s: invalid type: %s is %s, not a message or enum", scope, dsc.FullName(), descriptorTypeWithArticle(dsc))
	}
	return nil
}

func (r *result) resolveMethodTypes(handler *reporter.Handler, fqn protoreflect.FullName, mtd *descriptorpb.MethodDescriptorProto, scopes []scope) error {
	scope := fmt.Sprintf("method %s", fqn)
	file := r.FileNode()
	node := r.MethodNode(mtd)
	dsc := r.resolve(mtd.GetInputType(), false, scopes)
	if dsc == nil {
		if err := handler.HandleErrorf(file.NodeInfo(node.GetInputType()).Start(), "%s: unknown request type %s", scope, mtd.GetInputType()); err != nil {
			return err
		}
	} else if isSentinelDescriptor(dsc) {
		if err := handler.HandleErrorf(file.NodeInfo(node.GetInputType()).Start(), "%s: unknown request type %s; resolved to %s which is not defined; consider using a leading dot", scope, mtd.GetInputType(), dsc.FullName()); err != nil {
			return err
		}
	} else if _, ok := dsc.(protoreflect.MessageDescriptor); !ok {
		if err := handler.HandleErrorf(file.NodeInfo(node.GetInputType()).Start(), "%s: invalid request type: %s is %s, not a message", scope, dsc.FullName(), descriptorTypeWithArticle(dsc)); err != nil {
			return err
		}
	} else {
		mtd.InputType = proto.String("." + string(dsc.FullName()))
	}

	// TODO: make input and output type resolution more DRY
	dsc = r.resolve(mtd.GetOutputType(), false, scopes)
	if dsc == nil {
		if err := handler.HandleErrorf(file.NodeInfo(node.GetOutputType()).Start(), "%s: unknown response type %s", scope, mtd.GetOutputType()); err != nil {
			return err
		}
	} else if isSentinelDescriptor(dsc) {
		if err := handler.HandleErrorf(file.NodeInfo(node.GetInputType()).Start(), "%s: unknown response type %s; resolved to %s which is not defined; consider using a leading dot", scope, mtd.GetOutputType(), dsc.FullName()); err != nil {
			return err
		}
	} else if _, ok := dsc.(protoreflect.MessageDescriptor); !ok {
		if err := handler.HandleErrorf(file.NodeInfo(node.GetOutputType()).Start(), "%s: invalid response type: %s is %s, not a message", scope, dsc.FullName(), descriptorTypeWithArticle(dsc)); err != nil {
			return err
		}
	} else {
		mtd.OutputType = proto.String("." + string(dsc.FullName()))
	}

	return nil
}

func (r *result) resolveOptions(handler *reporter.Handler, elemType string, elemName protoreflect.FullName, opts []*descriptorpb.UninterpretedOption, scopes []scope) error {
	mc := &internal.MessageContext{
		File:        r,
		ElementName: string(elemName),
		ElementType: elemType,
	}
	file := r.FileNode()
opts:
	for _, opt := range opts {
		// resolve any extension names found in option names
		for _, nm := range opt.Name {
			if nm.GetIsExtension() {
				node := r.OptionNamePartNode(nm)
				fqn, err := r.resolveExtensionName(nm.GetNamePart(), scopes)
				if err != nil {
					if err := handler.HandleErrorf(file.NodeInfo(node).Start(), "%v%v", mc, err); err != nil {
						return err
					}
					continue opts
				}
				nm.NamePart = proto.String(fqn)
			}
		}
		// also resolve any extension names found inside message literals in option values
		mc.Option = opt
		optVal := r.OptionNode(opt).GetValue()
		if err := r.resolveOptionValue(handler, mc, optVal, scopes); err != nil {
			return err
		}
		mc.Option = nil
	}
	return nil
}

func (r *result) resolveOptionValue(handler *reporter.Handler, mc *internal.MessageContext, val ast.ValueNode, scopes []scope) error {
	optVal := val.Value()
	switch optVal := optVal.(type) {
	case []ast.ValueNode:
		origPath := mc.OptAggPath
		defer func() {
			mc.OptAggPath = origPath
		}()
		for i, v := range optVal {
			mc.OptAggPath = fmt.Sprintf("%s[%d]", origPath, i)
			if err := r.resolveOptionValue(handler, mc, v, scopes); err != nil {
				return err
			}
		}
	case []*ast.MessageFieldNode:
		origPath := mc.OptAggPath
		defer func() {
			mc.OptAggPath = origPath
		}()
		for _, fld := range optVal {
			// check for extension name
			if fld.Name.IsExtension() {
				fqn, err := r.resolveExtensionName(string(fld.Name.Name.AsIdentifier()), scopes)
				if err != nil {
					if err := handler.HandleErrorf(r.FileNode().NodeInfo(fld.Name.Name).Start(), "%v%v", mc, err); err != nil {
						return err
					}
				} else {
					r.optionQualifiedNames[fld.Name.Name] = fqn
				}
			}

			// recurse into value
			mc.OptAggPath = origPath
			if origPath != "" {
				mc.OptAggPath += "."
			}
			if fld.Name.IsExtension() {
				mc.OptAggPath = fmt.Sprintf("%s[%s]", mc.OptAggPath, string(fld.Name.Name.AsIdentifier()))
			} else {
				mc.OptAggPath = fmt.Sprintf("%s%s", mc.OptAggPath, string(fld.Name.Name.AsIdentifier()))
			}

			if err := r.resolveOptionValue(handler, mc, fld.Val, scopes); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *result) resolveExtensionName(name string, scopes []scope) (string, error) {
	dsc := r.resolve(name, false, scopes)
	if dsc == nil {
		return "", fmt.Errorf("unknown extension %s", name)
	}
	if isSentinelDescriptor(dsc) {
		return "", fmt.Errorf("unknown extension %s; resolved to %s which is not defined; consider using a leading dot", name, dsc.FullName())
	}
	if ext, ok := dsc.(protoreflect.FieldDescriptor); !ok {
		return "", fmt.Errorf("invalid extension: %s is %s, not an extension", name, descriptorTypeWithArticle(dsc))
	} else if !ext.IsExtension() {
		return "", fmt.Errorf("invalid extension: %s is a field but not an extension", name)
	}
	return string("." + dsc.FullName()), nil
}

func (r *result) resolve(name string, onlyTypes bool, scopes []scope) protoreflect.Descriptor {
	if strings.HasPrefix(name, ".") {
		// already fully-qualified
		return r.resolveElement(protoreflect.FullName(name[1:]))
	}
	// unqualified, so we look in the enclosing (last) scope first and move
	// towards outermost (first) scope, trying to resolve the symbol
	pos := strings.IndexByte(name, '.')
	firstName := name
	if pos > 0 {
		firstName = name[:pos]
	}
	var bestGuess protoreflect.Descriptor
	for i := len(scopes) - 1; i >= 0; i-- {
		d := scopes[i](firstName, name)
		if d != nil {
			// In `protoc`, it will skip a match of the wrong type and move on
			// to the next scope, but only if the reference is unqualified. So
			// we mirror that behavior here. When we skip and move on, we go
			// ahead and save the match of the wrong type so we can at least use
			// it to construct a better error in the event that we don't find
			// any match of the right type.
			if !onlyTypes || isType(d) || firstName != name {
				return d
			}
			if bestGuess == nil {
				bestGuess = d
			}
		}
	}
	// we return best guess, even though it was not an allowed kind of
	// descriptor, so caller can print a better error message (e.g.
	// indicating that the name was found but that it's the wrong type)
	return bestGuess
}

func isType(d protoreflect.Descriptor) bool {
	switch d.(type) {
	case protoreflect.MessageDescriptor, protoreflect.EnumDescriptor:
		return true
	}
	return false
}

// scope represents a lexical scope in a proto file in which messages and enums
// can be declared.
type scope func(firstName, fullName string) protoreflect.Descriptor

func fileScope(r *result) scope {
	// we search symbols in this file, but also symbols in other files that have
	// the same package as this file or a "parent" package (in protobuf,
	// packages are a hierarchy like C++ namespaces)
	prefixes := internal.CreatePrefixList(r.FileDescriptorProto().GetPackage())
	querySymbol := func(n string) protoreflect.Descriptor {
		return r.resolveElement(protoreflect.FullName(n))
	}
	return func(firstName, fullName string) protoreflect.Descriptor {
		for _, prefix := range prefixes {
			var n1, n string
			if prefix == "" {
				// exhausted all prefixes, so it must be in this one
				n1, n = fullName, fullName
			} else {
				n = prefix + "." + fullName
				n1 = prefix + "." + firstName
			}
			d := resolveElementRelative(n1, n, querySymbol)
			if d != nil {
				return d
			}
		}
		return nil
	}
}

func messageScope(r *result, messageName protoreflect.FullName) scope {
	querySymbol := func(n string) protoreflect.Descriptor {
		return resolveElementInFile(protoreflect.FullName(n), r)
	}
	return func(firstName, fullName string) protoreflect.Descriptor {
		n1 := string(messageName) + "." + firstName
		n := string(messageName) + "." + fullName
		return resolveElementRelative(n1, n, querySymbol)
	}
}

func resolveElementRelative(firstName, fullName string, query func(name string) protoreflect.Descriptor) protoreflect.Descriptor {
	d := query(firstName)
	if d == nil {
		return nil
	}
	if firstName == fullName {
		return d
	}
	if !isAggregateDescriptor(d) {
		// can't possibly find the rest of full name if
		// the first name indicated a leaf descriptor
		return nil
	}
	d = query(fullName)
	if d == nil {
		return newSentinelDescriptor(fullName)
	}
	return d
}

func resolveElementInFile(name protoreflect.FullName, f File) protoreflect.Descriptor {
	d := f.FindDescriptorByName(name)
	if d != nil {
		return d
	}

	if matchesPkgNamespace(name, f.Package()) {
		// this sentinel means the name is a valid namespace but
		// does not refer to a descriptor
		return newSentinelDescriptor(string(name))
	}
	return nil
}

func matchesPkgNamespace(fqn, pkg protoreflect.FullName) bool {
	if pkg == "" {
		return false
	}
	if fqn == pkg {
		return true
	}
	if len(pkg) > len(fqn) && strings.HasPrefix(string(pkg), string(fqn)) {
		// if char after fqn is a dot, then fqn is a namespace
		if pkg[len(fqn)] == '.' {
			return true
		}
	}
	return false
}

func isAggregateDescriptor(d protoreflect.Descriptor) bool {
	if isSentinelDescriptor(d) {
		// this indicates the name matched a package, not a
		// descriptor, but a package is an aggregate, so
		// we return true
		return true
	}
	switch d.(type) {
	case protoreflect.MessageDescriptor, protoreflect.EnumDescriptor, protoreflect.ServiceDescriptor:
		return true
	default:
		return false
	}
}

func isSentinelDescriptor(d protoreflect.Descriptor) bool {
	_, ok := d.(*sentinelDescriptor)
	return ok
}

func newSentinelDescriptor(name string) protoreflect.Descriptor {
	return &sentinelDescriptor{name: name}
}

// sentinelDescriptor is a placeholder descriptor. It is used instead of nil to
// distinguish between two situations:
//  1. The given name could not be found.
//  2. The given name *cannot* be a valid result so stop searching.
//
// In these cases, attempts to resolve an element name will return nil for the
// first case and will return a sentinelDescriptor in the second. The sentinel
// contains the fully-qualified name which caused the search to stop (which may
// be a prefix of the actual name being resolved).
type sentinelDescriptor struct {
	protoreflect.Descriptor
	name string
}

func (p *sentinelDescriptor) ParentFile() protoreflect.FileDescriptor {
	return nil
}

func (p *sentinelDescriptor) Parent() protoreflect.Descriptor {
	return nil
}

func (p *sentinelDescriptor) Index() int {
	return 0
}

func (p *sentinelDescriptor) Syntax() protoreflect.Syntax {
	return 0
}

func (p *sentinelDescriptor) Name() protoreflect.Name {
	return protoreflect.Name(p.name)
}

func (p *sentinelDescriptor) FullName() protoreflect.FullName {
	return protoreflect.FullName(p.name)
}

func (p *sentinelDescriptor) IsPlaceholder() bool {
	return false
}

func (p *sentinelDescriptor) Options() protoreflect.ProtoMessage {
	return nil
}

var _ protoreflect.Descriptor = (*sentinelDescriptor)(nil)
