package linker

import (
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"

	"github.com/jhump/protocompile/internal"
	"github.com/jhump/protocompile/reporter"
	"github.com/jhump/protocompile/walk"
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
		return extTypeDesc{ed}
	}
	return nil
}

type extTypeDesc struct {
	protoreflect.ExtensionDescriptor
}

func (e extTypeDesc) Type() protoreflect.ExtensionType {
	return dynamicpb.NewExtensionType(e.ExtensionDescriptor)
}

func (e extTypeDesc) Descriptor() protoreflect.ExtensionDescriptor {
	return e.ExtensionDescriptor
}

func (r *result) resolveElement(name protoreflect.FullName) protoreflect.Descriptor {
	importedFd, res := resolveElement(r, name, false, nil)
	if importedFd != nil {
		r.markUsed(importedFd.Path())
	}
	return res
}

func (r *result) markUsed(importPath string) {
	r.usedImports[importPath] = struct{}{}
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

	r := f.FindDescriptorByName(fqn)
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
		for i, en := range r.Proto().EnumType {
			if en.GetName() == names[0] {
				return r, i
			}
		}
		for i, ext := range r.Proto().Extension {
			if ext.GetName() == names[0] {
				return r, i
			}
		}
	}
	for i, msg := range r.Proto().MessageType {
		if msg.GetName() == names[0] {
			if len(names) == 1 {
				return r, i
			}
			md := r.asMessageDescriptor(msg, r, r, i, r.prefix+msg.GetName())
			return r.findParentMessage(md, names[1:])
		}
	}
	return nil, 0
}

