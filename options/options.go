// Package options contains the logic for interpreting options. The parse step
// of compilation stores the options in uninterpreted form, which contains raw
// identifiers and literal values.
//
// The process of interpreting an option is to resolve identifiers, by examining
// descriptors for the Options types and their available extensions (custom
// options). As field names are resolved, the values can be type-checked against
// the types indicated in field descriptors.
//
// On success, the various fields and extensions of the options message are
// populated and the field holding the uninterpreted form is cleared.
package options

import (
	"bytes"
	"fmt"
	"math"
	"sort"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"

	"github.com/jhump/protocompile/ast"
	"github.com/jhump/protocompile/internal"
	"github.com/jhump/protocompile/linker"
	"github.com/jhump/protocompile/parser"
	"github.com/jhump/protocompile/reporter"
)

// Index is a mapping of AST nodes that define options to a corresponding path
// into the containing file descriptor. The path is a sequence of field tags
// and indexes that define a traversal path from the root (the file descriptor)
// to the resolved option field.
type Index map[*ast.OptionNode][]int32

type interpreter struct {
	file     file
	resolver linker.Resolver
	lenient  bool
	reporter *reporter.Handler
	index    Index
}

type file interface {
	parser.Result
	ResolveEnumType(protoreflect.FullName) protoreflect.EnumDescriptor
	ResolveMessageType(protoreflect.FullName) protoreflect.MessageDescriptor
	ResolveExtension(protoreflect.FullName) protoreflect.ExtensionTypeDescriptor
}

type noResolveFile struct {
	parser.Result
}

func (n noResolveFile) ResolveEnumType(name protoreflect.FullName) protoreflect.EnumDescriptor {
	return nil
}

func (n noResolveFile) ResolveMessageType(name protoreflect.FullName) protoreflect.MessageDescriptor {
	return nil
}

func (n noResolveFile) ResolveExtension(name protoreflect.FullName) protoreflect.ExtensionTypeDescriptor {
	return nil
}

// InterpretOptions interprets options in the given linked result, returning
// an index that can be used to generate source code info. This step mutates
// the linked result's underlying proto to move option elements out of the
// "uninterpreted_option" fields and into proper option fields and extensions.
//
// The given handler is used to report errors and warnings. If any errors are
// reported, this function returns a non-nil error.
func InterpretOptions(linked linker.Result, handler *reporter.Handler) (Index, error) {
	return interpretOptions(false, linked, handler)
}

// InterpretOptionsLenient interprets options in a lenient/best-effort way in
// the given linked result, returning an index that can be used to generate
// source code info. This step mutates the linked result's underlying proto to
// move option elements out of the "uninterpreted_option" fields and into proper
// option fields and extensions.
//
// In lenient more, errors resolving option names and type errors are ignored.
// Any options that are uninterpretable (due to such errors) will remain in the
// "uninterpreted_option" fields.
func InterpretOptionsLenient(linked linker.Result) (Index, error) {
	return interpretOptions(true, linked, reporter.NewHandler(nil))
}

// InterpretUnlinkedOptions does a best-effort attempt to interpret options in
// the given parsed result, returning an index that can be used to generate
// source code info. This step mutates the parsed result's underlying proto to
// move option elements out of the "uninterpreted_option" fields and into proper
// option fields and extensions.
//
// This is the same as InterpretOptionsLenient except that it accepts an
// unlinked result. Because the file is unlinked, custom options cannot be
// interpreted. Other errors resolving option names or type errors will be
// effectively ignored. Any options that are uninterpretable (due to such
// errors) will remain in the "uninterpreted_option" fields.
func InterpretUnlinkedOptions(parsed parser.Result) (Index, error) {
	return interpretOptions(true, noResolveFile{parsed}, reporter.NewHandler(nil))
}

func interpretOptions(lenient bool, file file, handler *reporter.Handler) (Index, error) {
	interp := interpreter{
		file:     file,
		lenient:  lenient,
		reporter: handler,
		index:    Index{},
	}
	if f, ok := file.(linker.File); ok {
		interp.resolver = linker.ResolverFromFile(f)
	}

	fd := file.Proto()
	prefix := fd.GetPackage()
	if prefix != "" {
		prefix += "."
	}
	opts := fd.GetOptions()
	if opts != nil {
		if len(opts.UninterpretedOption) > 0 {
			if remain, err := interp.interpretOptions(fd.GetName(), fd, opts, opts.UninterpretedOption); err != nil {
				return nil, err
			} else {
				opts.UninterpretedOption = remain
			}
		}
	}
	for _, md := range fd.GetMessageType() {
		fqn := prefix + md.GetName()
		if err := interp.interpretMessageOptions(fqn, md); err != nil {
			return nil, err
		}
	}
	for _, fld := range fd.GetExtension() {
		fqn := prefix + fld.GetName()
		if err := interp.interpretFieldOptions(fqn, fld); err != nil {
			return nil, err
		}
	}
	for _, ed := range fd.GetEnumType() {
		fqn := prefix + ed.GetName()
		if err := interp.interpretEnumOptions(fqn, ed); err != nil {
			return nil, err
		}
	}
	for _, sd := range fd.GetService() {
		fqn := prefix + sd.GetName()
		opts := sd.GetOptions()
		if len(opts.GetUninterpretedOption()) > 0 {
			if remain, err := interp.interpretOptions(fqn, sd, opts, opts.UninterpretedOption); err != nil {
				return nil, err
			} else {
				opts.UninterpretedOption = remain
			}
		}
		for _, mtd := range sd.GetMethod() {
			mtdFqn := fqn + "." + mtd.GetName()
			opts := mtd.GetOptions()
			if len(opts.GetUninterpretedOption()) > 0 {
				if remain, err := interp.interpretOptions(mtdFqn, mtd, opts, opts.UninterpretedOption); err != nil {
					return nil, err
				} else {
					opts.UninterpretedOption = remain
				}
			}
		}
	}
	return interp.index, nil
}

