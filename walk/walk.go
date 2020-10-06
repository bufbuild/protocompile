package walk

import (
	"github.com/jhump/protocompile/internal"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

func Descriptors(file protoreflect.FileDescriptor, fn func(protoreflect.Descriptor) error) error {
	return DescriptorsEnterAndExit(file, fn, nil)
}

func DescriptorsEnterAndExit(file protoreflect.FileDescriptor, enter, exit func(protoreflect.Descriptor) error) error {
	for i := 0; i < file.Messages().Len(); i++ {
		msg := file.Messages().Get(i)
		if err := messageDescriptor(msg, enter, exit); err != nil {
			return err
		}
	}
	for i := 0; i < file.Enums().Len(); i++ {
		en := file.Enums().Get(i)
		if err := enumDescriptor(en, enter, exit); err != nil {
			return err
		}
	}
	for i := 0; i < file.Extensions().Len(); i++ {
		ext := file.Extensions().Get(i)
		if err := enter(ext); err != nil {
			return err
		}
		if exit != nil {
			if err := exit(ext); err != nil {
				return err
			}
		}
	}
	for i := 0; i < file.Services().Len(); i++ {
		svc := file.Services().Get(i)
		if err := enter(svc); err != nil {
			return err
		}
		for i := 0; i < svc.Methods().Len(); i++ {
			mtd := svc.Methods().Get(i)
			if err := enter(mtd); err != nil {
				return err
			}
			if exit != nil {
				if err := exit(mtd); err != nil {
					return err
				}
			}
		}
		if exit != nil {
			if err := exit(svc); err != nil {
				return err
			}
		}
	}
	return nil
}

func messageDescriptor(msg protoreflect.MessageDescriptor, enter, exit func(protoreflect.Descriptor) error) error {
	if err := enter(msg); err != nil {
		return err
	}
	for i := 0; i < msg.Fields().Len(); i++ {
		fld := msg.Fields().Get(i)
		if err := enter(fld); err != nil {
			return err
		}
		if exit != nil {
			if err := exit(fld); err != nil {
				return err
			}
		}
	}
	for i := 0; i < msg.Oneofs().Len(); i++ {
		oo := msg.Oneofs().Get(i)
		if err := enter(oo); err != nil {
			return err
		}
		if exit != nil {
			if err := exit(oo); err != nil {
				return err
			}
		}
	}
	for i := 0; i < msg.Messages().Len(); i++ {
		nested := msg.Messages().Get(i)
		if err := messageDescriptor(nested, enter, exit); err != nil {
			return err
		}
	}
	for i := 0; i < msg.Enums().Len(); i++ {
		en := msg.Enums().Get(i)
		if err := enumDescriptor(en, enter, exit); err != nil {
			return err
		}
	}
	for i := 0; i < msg.Extensions().Len(); i++ {
		ext := msg.Extensions().Get(i)
		if err := enter(ext); err != nil {
			return err
		}
		if exit != nil {
			if err := exit(ext); err != nil {
				return err
			}
		}
	}
	if exit != nil {
		if err := exit(msg); err != nil {
			return err
		}
	}
	return nil
}

func enumDescriptor(en protoreflect.EnumDescriptor, enter, exit func(protoreflect.Descriptor) error) error {
	if err := enter(en); err != nil {
		return err
	}
	for i := 0; i < en.Values().Len(); i++ {
		enVal := en.Values().Get(i)
		if err := enter(enVal); err != nil {
			return err
		}
		if exit != nil {
			if err := exit(enVal); err != nil {
				return err
			}
		}
	}
	if exit != nil {
		if err := exit(en); err != nil {
			return err
		}
	}
	return nil
}

func DescriptorProtosWithPath(file *descriptorpb.FileDescriptorProto, fn func(protoreflect.FullName, protoreflect.SourcePath, proto.Message) error) error {
	return DescriptorProtosWithPathEnterAndExit(file, fn, nil)
}

func DescriptorProtosWithPathEnterAndExit(file *descriptorpb.FileDescriptorProto, enter, exit func(protoreflect.FullName, protoreflect.SourcePath, proto.Message) error) error {
	w := &protoWalker{usePath: true, enter: enter, exit: exit}
	return w.walkDescriptorProtos(file)
}

func DescriptorProtos(file *descriptorpb.FileDescriptorProto, fn func(protoreflect.FullName, proto.Message) error) error {
	return DescriptorProtosEnterAndExit(file, fn, nil)
}

func DescriptorProtosEnterAndExit(file *descriptorpb.FileDescriptorProto, enter, exit func(protoreflect.FullName, proto.Message) error) error {
	enterWithPath := func(n protoreflect.FullName, p protoreflect.SourcePath, m proto.Message) error {
		return enter(n, m)
	}
	var exitWithPath func(n protoreflect.FullName, p protoreflect.SourcePath, m proto.Message) error
	if exit != nil {
		exitWithPath = func(n protoreflect.FullName, p protoreflect.SourcePath, m proto.Message) error {
			return exit(n, m)
		}
	}
	w := &protoWalker{
		enter: enterWithPath,
		exit:  exitWithPath,
	}
	return w.walkDescriptorProtos(file)
}

type protoWalker struct {
	usePath     bool
	enter, exit func(protoreflect.FullName, protoreflect.SourcePath, proto.Message) error
}

func (w *protoWalker) walkDescriptorProtos(file *descriptorpb.FileDescriptorProto) error {
	prefix := file.GetPackage()
	if prefix != "" {
		prefix = prefix + "."
	}
	var path protoreflect.SourcePath
	for i, msg := range file.MessageType {
		var p protoreflect.SourcePath
		if w.usePath {
			p = append(path, internal.File_messagesTag, int32(i))
		}
		if err := w.walkDescriptorProto(prefix, p, msg); err != nil {
			return err
		}
	}
	for i, en := range file.EnumType {
		var p protoreflect.SourcePath
		if w.usePath {
			p = append(path, internal.File_enumsTag, int32(i))
		}
		if err := w.walkEnumDescriptorProto(prefix, p, en); err != nil {
			return err
		}
	}
	for i, ext := range file.Extension {
		var p protoreflect.SourcePath
		if w.usePath {
			p = append(path, internal.File_extensionsTag, int32(i))
		}
		fqn := prefix + ext.GetName()
		if err := w.enter(protoreflect.FullName(fqn), p, ext); err != nil {
			return err
		}
		if w.exit != nil {
			if err := w.exit(protoreflect.FullName(fqn), p, ext); err != nil {
				return err
			}
		}
	}
	for i, svc := range file.Service {
		var p protoreflect.SourcePath
		if w.usePath {
			p = append(path, internal.File_servicesTag, int32(i))
		}
		fqn := prefix + svc.GetName()
		if err := w.enter(protoreflect.FullName(fqn), p, svc); err != nil {
			return err
		}
		for j, mtd := range svc.Method {
			var mp protoreflect.SourcePath
			if w.usePath {
				mp = append(p, internal.Service_methodsTag, int32(j))
			}
			mtdFqn := fqn + "." + mtd.GetName()
			if err := w.enter(protoreflect.FullName(mtdFqn), mp, mtd); err != nil {
				return err
			}
			if w.exit != nil {
				if err := w.exit(protoreflect.FullName(mtdFqn), mp, mtd); err != nil {
					return err
				}
			}
		}
		if w.exit != nil {
			if err := w.exit(protoreflect.FullName(fqn), p, svc); err != nil {
				return err
			}
		}
	}
	return nil
}

func (w *protoWalker) walkDescriptorProto(prefix string, path protoreflect.SourcePath, msg *descriptorpb.DescriptorProto) error {
	fqn := prefix + msg.GetName()
	if err := w.enter(protoreflect.FullName(fqn), path, msg); err != nil {
		return err
	}
	prefix = fqn + "."
	for i, fld := range msg.Field {
		var p protoreflect.SourcePath
		if w.usePath {
			p = append(path, internal.Message_fieldsTag, int32(i))
		}
		fqn := prefix + fld.GetName()
		if err := w.enter(protoreflect.FullName(fqn), p, fld); err != nil {
			return err
		}
		if w.exit != nil {
			if err := w.exit(protoreflect.FullName(fqn), p, fld); err != nil {
				return err
			}
		}
	}
	for i, oo := range msg.OneofDecl {
		var p protoreflect.SourcePath
		if w.usePath {
			p = append(path, internal.Message_oneOfsTag, int32(i))
		}
		fqn := prefix + oo.GetName()
		if err := w.enter(protoreflect.FullName(fqn), p, oo); err != nil {
			return err
		}
		if w.exit != nil {
			if err := w.exit(protoreflect.FullName(fqn), p, oo); err != nil {
				return err
			}
		}
	}
	for i, nested := range msg.NestedType {
		var p protoreflect.SourcePath
		if w.usePath {
			p = append(path, internal.Message_nestedMessagesTag, int32(i))
		}
		if err := w.walkDescriptorProto(prefix, p, nested); err != nil {
			return err
		}
	}
	for i, en := range msg.EnumType {
		var p protoreflect.SourcePath
		if w.usePath {
			p = append(path, internal.Message_enumsTag, int32(i))
		}
		if err := w.walkEnumDescriptorProto(prefix, p, en); err != nil {
			return err
		}
	}
	for i, ext := range msg.Extension {
		var p protoreflect.SourcePath
		if w.usePath {
			p = append(path, internal.Message_extensionsTag, int32(i))
		}
		fqn := prefix + ext.GetName()
		if err := w.enter(protoreflect.FullName(fqn), p, ext); err != nil {
			return err
		}
		if w.exit != nil {
			if err := w.exit(protoreflect.FullName(fqn), p, ext); err != nil {
				return err
			}
		}
	}
	if w.exit != nil {
		if err := w.exit(protoreflect.FullName(fqn), path, msg); err != nil {
			return err
		}
	}
	return nil
}

func (w *protoWalker) walkEnumDescriptorProto(prefix string, path protoreflect.SourcePath, en *descriptorpb.EnumDescriptorProto) error {
	fqn := prefix + en.GetName()
	if err := w.enter(protoreflect.FullName(fqn), path, en); err != nil {
		return err
	}
	for i, val := range en.Value {
		var p protoreflect.SourcePath
		if w.usePath {
			p = append(path, internal.Enum_valuesTag, int32(i))
		}
		fqn := prefix + val.GetName()
		if err := w.enter(protoreflect.FullName(fqn), p, val); err != nil {
			return err
		}
		if w.exit != nil {
			if err := w.exit(protoreflect.FullName(fqn), p, val); err != nil {
				return err
			}
		}
	}
	if w.exit != nil {
		if err := w.exit(protoreflect.FullName(fqn), path, en); err != nil {
			return err
		}
	}
	return nil
}