func (r *result) findParentMessage(msg *msgDescriptor, names []string) (protoreflect.MessageDescriptor, int) {
	if len(names) == 1 {
		for i, en := range msg.proto.EnumType {
			if en.GetName() == names[0] {
				return msg, i
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
	}
	for i, nested := range msg.proto.NestedType {
		if nested.GetName() == names[0] {
			if len(names) == 1 {
				return msg, i
			}
			md := r.asMessageDescriptor(nested, msg.file, msg, i, msg.fqn+"."+nested.GetName())
			return r.findParentMessage(md, names[1:])
		}
	}
	return nil, 0
}

func descriptorType(d protoreflect.Descriptor) string {
	switch d := d.(type) {
	case protoreflect.MessageDescriptor:
		return "message"
	case protoreflect.FieldDescriptor:
		if d.IsExtension() {
			return "extension"
		}
		return "field"
	case protoreflect.EnumDescriptor:
		return "enum"
	case protoreflect.EnumValueDescriptor:
		return "enum value"
	case protoreflect.ServiceDescriptor:
		return "service"
	case protoreflect.MethodDescriptor:
		return "method"
	case protoreflect.FileDescriptor:
		return "file"
	default:
		// shouldn't be possible
		return fmt.Sprintf("%T", d)
	}
}

func (r *result) resolveReferences(handler *reporter.Handler, s *Symbols) error {
	fd := r.Proto()
	scopes := []scope{fileScope(r)}
	if fd.Options != nil {
		if err := r.resolveOptions(handler, "file", protoreflect.FullName(fd.GetName()), fd.Options.UninterpretedOption, scopes); err != nil {
			return err
		}
	}

	return walk.DescriptorProtosEnterAndExit(fd,
		func(fqn protoreflect.FullName, d proto.Message) error {
			switch d := d.(type) {
			case *descriptorpb.DescriptorProto:
				scopes = append(scopes, messageScope(r, fqn)) // push new scope on entry
				if d.Options != nil {
					if err := r.resolveOptions(handler, "message", fqn, d.Options.UninterpretedOption, scopes); err != nil {
						return err
					}
				}
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
			if _, ok := d.(*descriptorpb.DescriptorProto); ok {
				// pop message scope on exit
				scopes = scopes[:len(scopes)-1]
			}
			return nil
		})
}

func (r *result) resolveFieldTypes(handler *reporter.Handler, s *Symbols, fqn protoreflect.FullName, fld *descriptorpb.FieldDescriptorProto, scopes []scope) error {
	scope := fmt.Sprintf("field %s", fqn)
	node := r.FieldNode(fld)
	elemType := "field"
	if fld.GetExtendee() != "" {
		elemType = "extension"
		dsc := r.resolve(fld.GetExtendee(), isMessage, scopes)
		if dsc == nil {
			return handler.HandleErrorf(node.FieldExtendee().Start(), "unknown extendee type %s", fld.GetExtendee())
		}
		extd, ok := dsc.(protoreflect.MessageDescriptor)
		if !ok {
			otherType := descriptorType(dsc)
			return handler.HandleErrorf(node.FieldExtendee().Start(), "extendee is invalid: %s is a %s, not a message", fqn, otherType)
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
			if err := handler.HandleErrorf(node.FieldTag().Start(), "%s: tag %d is not in valid range for extended type %s", scope, tag, fqn); err != nil {
				return err
			}
		} else {
			// make sure tag is not a duplicate
			if err := s.addExtension(dsc.FullName(), tag, node.Start(), handler); err != nil {
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

	dsc := r.resolve(fld.GetTypeName(), isType, scopes)
	if dsc == nil {
		return handler.HandleErrorf(node.FieldType().Start(), "%s: unknown type %s", scope, fld.GetTypeName())
	}
	switch dsc := dsc.(type) {
	case protoreflect.MessageDescriptor:
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
			return handler.HandleErrorf(node.FieldType().Start(), "%s: cannot use proto2 enum %s in a proto3 message", scope, fld.GetTypeName())
		}
		fld.TypeName = proto.String("." + string(dsc.FullName()))
		// the type was tentatively unset, but now we know it's actually an enum
		fld.Type = descriptorpb.FieldDescriptorProto_TYPE_ENUM.Enum()
	default:
		otherType := descriptorType(dsc)
		return handler.HandleErrorf(node.FieldType().Start(), "%s: invalid type: %s is a %s, not a message or enum", scope, dsc.FullName(), otherType)
	}
	return nil
}

func (r *result) resolveMethodTypes(handler *reporter.Handler, fqn protoreflect.FullName, mtd *descriptorpb.MethodDescriptorProto, scopes []scope) error {
	scope := fmt.Sprintf("method %s", fqn)
	node := r.MethodNode(mtd)
	dsc := r.resolve(mtd.GetInputType(), isMessage, scopes)
	if dsc == nil {
		if err := handler.HandleErrorf(node.GetInputType().Start(), "%s: unknown request type %s", scope, mtd.GetInputType()); err != nil {
			return err
		}
	} else if _, ok := dsc.(protoreflect.MessageDescriptor); !ok {
		otherType := descriptorType(dsc)
		if err := handler.HandleErrorf(node.GetInputType().Start(), "%s: invalid request type: %s is a %s, not a message", scope, dsc.FullName(), otherType); err != nil {
			return err
		}
	} else {
		mtd.InputType = proto.String("." + string(dsc.FullName()))
	}

	dsc = r.resolve(mtd.GetOutputType(), isMessage, scopes)
	if dsc == nil {
		if err := handler.HandleErrorf(node.GetOutputType().Start(), "%s: unknown response type %s", scope, mtd.GetOutputType()); err != nil {
			return err
		}
	} else if _, ok := dsc.(protoreflect.MessageDescriptor); !ok {
		otherType := descriptorType(dsc)
		if err := handler.HandleErrorf(node.GetOutputType().Start(), "%s: invalid response type: %s is a %s, not a message", scope, dsc.FullName(), otherType); err != nil {
			return err
		}
	} else {
		mtd.OutputType = proto.String("." + string(dsc.FullName()))
	}

	return nil
}

func (r *result) resolveOptions(handler *reporter.Handler, elemType string, elemName protoreflect.FullName, opts []*descriptorpb.UninterpretedOption, scopes []scope) error {
	var scope string
	if elemType != "file" {
		scope = fmt.Sprintf("%s %s: ", elemType, elemName)
	}
opts:
	for _, opt := range opts {
		for _, nm := range opt.Name {
			if nm.GetIsExtension() {
				node := r.OptionNamePartNode(nm)
				dsc := r.resolve(nm.GetNamePart(), isField, scopes)
				if dsc == nil {
					if err := handler.HandleErrorf(node.Start(), "%sunknown extension %s", scope, nm.GetNamePart()); err != nil {
						return err
					}
					continue opts
				}
				if ext, ok := dsc.(protoreflect.FieldDescriptor); !ok {
					otherType := descriptorType(dsc)
					if err := handler.HandleErrorf(node.Start(), "%sinvalid extension: %s is a %s, not an extension", scope, nm.GetNamePart(), otherType); err != nil {
						return err
					}
					continue opts
				} else if !ext.IsExtension() {
					if err := handler.HandleErrorf(node.Start(), "%sinvalid extension: %s is a field but not an extension", scope, nm.GetNamePart()); err != nil {
						return err
					}
					continue opts
				}
				nm.NamePart = proto.String("." + string(dsc.FullName()))
			}
		}
	}
	return nil
}

func (r *result) resolve(name string, allowed func(protoreflect.Descriptor) bool, scopes []scope) protoreflect.Descriptor {
	if strings.HasPrefix(name, ".") {
		// already fully-qualified
		return r.resolveElement(protoreflect.FullName(name[1:]))
	}
	// unqualified, so we look in the enclosing (last) scope first and move
	// towards outermost (first) scope, trying to resolve the symbol
	var bestGuess protoreflect.Descriptor
	for i := len(scopes) - 1; i >= 0; i-- {
		d := scopes[i](name)
		if d != nil {
			if allowed(d) {
				return d
			} else if bestGuess == nil {
				bestGuess = d
			}
		}
	}
	// we return best guess, even though it was not an allowed kind of
	// descriptor, so caller can print a better error message (e.g.
	// indicating that the name was found but that it's the wrong type)
	return bestGuess
}

func isField(d protoreflect.Descriptor) bool {
	_, ok := d.(protoreflect.FieldDescriptor)
	return ok
}

func isMessage(d protoreflect.Descriptor) bool {
	_, ok := d.(protoreflect.MessageDescriptor)
	return ok
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
type scope func(symbol string) protoreflect.Descriptor

func fileScope(r *result) scope {
	// we search symbols in this file, but also symbols in other files that have
	// the same package as this file or a "parent" package (in protobuf,
	// packages are a hierarchy like C++ namespaces)
	prefixes := internal.CreatePrefixList(r.Proto().GetPackage())
	return func(name string) protoreflect.Descriptor {
		for _, prefix := range prefixes {
			var n string
			if prefix == "" {
				n = name
			} else {
				n = prefix + "." + name
			}
			d := r.resolveElement(protoreflect.FullName(n))
			if d != nil {
				return d
			}
		}
		return nil
	}
}

func messageScope(r *result, messageName protoreflect.FullName) scope {
	return func(name string) protoreflect.Descriptor {
		n := string(messageName) + "." + name
		if d, ok := r.descriptorPool[n]; ok {
			return r.toDescriptor(n, d)
		}
		return nil
	}
}