func (interp *interpreter) nodeInfo(n ast.Node) ast.NodeInfo {
	return interp.file.FileNode().NodeInfo(n)
}

func (interp *interpreter) interpretMessageOptions(fqn string, md *descriptorpb.DescriptorProto) error {
	opts := md.GetOptions()
	if opts != nil {
		if len(opts.UninterpretedOption) > 0 {
			if remain, err := interp.interpretOptions(fqn, md, opts, opts.UninterpretedOption); err != nil {
				return err
			} else {
				opts.UninterpretedOption = remain
			}
		}
	}
	for _, fld := range md.GetField() {
		fldFqn := fqn + "." + fld.GetName()
		if err := interp.interpretFieldOptions(fldFqn, fld); err != nil {
			return err
		}
	}
	for _, ood := range md.GetOneofDecl() {
		oodFqn := fqn + "." + ood.GetName()
		opts := ood.GetOptions()
		if len(opts.GetUninterpretedOption()) > 0 {
			if remain, err := interp.interpretOptions(oodFqn, ood, opts, opts.UninterpretedOption); err != nil {
				return err
			} else {
				opts.UninterpretedOption = remain
			}
		}
	}
	for _, fld := range md.GetExtension() {
		fldFqn := fqn + "." + fld.GetName()
		if err := interp.interpretFieldOptions(fldFqn, fld); err != nil {
			return err
		}
	}
	for _, er := range md.GetExtensionRange() {
		erFqn := fmt.Sprintf("%s.%d-%d", fqn, er.GetStart(), er.GetEnd())
		opts := er.GetOptions()
		if len(opts.GetUninterpretedOption()) > 0 {
			if remain, err := interp.interpretOptions(erFqn, er, opts, opts.UninterpretedOption); err != nil {
				return err
			} else {
				opts.UninterpretedOption = remain
			}
		}
	}
	for _, nmd := range md.GetNestedType() {
		nmdFqn := fqn + "." + nmd.GetName()
		if err := interp.interpretMessageOptions(nmdFqn, nmd); err != nil {
			return err
		}
	}
	for _, ed := range md.GetEnumType() {
		edFqn := fqn + "." + ed.GetName()
		if err := interp.interpretEnumOptions(edFqn, ed); err != nil {
			return err
		}
	}
	return nil
}

func (interp *interpreter) interpretFieldOptions(fqn string, fld *descriptorpb.FieldDescriptorProto) error {
	opts := fld.GetOptions()
	if len(opts.GetUninterpretedOption()) > 0 {
		uo := opts.UninterpretedOption
		scope := fmt.Sprintf("field %s", fqn)

		// process json_name pseudo-option
		if index, err := internal.FindOption(interp.file, interp.reporter, scope, uo, "json_name"); err != nil && !interp.lenient {
			return err
		} else if index >= 0 {
			opt := uo[index]
			optNode := interp.file.OptionNode(opt)

			// attribute source code info
			if on, ok := optNode.(*ast.OptionNode); ok {
				interp.index[on] = []int32{-1, internal.Field_jsonNameTag}
			}
			uo = internal.RemoveOption(uo, index)
			if opt.StringValue == nil {
				if err := interp.reporter.HandleErrorf(interp.nodeInfo(optNode.GetValue()).Start(), "%s: expecting string value for json_name option", scope); err != nil {
					return err
				}
			} else {
				fld.JsonName = proto.String(string(opt.StringValue))
			}
		}

		// and process default pseudo-option
		if index, err := interp.processDefaultOption(scope, fqn, fld, uo); err != nil && !interp.lenient {
			return err
		} else if index >= 0 {
			// attribute source code info
			optNode := interp.file.OptionNode(uo[index])
			if on, ok := optNode.(*ast.OptionNode); ok {
				interp.index[on] = []int32{-1, internal.Field_defaultTag}
			}
			uo = internal.RemoveOption(uo, index)
		}

		if len(uo) == 0 {
			// no real options, only pseudo-options above? clear out options
			fld.Options = nil
		} else if remain, err := interp.interpretOptions(fqn, fld, opts, uo); err != nil {
			return err
		} else {
			opts.UninterpretedOption = remain
		}
	}
	return nil
}

func (interp *interpreter) processDefaultOption(scope string, fqn string, fld *descriptorpb.FieldDescriptorProto, uos []*descriptorpb.UninterpretedOption) (defaultIndex int, err error) {
	found, err := internal.FindOption(interp.file, interp.reporter, scope, uos, "default")
	if err != nil || found == -1 {
		return -1, err
	}
	opt := uos[found]
	optNode := interp.file.OptionNode(opt)
	if fld.GetLabel() == descriptorpb.FieldDescriptorProto_LABEL_REPEATED {
		return -1, interp.reporter.HandleErrorf(interp.nodeInfo(optNode.GetName()).Start(), "%s: default value cannot be set because field is repeated", scope)
	}
	if fld.GetType() == descriptorpb.FieldDescriptorProto_TYPE_GROUP || fld.GetType() == descriptorpb.FieldDescriptorProto_TYPE_MESSAGE {
		return -1, interp.reporter.HandleErrorf(interp.nodeInfo(optNode.GetName()).Start(), "%s: default value cannot be set because field is a message", scope)
	}
	val := optNode.GetValue()
	if _, ok := val.(*ast.MessageLiteralNode); ok {
		return -1, interp.reporter.HandleErrorf(interp.nodeInfo(val).Start(), "%s: default value cannot be a message", scope)
	}
	mc := &messageContext{
		res:         interp.file,
		file:        interp.file.Proto(),
		elementName: fqn,
		elementType: descriptorType(fld),
		option:      opt,
	}
	var v interface{}
	if fld.GetType() == descriptorpb.FieldDescriptorProto_TYPE_ENUM {
		ed := interp.file.ResolveEnumType(protoreflect.FullName(fld.GetTypeName()))
		ev, err := interp.enumFieldValue(mc, ed, val)
		if err != nil {
			return -1, interp.reporter.HandleError(err)
		}
		v = string(ev.Name())
	} else {
		v, err = interp.scalarFieldValue(mc, fld.GetType(), val)
		if err != nil {
			return -1, interp.reporter.HandleError(err)
		}
	}
	if str, ok := v.(string); ok {
		fld.DefaultValue = proto.String(str)
	} else if b, ok := v.([]byte); ok {
		fld.DefaultValue = proto.String(encodeDefaultBytes(b))
	} else {
		var flt float64
		var ok bool
		if flt, ok = v.(float64); !ok {
			var flt32 float32
			if flt32, ok = v.(float32); ok {
				flt = float64(flt32)
			}
		}
		if ok {
			if math.IsInf(flt, 1) {
				fld.DefaultValue = proto.String("inf")
			} else if ok && math.IsInf(flt, -1) {
				fld.DefaultValue = proto.String("-inf")
			} else if ok && math.IsNaN(flt) {
				fld.DefaultValue = proto.String("nan")
			} else {
				fld.DefaultValue = proto.String(fmt.Sprintf("%v", v))
			}
		} else {
			fld.DefaultValue = proto.String(fmt.Sprintf("%v", v))
		}
	}
	return found, nil
}

func encodeDefaultBytes(b []byte) string {
	var buf bytes.Buffer
	internal.WriteEscapedBytes(&buf, b)
	return buf.String()
}

func (interp *interpreter) interpretEnumOptions(fqn string, ed *descriptorpb.EnumDescriptorProto) error {
	opts := ed.GetOptions()
	if opts != nil {
		if len(opts.UninterpretedOption) > 0 {
			if remain, err := interp.interpretOptions(fqn, ed, opts, opts.UninterpretedOption); err != nil {
				return err
			} else {
				opts.UninterpretedOption = remain
			}
		}
	}
	for _, evd := range ed.GetValue() {
		evdFqn := fqn + "." + evd.GetName()
		opts := evd.GetOptions()
		if len(opts.GetUninterpretedOption()) > 0 {
			if remain, err := interp.interpretOptions(evdFqn, evd, opts, opts.UninterpretedOption); err != nil {
				return err
			} else {
				opts.UninterpretedOption = remain
			}
		}
	}
	return nil
}

func (interp *interpreter) interpretOptions(fqn string, element, opts proto.Message, uninterpreted []*descriptorpb.UninterpretedOption) ([]*descriptorpb.UninterpretedOption, error) {
	optsFqn := string(opts.ProtoReflect().Descriptor().FullName())
	var msg protoreflect.Message
	// see if the parse included an override copy for these options
	if md := interp.file.ResolveMessageType(protoreflect.FullName(optsFqn)); md != nil {
		dm := newDynamic(md)
		if err := cloneInto(dm, opts, nil); err != nil {
			node := interp.file.Node(element)
			return nil, interp.reporter.HandleError(reporter.Error(interp.nodeInfo(node).Start(), err))
		}
		msg = dm
	} else {
		msg = proto.Clone(opts).ProtoReflect()
	}

	mc := &messageContext{res: interp.file, file: interp.file.Proto(), elementName: fqn, elementType: descriptorType(element)}
	var remain []*descriptorpb.UninterpretedOption
	for _, uo := range uninterpreted {
		node := interp.file.OptionNode(uo)
		if !uo.Name[0].GetIsExtension() && uo.Name[0].GetNamePart() == "uninterpreted_option" {
			if interp.lenient {
				remain = append(remain, uo)
				continue
			}
			// uninterpreted_option might be found reflectively, but is not actually valid for use
			if err := interp.reporter.HandleErrorf(interp.nodeInfo(node.GetName()).Start(), "%vinvalid option 'uninterpreted_option'", mc); err != nil {
				return nil, err
			}
		}
		mc.option = uo
		path, err := interp.interpretField(mc, element, msg, uo, 0, nil)
		if err != nil {
			if interp.lenient {
				remain = append(remain, uo)
				continue
			}
			return nil, err
		}
		if optn, ok := node.(*ast.OptionNode); ok {
			interp.index[optn] = path
		}
	}

	if interp.lenient {
		// If we're lenient, then we don't want to clobber the passed in message
		// and leave it partially populated. So we convert into a copy first
		optsClone := opts.ProtoReflect().New().Interface()
		if err := cloneInto(optsClone, msg.Interface(), interp.resolver); err != nil {
			// TODO: do this in a more granular way, so we can convert individual
			// fields and leave bad ones uninterpreted instead of skipping all of
			// the work we've done so far.
			return uninterpreted, nil
		}
		// conversion from dynamic message above worked, so now
		// it is safe to overwrite the passed in message
		proto.Reset(opts)
		proto.Merge(opts, optsClone)

		return remain, nil
	}

	if err := validateRecursive(msg, ""); err != nil {
		node := interp.file.Node(element)
		if err := interp.reporter.HandleErrorf(interp.nodeInfo(node).Start(), "error in %s options: %v", descriptorType(element), err); err != nil {
			return nil, err
		}
	}

	// now try to convert into the passed in message and fail if not successful
	if err := cloneInto(opts, msg.Interface(), interp.resolver); err != nil {
		node := interp.file.Node(element)
		return nil, interp.reporter.HandleError(reporter.Error(interp.nodeInfo(node).Start(), err))
	}

	return nil, nil
}

func cloneInto(dest proto.Message, src proto.Message, res linker.Resolver) error {
	if dest.ProtoReflect().Descriptor() == src.ProtoReflect().Descriptor() {
		proto.Reset(dest)
		proto.Merge(dest, src)
		if err := proto.CheckInitialized(dest); err != nil {
			return err
		}
		return nil
	}

	// If descriptors are not the same, we could have field descriptors in src that
	// don't match the ones in dest. There's no easy/sane way to handle that. So we
	// just marshal to bytes and back to do this
	data, err := proto.Marshal(src)
	if err != nil {
		return err
	}
	return proto.UnmarshalOptions{Resolver: res}.Unmarshal(data, dest)
}

func newDynamic(md protoreflect.MessageDescriptor) *deterministicDynamic {
	return &deterministicDynamic{Message: dynamicpb.NewMessage(md)}
}

type deterministicDynamic struct {
	*dynamicpb.Message
}

// ProtoReflect implements the protoreflect.ProtoMessage interface.
func (d *deterministicDynamic) ProtoReflect() protoreflect.Message {
	return d
}

func (d *deterministicDynamic) Interface() protoreflect.ProtoMessage {
	return d
}

func (d *deterministicDynamic) Range(f func(protoreflect.FieldDescriptor, protoreflect.Value) bool) {
	var fields []protoreflect.FieldDescriptor
	d.Message.Range(func(fd protoreflect.FieldDescriptor, _ protoreflect.Value) bool {
		fields = append(fields, fd)
		return true
	})
	// simple sort for deterministic marshaling
	sort.Slice(fields, func(i, j int) bool {
		return fields[i].Number() < fields[j].Number()
	})
	for _, fld := range fields {
		if !f(fld, d.Get(fld)) {
			return
		}
	}
}

func (d *deterministicDynamic) Get(fd protoreflect.FieldDescriptor) protoreflect.Value {
	v := d.Message.Get(fd)
	if v.IsValid() && fd.IsMap() {
		mp := v.Map()
		if _, ok := mp.(*deterministicMap); !ok {
			return protoreflect.ValueOfMap(&deterministicMap{Map: v.Map(), keyKind: fd.MapKey().Kind()})
		}
	}
	return v
}

type deterministicMap struct {
	protoreflect.Map
	keyKind protoreflect.Kind
}

func (m *deterministicMap) Range(f func(protoreflect.MapKey, protoreflect.Value) bool) {
	var keys []protoreflect.MapKey
	m.Map.Range(func(k protoreflect.MapKey, _ protoreflect.Value) bool {
		keys = append(keys, k)
		return true
	})
	sort.Slice(keys, func(i, j int) bool {
		switch m.keyKind {
		case protoreflect.BoolKind:
			return !keys[i].Bool() && keys[j].Bool()
		case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind,
			protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
			return keys[i].Int() < keys[j].Int()
		case protoreflect.Uint32Kind, protoreflect.Fixed32Kind,
			protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
			return keys[i].Uint() < keys[j].Uint()
		case protoreflect.StringKind:
			return keys[i].String() < keys[j].String()
		default:
			panic("invalid kind: " + m.keyKind.String())
		}
	})
	for _, key := range keys {
		if !f(key, m.Map.Get(key)) {
			break
		}
	}
}

func validateRecursive(msg protoreflect.Message, prefix string) error {
	flds := msg.Descriptor().Fields()
	var missingFields []string
	for i := 0; i < flds.Len(); i++ {
		fld := flds.Get(i)
		if fld.Cardinality() == protoreflect.Required && !msg.Has(fld) {
			missingFields = append(missingFields, fmt.Sprintf("%s%s", prefix, fld.Name()))
		}
	}
	if len(missingFields) > 0 {
		return fmt.Errorf("some required fields missing: %v", strings.Join(missingFields, ", "))
	}

	var err error
	msg.Range(func(fld protoreflect.FieldDescriptor, val protoreflect.Value) bool {
		if fld.IsMap() {
			md := fld.MapValue().Message()
			if md != nil {
				val.Map().Range(func(k protoreflect.MapKey, v protoreflect.Value) bool {
					chprefix := fmt.Sprintf("%s%s[%v].", prefix, fieldName(fld), k)
					err = validateRecursive(v.Message(), chprefix)
					if err != nil {
						return false
					}
					return true
				})
				if err != nil {
					return false
				}
			}
		} else {
			md := fld.Message()
			if md != nil {
				if fld.IsList() {
					sl := val.List()
					for i := 0; i < sl.Len(); i++ {
						v := sl.Get(i)
						chprefix := fmt.Sprintf("%s%s[%d].", prefix, fieldName(fld), i)
						err = validateRecursive(v.Message(), chprefix)
						if err != nil {
							return false
						}
					}
				} else {
					chprefix := fmt.Sprintf("%s%s.", prefix, fieldName(fld))
					err = validateRecursive(val.Message(), chprefix)
					if err != nil {
						return false
					}
				}
			}
		}
		return true
	})
	return err
}

func (interp *interpreter) interpretField(mc *messageContext, element proto.Message, msg protoreflect.Message, opt *descriptorpb.UninterpretedOption, nameIndex int, pathPrefix []int32) (path []int32, err error) {
	var fld protoreflect.FieldDescriptor
	nm := opt.GetName()[nameIndex]
	node := interp.file.OptionNamePartNode(nm)
	if nm.GetIsExtension() {
		extName := nm.GetNamePart()
		if extName[0] == '.' {
			extName = extName[1:] /* skip leading dot */
		}
		fld = interp.file.ResolveExtension(protoreflect.FullName(extName))
		if fld == nil {
			return nil, interp.reporter.HandleErrorf(interp.nodeInfo(node).Start(),
				"%vunrecognized extension %s of %s",
				mc, extName, msg.Descriptor().FullName())
		}
		if fld.ContainingMessage().FullName() != msg.Descriptor().FullName() {
			return nil, interp.reporter.HandleErrorf(interp.nodeInfo(node).Start(),
				"%vextension %s should extend %s but instead extends %s",
				mc, extName, msg.Descriptor().FullName(), fld.ContainingMessage().FullName())
		}
	} else {
		fld = msg.Descriptor().Fields().ByName(protoreflect.Name(nm.GetNamePart()))
		if fld == nil {
			return nil, interp.reporter.HandleErrorf(interp.nodeInfo(node).Start(),
				"%vfield %s of %s does not exist",
				mc, nm.GetNamePart(), msg.Descriptor().FullName())
		}
	}

	path = append(pathPrefix, int32(fld.Number()))

	if len(opt.GetName()) > nameIndex+1 {
		nextnm := opt.GetName()[nameIndex+1]
		nextnode := interp.file.OptionNamePartNode(nextnm)
		k := fld.Kind()
		if k != protoreflect.MessageKind && k != protoreflect.GroupKind {
			return nil, interp.reporter.HandleErrorf(interp.nodeInfo(nextnode).Start(),
				"%vcannot set field %s because %s is not a message",
				mc, nextnm.GetNamePart(), nm.GetNamePart())
		}
		if fld.Cardinality() == protoreflect.Repeated {
			return nil, interp.reporter.HandleErrorf(interp.nodeInfo(nextnode).Start(),
				"%vcannot set field %s because %s is repeated (must use an aggregate)",
				mc, nextnm.GetNamePart(), nm.GetNamePart())
		}
		var fdm protoreflect.Message
		if msg.Has(fld) {
			v := msg.Mutable(fld)
			fdm = v.Message()
		} else {
			fdm = newDynamic(fld.Message())
			msg.Set(fld, protoreflect.ValueOfMessage(fdm))
		}
		// recurse to set next part of name
		return interp.interpretField(mc, element, fdm, opt, nameIndex+1, path)
	}

	optNode := interp.file.OptionNode(opt)
	if err := interp.setOptionField(mc, msg, fld, node, optNode.GetValue()); err != nil {
		return nil, interp.reporter.HandleError(err)
	}
	if fld.IsMap() {
		path = append(path, int32(msg.Get(fld).Map().Len())-1)
	} else if fld.IsList() {
		path = append(path, int32(msg.Get(fld).List().Len())-1)
	}
	return path, nil
}

func (interp *interpreter) setOptionField(mc *messageContext, msg protoreflect.Message, fld protoreflect.FieldDescriptor, name ast.Node, val ast.ValueNode) error {
	v := val.Value()
	if sl, ok := v.([]ast.ValueNode); ok {
		// handle slices a little differently than the others
		if fld.Cardinality() != protoreflect.Repeated {
			return reporter.Errorf(interp.nodeInfo(val).Start(), "%vvalue is an array but field is not repeated", mc)
		}
		origPath := mc.optAggPath
		defer func() {
			mc.optAggPath = origPath
		}()
		for index, item := range sl {
			mc.optAggPath = fmt.Sprintf("%s[%d]", origPath, index)
			value, err := interp.fieldValue(mc, fld, item)
			if err != nil {
				return err
			}
			if fld.IsMap() {
				entry := value.Message()
				key := entry.Get(fld.MapKey()).MapKey()
				val := entry.Get(fld.MapValue())
				if dm, ok := val.Interface().(*dynamicpb.Message); ok && (dm == nil || !dm.IsValid()) {
					val = protoreflect.ValueOfMessage(newDynamic(fld.MapValue().Message()))
				}
				msg.Mutable(fld).Map().Set(key, val)
			} else {
				msg.Mutable(fld).List().Append(value)
			}
		}
		return nil
	}

	value, err := interp.fieldValue(mc, fld, val)
	if err != nil {
		return err
	}

	if ood := fld.ContainingOneof(); ood != nil {
		existingFld := msg.WhichOneof(ood)
		if existingFld != nil && existingFld.Number() != fld.Number() {
			return reporter.Errorf(interp.nodeInfo(name).Start(), "%voneof %q already has field %q set", mc, ood.Name(), fieldName(existingFld))
		}
	}

	if fld.IsMap() {
		entry := value.Message()
		key := entry.Get(fld.MapKey()).MapKey()
		val := entry.Get(fld.MapValue())
		if dm, ok := val.Interface().(*dynamicpb.Message); ok && (dm == nil || !dm.IsValid()) {
			val = protoreflect.ValueOfMessage(newDynamic(fld.MapValue().Message()))
		}
		msg.Mutable(fld).Map().Set(key, val)
	} else if fld.IsList() {
		msg.Mutable(fld).List().Append(value)
	} else {
		if msg.Has(fld) {
			return reporter.Errorf(interp.nodeInfo(name).Start(), "%vnon-repeated option field %s already set", mc, fieldName(fld))
		}
		msg.Set(fld, value)
	}

	return nil
}

type messageContext struct {
	res         parser.Result
	file        *descriptorpb.FileDescriptorProto
	elementType string
	elementName string
	option      *descriptorpb.UninterpretedOption
	optAggPath  string
}

func (c *messageContext) String() string {
	var ctx bytes.Buffer
	if c.elementType != "file" {
		_, _ = fmt.Fprintf(&ctx, "%s %s: ", c.elementType, c.elementName)
	}
	if c.option != nil && c.option.Name != nil {
		ctx.WriteString("option ")
		writeOptionName(&ctx, c.option.Name)
		if c.res.AST() == nil {
			// if we have no source position info, try to provide as much context
			// as possible (if nodes != nil, we don't need this because any errors
			// will actually have file and line numbers)
			if c.optAggPath != "" {
				_, _ = fmt.Fprintf(&ctx, " at %s", c.optAggPath)
			}
		}
		ctx.WriteString(": ")
	}
	return ctx.String()
}

func writeOptionName(buf *bytes.Buffer, parts []*descriptorpb.UninterpretedOption_NamePart) {
	first := true
	for _, p := range parts {
		if first {
			first = false
		} else {
			buf.WriteByte('.')
		}
		nm := p.GetNamePart()
		if nm[0] == '.' {
			// skip leading dot
			nm = nm[1:]
		}
		if p.GetIsExtension() {
			buf.WriteByte('(')
			buf.WriteString(nm)
			buf.WriteByte(')')
		} else {
			buf.WriteString(nm)
		}
	}
}

func fieldName(fld protoreflect.FieldDescriptor) string {
	if fld.IsExtension() {
		return fmt.Sprintf("(%s)", fld.FullName())
	} else {
		return string(fld.Name())
	}
}

func valueKind(val interface{}) string {
	switch val := val.(type) {
	case ast.Identifier:
		return "identifier"
	case bool:
		return "bool"
	case int64:
		if val < 0 {
			return "negative integer"
		}
		return "integer"
	case uint64:
		return "integer"
	case float64:
		return "double"
	case string, []byte:
		return "string"
	case []*ast.MessageFieldNode:
		return "message"
	case []ast.ValueNode:
		return "array"
	default:
		return fmt.Sprintf("%T", val)
	}
}

func (interp *interpreter) fieldValue(mc *messageContext, fld protoreflect.FieldDescriptor, val ast.ValueNode) (protoreflect.Value, error) {
	k := fld.Kind()
	switch k {
	case protoreflect.EnumKind:
		evd, err := interp.enumFieldValue(mc, fld.Enum(), val)
		if err != nil {
			return protoreflect.Value{}, err
		}
		return protoreflect.ValueOfEnum(evd.Number()), nil

	case protoreflect.MessageKind, protoreflect.GroupKind:
		v := val.Value()
		if aggs, ok := v.([]*ast.MessageFieldNode); ok {
			fmd := fld.Message()
			fdm := newDynamic(fmd)
			origPath := mc.optAggPath
			defer func() {
				mc.optAggPath = origPath
			}()
			for _, a := range aggs {
				if origPath == "" {
					mc.optAggPath = a.Name.Value()
				} else {
					mc.optAggPath = origPath + "." + a.Name.Value()
				}
				var ffld protoreflect.FieldDescriptor
				if a.Name.IsExtension() {
					n := string(a.Name.Name.AsIdentifier())
					ffld = interp.file.ResolveExtension(protoreflect.FullName(n))
					if ffld == nil {
						// may need to qualify with package name
						pkg := mc.file.GetPackage()
						if pkg != "" {
							ffld = interp.file.ResolveExtension(protoreflect.FullName(pkg + "." + n))
						}
					}
				} else {
					ffld = fmd.Fields().ByName(protoreflect.Name(a.Name.Value()))
					// Groups are indicated in the text format by the group name (which is
					// camel-case), NOT the field name (which is lower-case).
					// ...but only regular fields, not extensions that are groups...
					if ffld != nil && ffld.Kind() == protoreflect.GroupKind && ffld.Message().Name() != protoreflect.Name(a.Name.Value()) {
						// this is kind of silly to fail here, but this mimics protoc behavior
						return protoreflect.Value{}, reporter.Errorf(interp.nodeInfo(val).Start(), "%vfield %s not found (did you mean the group named %s?)", mc, a.Name.Value(), ffld.Message().Name())
					}
					if ffld == nil {
						// could be a group name
						for i := 0; i < fmd.Fields().Len(); i++ {
							fd := fmd.Fields().Get(i)
							if fd.Kind() == protoreflect.GroupKind && fd.Message().Name() == protoreflect.Name(a.Name.Value()) {
								// found it!
								ffld = fd
								break
							}
						}
					}
				}
				if ffld == nil {
					return protoreflect.Value{}, reporter.Errorf(interp.nodeInfo(val).Start(), "%vfield %s not found", mc, string(a.Name.Name.AsIdentifier()))
				}
				if err := interp.setOptionField(mc, fdm, ffld, a.Name, a.Val); err != nil {
					return protoreflect.Value{}, err
				}
			}
			return protoreflect.ValueOfMessage(fdm), nil
		}
		return protoreflect.Value{}, reporter.Errorf(interp.nodeInfo(val).Start(), "%vexpecting message, got %s", mc, valueKind(v))

	default:
		v, err := interp.scalarFieldValue(mc, descriptorpb.FieldDescriptorProto_Type(k), val)
		if err != nil {
			return protoreflect.Value{}, err
		}
		return protoreflect.ValueOf(v), nil
	}
}

func (interp *interpreter) enumFieldValue(mc *messageContext, ed protoreflect.EnumDescriptor, val ast.ValueNode) (protoreflect.EnumValueDescriptor, error) {
	v := val.Value()
	if id, ok := v.(ast.Identifier); ok {
		ev := ed.Values().ByName(protoreflect.Name(id))
		if ev == nil {
			return nil, reporter.Errorf(interp.nodeInfo(val).Start(), "%venum %s has no value named %s", mc, ed.FullName(), id)
		}
		return ev, nil
	}
	return nil, reporter.Errorf(interp.nodeInfo(val).Start(), "%vexpecting enum, got %s", mc, valueKind(v))
}

func (interp *interpreter) scalarFieldValue(mc *messageContext, fldType descriptorpb.FieldDescriptorProto_Type, val ast.ValueNode) (interface{}, error) {
	v := val.Value()
	switch fldType {
	case descriptorpb.FieldDescriptorProto_TYPE_BOOL:
		if b, ok := v.(bool); ok {
			return b, nil
		}
		return nil, reporter.Errorf(interp.nodeInfo(val).Start(), "%vexpecting bool, got %s", mc, valueKind(v))
	case descriptorpb.FieldDescriptorProto_TYPE_BYTES:
		if str, ok := v.(string); ok {
			return []byte(str), nil
		}
		return nil, reporter.Errorf(interp.nodeInfo(val).Start(), "%vexpecting bytes, got %s", mc, valueKind(v))
	case descriptorpb.FieldDescriptorProto_TYPE_STRING:
		if str, ok := v.(string); ok {
			return str, nil
		}
		return nil, reporter.Errorf(interp.nodeInfo(val).Start(), "%vexpecting string, got %s", mc, valueKind(v))
	case descriptorpb.FieldDescriptorProto_TYPE_INT32, descriptorpb.FieldDescriptorProto_TYPE_SINT32, descriptorpb.FieldDescriptorProto_TYPE_SFIXED32:
		if i, ok := v.(int64); ok {
			if i > math.MaxInt32 || i < math.MinInt32 {
				return nil, reporter.Errorf(interp.nodeInfo(val).Start(), "%vvalue %d is out of range for int32", mc, i)
			}
			return int32(i), nil
		}
		if ui, ok := v.(uint64); ok {
			if ui > math.MaxInt32 {
				return nil, reporter.Errorf(interp.nodeInfo(val).Start(), "%vvalue %d is out of range for int32", mc, ui)
			}
			return int32(ui), nil
		}
		return nil, reporter.Errorf(interp.nodeInfo(val).Start(), "%vexpecting int32, got %s", mc, valueKind(v))
	case descriptorpb.FieldDescriptorProto_TYPE_UINT32, descriptorpb.FieldDescriptorProto_TYPE_FIXED32:
		if i, ok := v.(int64); ok {
			if i > math.MaxUint32 || i < 0 {
				return nil, reporter.Errorf(interp.nodeInfo(val).Start(), "%vvalue %d is out of range for uint32", mc, i)
			}
			return uint32(i), nil
		}
		if ui, ok := v.(uint64); ok {
			if ui > math.MaxUint32 {
				return nil, reporter.Errorf(interp.nodeInfo(val).Start(), "%vvalue %d is out of range for uint32", mc, ui)
			}
			return uint32(ui), nil
		}
		return nil, reporter.Errorf(interp.nodeInfo(val).Start(), "%vexpecting uint32, got %s", mc, valueKind(v))
	case descriptorpb.FieldDescriptorProto_TYPE_INT64, descriptorpb.FieldDescriptorProto_TYPE_SINT64, descriptorpb.FieldDescriptorProto_TYPE_SFIXED64:
		if i, ok := v.(int64); ok {
			return i, nil
		}
		if ui, ok := v.(uint64); ok {
			if ui > math.MaxInt64 {
				return nil, reporter.Errorf(interp.nodeInfo(val).Start(), "%vvalue %d is out of range for int64", mc, ui)
			}
			return int64(ui), nil
		}
		return nil, reporter.Errorf(interp.nodeInfo(val).Start(), "%vexpecting int64, got %s", mc, valueKind(v))
	case descriptorpb.FieldDescriptorProto_TYPE_UINT64, descriptorpb.FieldDescriptorProto_TYPE_FIXED64:
		if i, ok := v.(int64); ok {
			if i < 0 {
				return nil, reporter.Errorf(interp.nodeInfo(val).Start(), "%vvalue %d is out of range for uint64", mc, i)
			}
			return uint64(i), nil
		}
		if ui, ok := v.(uint64); ok {
			return ui, nil
		}
		return nil, reporter.Errorf(interp.nodeInfo(val).Start(), "%vexpecting uint64, got %s", mc, valueKind(v))
	case descriptorpb.FieldDescriptorProto_TYPE_DOUBLE:
		if d, ok := v.(float64); ok {
			return d, nil
		}
		if i, ok := v.(int64); ok {
			return float64(i), nil
		}
		if u, ok := v.(uint64); ok {
			return float64(u), nil
		}
		return nil, reporter.Errorf(interp.nodeInfo(val).Start(), "%vexpecting double, got %s", mc, valueKind(v))
	case descriptorpb.FieldDescriptorProto_TYPE_FLOAT:
		if d, ok := v.(float64); ok {
			if (d > math.MaxFloat32 || d < -math.MaxFloat32) && !math.IsInf(d, 1) && !math.IsInf(d, -1) && !math.IsNaN(d) {
				return nil, reporter.Errorf(interp.nodeInfo(val).Start(), "%vvalue %f is out of range for float", mc, d)
			}
			return float32(d), nil
		}
		if i, ok := v.(int64); ok {
			return float32(i), nil
		}
		if u, ok := v.(uint64); ok {
			return float32(u), nil
		}
		return nil, reporter.Errorf(interp.nodeInfo(val).Start(), "%vexpecting float, got %s", mc, valueKind(v))
	default:
		return nil, reporter.Errorf(interp.nodeInfo(val).Start(), "%vunrecognized field type: %s", mc, fldType)
	}
}

func descriptorType(m proto.Message) string {
	switch m := m.(type) {
	case *descriptorpb.DescriptorProto:
		return "message"
	case *descriptorpb.DescriptorProto_ExtensionRange:
		return "extension range"
	case *descriptorpb.FieldDescriptorProto:
		if m.GetExtendee() == "" {
			return "field"
		} else {
			return "extension"
		}
	case *descriptorpb.EnumDescriptorProto:
		return "enum"
	case *descriptorpb.EnumValueDescriptorProto:
		return "enum value"
	case *descriptorpb.ServiceDescriptorProto:
		return "service"
	case *descriptorpb.MethodDescriptorProto:
		return "method"
	case *descriptorpb.FileDescriptorProto:
		return "file"
	default:
		// shouldn't be possible
		return fmt.Sprintf("%T", m)
	}
}
