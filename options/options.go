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

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"

	"github.com/bufbuild/protocompile/ast"
	"github.com/bufbuild/protocompile/internal"
	"github.com/bufbuild/protocompile/linker"
	"github.com/bufbuild/protocompile/parser"
	"github.com/bufbuild/protocompile/reporter"
)

// Index is a mapping of AST nodes that define options to a corresponding path
// into the containing file descriptor. The path is a sequence of field tags
// and indexes that define a traversal path from the root (the file descriptor)
// to the resolved option field.
type Index map[*ast.OptionNode][]int32

type interpreter struct {
	file      file
	resolver  linker.Resolver
	container optionsContainer
	lenient   bool
	reporter  *reporter.Handler
	index     Index
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
	interp.container, _ = file.(optionsContainer)
	if f, ok := file.(linker.File); ok {
		interp.resolver = linker.ResolverFromFile(f)
	}

	fd := file.FileDescriptorProto()
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
	if len(opts.GetUninterpretedOption()) == 0 {
		return nil
	}
	uo := opts.UninterpretedOption
	scope := fmt.Sprintf("field %s", fqn)

	// process json_name pseudo-option
	index, err := internal.FindOption(interp.file, interp.reporter, scope, uo, "json_name")
	if err != nil && !interp.lenient {
		return err
	}
	if index >= 0 {
		opt := uo[index]
		optNode := interp.file.OptionNode(opt)
		if fld.GetExtendee() != "" {
			return interp.reporter.HandleErrorf(interp.nodeInfo(optNode.GetName()).Start(), "%s: option json_name is not allowed on extensions", scope)
		}
		// attribute source code info
		if on, ok := optNode.(*ast.OptionNode); ok {
			interp.index[on] = []int32{-1, internal.FieldJSONNameTag}
		}
		uo = internal.RemoveOption(uo, index)
		if opt.StringValue == nil {
			return interp.reporter.HandleErrorf(interp.nodeInfo(optNode.GetValue()).Start(), "%s: expecting string value for json_name option", scope)
		}
		fld.JsonName = proto.String(string(opt.StringValue))
	}

	// and process default pseudo-option
	if index, err := interp.processDefaultOption(scope, fqn, fld, uo); err != nil && !interp.lenient {
		return err
	} else if index >= 0 {
		// attribute source code info
		optNode := interp.file.OptionNode(uo[index])
		if on, ok := optNode.(*ast.OptionNode); ok {
			interp.index[on] = []int32{-1, internal.FieldDefaultTag}
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
		file:        interp.file.FileDescriptorProto(),
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
		v, err = interp.scalarFieldValue(mc, fld.GetType(), val, false)
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

// interpretedOption represents the result of interpreting an option.
// This includes metadata that allows the option to be serialized to
// bytes in a way that is deterministic and can preserve the structure
// of the source (the way the options are de-structured and the order in
// which options appear).
type interpretedOption struct {
	unknown    bool
	pathPrefix []int32
	interpretedField
}

func (o *interpretedOption) path() []int32 {
	path := append(o.pathPrefix, o.number)
	if o.repeated {
		path = append(path, o.index)
	}
	return path
}

func (o *interpretedOption) appendOptionBytes(b []byte) ([]byte, error) {
	return o.appendOptionBytesWithPath(b, o.pathPrefix)
}

func (o *interpretedOption) appendOptionBytesWithPath(b []byte, path []int32) ([]byte, error) {
	if len(path) == 0 {
		return appendOptionBytesSingle(b, &o.interpretedField)
	}
	// NB: if we add functions to compute sizes of the options first, we could
	// allocate precisely sized slice up front, which would be more efficient than
	// repeated creation/growing/concatenation.
	enclosed, err := o.appendOptionBytesWithPath(nil, path[1:])
	if err != nil {
		return nil, err
	}
	b = protowire.AppendTag(b, protowire.Number(path[0]), protowire.BytesType)
	return protowire.AppendBytes(b, enclosed), nil
}

// interpretedField represents a field in an options message that is the
// result of interpreting an option. This is used for the option value
// itself as well as for subfields when an option value is a message
// literal.
type interpretedField struct {
	// field number
	number int32
	// index of this element inside a repeated field; only set if repeated == true
	index int32
	// true if this is a repeated field
	repeated bool
	// true if this is a repeated field that stores scalar values in packed form
	packed bool
	// the field's kind
	kind protoreflect.Kind

	value interpretedFieldValue
}

// interpretedFieldValue is a wrapper around protoreflect.Value that
// includes extra metadata.
type interpretedFieldValue struct {
	// the field value
	val protoreflect.Value
	// if true, this value is a list of values, not a singular value
	isList bool
	// non-nil for singular message values
	msgVal []*interpretedField
	// non-nil for non-empty lists of message values
	msgListVal [][]*interpretedField
}

func appendOptionBytes(b []byte, flds []*interpretedField) ([]byte, error) {
	// protoc emits messages sorted by field number
	if len(flds) > 1 {
		sort.SliceStable(flds, func(i, j int) bool {
			return flds[i].number < flds[j].number
		})
	}

	for i := 0; i < len(flds); i++ {
		f := flds[i]
		if f.packed && canPack(f.kind) {
			// for packed repeated numeric fields, all runs of values are merged into one packed list
			num := f.number
			j := i
			for j < len(flds) && flds[j].number == num {
				j++
			}
			// now flds[i:j] is the range of contiguous fields for the same field number
			enclosed, err := appendOptionBytesPacked(nil, f.kind, flds[i:j])
			if err != nil {
				return nil, err
			}
			b = protowire.AppendTag(b, protowire.Number(f.number), protowire.BytesType)
			b = protowire.AppendBytes(b, enclosed)
			// skip over the other subsequent fields we just serialized
			i = j - 1
		} else if f.value.isList {
			// if not packed, then emit one value at a time
			single := *f
			single.value.isList = false
			single.value.msgListVal = nil
			l := f.value.val.List()
			for i := 0; i < l.Len(); i++ {
				single.value.val = l.Get(i)
				if f.kind == protoreflect.MessageKind || f.kind == protoreflect.GroupKind {
					single.value.msgVal = f.value.msgListVal[i]
				}
				var err error
				b, err = appendOptionBytesSingle(b, &single)
				if err != nil {
					return nil, err
				}
			}
		} else {
			// simple singular value
			var err error
			b, err = appendOptionBytesSingle(b, f)
			if err != nil {
				return nil, err
			}
		}
	}

	return b, nil
}

func canPack(k protoreflect.Kind) bool {
	switch k {
	case protoreflect.MessageKind, protoreflect.GroupKind, protoreflect.StringKind, protoreflect.BytesKind:
		return false
	default:
		return true
	}
}

func appendOptionBytesPacked(b []byte, k protoreflect.Kind, flds []*interpretedField) ([]byte, error) {
	for i := range flds {
		val := flds[i].value
		if val.isList {
			l := val.val.List()
			var err error
			b, err = appendNumericValueBytesPacked(b, k, l)
			if err != nil {
				return nil, err
			}
		} else {
			var err error
			b, err = appendNumericValueBytes(b, k, val.val)
			if err != nil {
				return nil, err
			}
		}
	}
	return b, nil
}

func appendOptionBytesSingle(b []byte, f *interpretedField) ([]byte, error) {
	num := protowire.Number(f.number)
	switch f.kind {
	case protoreflect.MessageKind:
		enclosed, err := appendOptionBytes(nil, f.value.msgVal)
		if err != nil {
			return nil, err
		}
		b = protowire.AppendTag(b, num, protowire.BytesType)
		return protowire.AppendBytes(b, enclosed), nil

	case protoreflect.GroupKind:
		b = protowire.AppendTag(b, num, protowire.StartGroupType)
		var err error
		b, err = appendOptionBytes(b, f.value.msgVal)
		if err != nil {
			return nil, err
		}
		return protowire.AppendTag(b, num, protowire.EndGroupType), nil

	case protoreflect.StringKind:
		b = protowire.AppendTag(b, num, protowire.BytesType)
		return protowire.AppendString(b, f.value.val.String()), nil

	case protoreflect.BytesKind:
		b = protowire.AppendTag(b, num, protowire.BytesType)
		return protowire.AppendBytes(b, f.value.val.Bytes()), nil

	case protoreflect.Int32Kind, protoreflect.Int64Kind, protoreflect.Uint32Kind, protoreflect.Uint64Kind,
		protoreflect.Sint32Kind, protoreflect.Sint64Kind, protoreflect.EnumKind, protoreflect.BoolKind:
		b = protowire.AppendTag(b, num, protowire.VarintType)
		return appendNumericValueBytes(b, f.kind, f.value.val)

	case protoreflect.Fixed32Kind, protoreflect.Sfixed32Kind, protoreflect.FloatKind:
		b = protowire.AppendTag(b, num, protowire.Fixed32Type)
		return appendNumericValueBytes(b, f.kind, f.value.val)

	case protoreflect.Fixed64Kind, protoreflect.Sfixed64Kind, protoreflect.DoubleKind:
		b = protowire.AppendTag(b, num, protowire.Fixed64Type)
		return appendNumericValueBytes(b, f.kind, f.value.val)

	default:
		return nil, fmt.Errorf("unknown field kind: %v", f.kind)
	}
}

func appendNumericValueBytesPacked(b []byte, k protoreflect.Kind, l protoreflect.List) ([]byte, error) {
	for i := 0; i < l.Len(); i++ {
		var err error
		b, err = appendNumericValueBytes(b, k, l.Get(i))
		if err != nil {
			return nil, err
		}
	}
	return b, nil
}

func appendNumericValueBytes(b []byte, k protoreflect.Kind, v protoreflect.Value) ([]byte, error) {
	switch k {
	case protoreflect.Int32Kind, protoreflect.Int64Kind:
		return protowire.AppendVarint(b, uint64(v.Int())), nil
	case protoreflect.Uint32Kind, protoreflect.Uint64Kind:
		return protowire.AppendVarint(b, v.Uint()), nil
	case protoreflect.Sint32Kind, protoreflect.Sint64Kind:
		return protowire.AppendVarint(b, protowire.EncodeZigZag(v.Int())), nil
	case protoreflect.Fixed32Kind:
		return protowire.AppendFixed32(b, uint32(v.Uint())), nil
	case protoreflect.Fixed64Kind:
		return protowire.AppendFixed64(b, v.Uint()), nil
	case protoreflect.Sfixed32Kind:
		return protowire.AppendFixed32(b, uint32(v.Int())), nil
	case protoreflect.Sfixed64Kind:
		return protowire.AppendFixed64(b, uint64(v.Int())), nil
	case protoreflect.FloatKind:
		return protowire.AppendFixed32(b, math.Float32bits(float32(v.Float()))), nil
	case protoreflect.DoubleKind:
		return protowire.AppendFixed64(b, math.Float64bits(v.Float())), nil
	case protoreflect.BoolKind:
		return protowire.AppendVarint(b, protowire.EncodeBool(v.Bool())), nil
	case protoreflect.EnumKind:
		return protowire.AppendVarint(b, uint64(v.Enum())), nil
	default:
		return nil, fmt.Errorf("unknown field kind: %v", k)
	}
}

// optionsContainer may be optionally implemented by a linker.Result. It is
// not part of the linker.Result interface as it is meant only for internal use.
// This allows the option interpreter step to store extra metadata about the
// serialized structure of options.
type optionsContainer interface {
	// AddOptionBytes adds the given pre-serialized option bytes to a file,
	// associated with the given options message. The type of the given message
	// should be an options message, for example *descriptorpb.MessageOptions.
	// This value should be part of the message hierarchy whose root is the
	// *descriptorpb.FileDescriptorProto that corresponds to this result.
	AddOptionBytes(pm proto.Message, opts []byte)
}

// interpretOptions processes the options in uninterpreted, which are interpreted as fields
// of the given opts message. On success, it will usually return nil, nil. But if the current
// operation is lenient, it may return a non-nil slice of uninterpreted options on success.
// In such a case, the returned value is the remaining slice of options which could not be
// interpreted.
func (interp *interpreter) interpretOptions(fqn string, element, opts proto.Message, uninterpreted []*descriptorpb.UninterpretedOption) ([]*descriptorpb.UninterpretedOption, error) {
	optsDesc := opts.ProtoReflect().Descriptor()
	optsFqn := string(optsDesc.FullName())
	var msg protoreflect.Message
	// see if the parse included an override copy for these options
	if md := interp.file.ResolveMessageType(protoreflect.FullName(optsFqn)); md != nil {
		dm := dynamicpb.NewMessage(md)
		if err := cloneInto(dm, opts, nil); err != nil {
			node := interp.file.Node(element)
			return nil, interp.reporter.HandleError(reporter.Error(interp.nodeInfo(node).Start(), err))
		}
		msg = dm
	} else {
		msg = proto.Clone(opts).ProtoReflect()
	}

	mc := &messageContext{res: interp.file, file: interp.file.FileDescriptorProto(), elementName: fqn, elementType: descriptorType(element)}
	var remain []*descriptorpb.UninterpretedOption
	var results []*interpretedOption
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
		res, err := interp.interpretField(mc, msg, uo, 0, nil)
		if err != nil {
			if interp.lenient {
				remain = append(remain, uo)
				continue
			}
			return nil, err
		}
		res.unknown = !isKnownField(optsDesc, res)
		results = append(results, res)
		if optn, ok := node.(*ast.OptionNode); ok {
			interp.index[optn] = res.path()
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

		if interp.container != nil {
			b, err := interp.toOptionBytes(mc, results)
			if err != nil {
				return nil, err
			}
			interp.container.AddOptionBytes(opts, b)
		}

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
	if interp.container != nil {
		b, err := interp.toOptionBytes(mc, results)
		if err != nil {
			return nil, err
		}
		interp.container.AddOptionBytes(opts, b)
	}

	return nil, nil
}

func isKnownField(desc protoreflect.MessageDescriptor, opt *interpretedOption) bool {
	var num int32
	if len(opt.pathPrefix) > 0 {
		num = opt.pathPrefix[0]
	} else {
		num = opt.number
	}
	return desc.Fields().ByNumber(protoreflect.FieldNumber(num)) != nil
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

func (interp *interpreter) toOptionBytes(mc *messageContext, results []*interpretedOption) ([]byte, error) {
	// protoc emits non-custom options in tag order and then
	// the rest are emitted in the order they are defined in source
	sort.SliceStable(results, func(i, j int) bool {
		if !results[i].unknown && results[j].unknown {
			return true
		}
		if !results[i].unknown && !results[j].unknown {
			return results[i].number < results[j].number
		}
		return false
	})
	var b []byte
	for _, res := range results {
		var err error
		b, err = res.appendOptionBytes(b)
		if err != nil {
			if _, ok := err.(reporter.ErrorWithPos); !ok {
				pos := ast.SourcePos{Filename: interp.file.AST().Name()}
				err = reporter.Errorf(pos, "%sfailed to encode options: %w", mc, err)
			}
			if err := interp.reporter.HandleError(err); err != nil {
				return nil, err
			}
		}
	}
	return b, nil
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

// interpretField interprets the option described by opt, as a field inside the given msg. This
// interprets components of the option name starting at nameIndex. When nameIndex == 0, then
// msg must be an options message. For nameIndex > 0, msg is a nested message inside of the
// options message. The given pathPrefix is the path (sequence of field numbers and indices
// with a FileDescriptorProto as the start) up to but not including the given nameIndex.
func (interp *interpreter) interpretField(mc *messageContext, msg protoreflect.Message, opt *descriptorpb.UninterpretedOption, nameIndex int, pathPrefix []int32) (*interpretedOption, error) {
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
			if ood := fld.ContainingOneof(); ood != nil {
				existingFld := msg.WhichOneof(ood)
				if existingFld != nil && existingFld.Number() != fld.Number() {
					return nil, interp.reporter.HandleErrorf(interp.nodeInfo(node).Start(),
						"%voneof %q already has field %q set",
						mc, ood.Name(), fieldName(existingFld))
				}
			}
			fdm = dynamicpb.NewMessage(fld.Message())
			msg.Set(fld, protoreflect.ValueOfMessage(fdm))
		}
		// recurse to set next part of name
		return interp.interpretField(mc, fdm, opt, nameIndex+1, append(pathPrefix, int32(fld.Number())))
	}

	optNode := interp.file.OptionNode(opt)
	val, err := interp.setOptionField(mc, msg, fld, node, optNode.GetValue())
	if err != nil {
		return nil, interp.reporter.HandleError(err)
	}
	var index int32
	if fld.IsMap() {
		index = int32(msg.Get(fld).Map().Len()) - 1
	} else if fld.IsList() {
		index = int32(msg.Get(fld).List().Len()) - 1
	}
	return &interpretedOption{
		pathPrefix: pathPrefix,
		interpretedField: interpretedField{
			number:   int32(fld.Number()),
			index:    index,
			kind:     fld.Kind(),
			repeated: fld.Cardinality() == protoreflect.Repeated,
			value:    val,
			// NB: don't set packed here in a top-level option
			// (only values in message literals will be serialized
			// in packed format)
		},
	}, nil
}

// setOptionField sets the value for field fld in the given message msg to the value represented
// by val. The given name is the AST node that corresponds to the name of fld. On success, it
// returns additional metadata about the field that was set.
func (interp *interpreter) setOptionField(mc *messageContext, msg protoreflect.Message, fld protoreflect.FieldDescriptor, name ast.Node, val ast.ValueNode) (interpretedFieldValue, error) {
	v := val.Value()
	if sl, ok := v.([]ast.ValueNode); ok {
		// handle slices a little differently than the others
		if fld.Cardinality() != protoreflect.Repeated {
			return interpretedFieldValue{}, reporter.Errorf(interp.nodeInfo(val).Start(), "%vvalue is an array but field is not repeated", mc)
		}
		origPath := mc.optAggPath
		defer func() {
			mc.optAggPath = origPath
		}()
		var resVal listValue
		var resMsgVals [][]*interpretedField
		for index, item := range sl {
			mc.optAggPath = fmt.Sprintf("%s[%d]", origPath, index)
			value, err := interp.fieldValue(mc, fld, item)
			if err != nil {
				return interpretedFieldValue{}, err
			}
			if fld.IsMap() {
				setMapEntry(msg, fld, &value)
			} else {
				msg.Mutable(fld).List().Append(value.val)
			}
			resVal = append(resVal, value.val)
			if value.msgVal != nil {
				resMsgVals = append(resMsgVals, value.msgVal)
			}
		}
		return interpretedFieldValue{
			isList:     true,
			val:        protoreflect.ValueOfList(&resVal),
			msgListVal: resMsgVals,
		}, nil
	}

	value, err := interp.fieldValue(mc, fld, val)
	if err != nil {
		return interpretedFieldValue{}, err
	}

	if ood := fld.ContainingOneof(); ood != nil {
		existingFld := msg.WhichOneof(ood)
		if existingFld != nil && existingFld.Number() != fld.Number() {
			return interpretedFieldValue{}, reporter.Errorf(interp.nodeInfo(name).Start(), "%voneof %q already has field %q set", mc, ood.Name(), fieldName(existingFld))
		}
	}

	if fld.IsMap() {
		setMapEntry(msg, fld, &value)
	} else if fld.IsList() {
		msg.Mutable(fld).List().Append(value.val)
	} else {
		if msg.Has(fld) {
			return interpretedFieldValue{}, reporter.Errorf(interp.nodeInfo(name).Start(), "%vnon-repeated option field %s already set", mc, fieldName(fld))
		}
		msg.Set(fld, value.val)
	}

	return value, nil
}

func setMapEntry(msg protoreflect.Message, fld protoreflect.FieldDescriptor, value *interpretedFieldValue) {
	entry := value.val.Message()
	keyFld, valFld := fld.MapKey(), fld.MapValue()
	// if an entry is missing a key or value, we add in an explicit
	// zero value to msgVals to match protoc (which also odds these
	// in even if not present in source)
	if !entry.Has(keyFld) {
		// put key before value
		value.msgVal = append(append(([]*interpretedField)(nil), zeroValue(keyFld)), value.msgVal...)
	}
	if !entry.Has(valFld) {
		value.msgVal = append(value.msgVal, zeroValue(valFld))
	}
	key := entry.Get(keyFld)
	val := entry.Get(valFld)
	if dm, ok := val.Interface().(*dynamicpb.Message); ok && (dm == nil || !dm.IsValid()) {
		val = protoreflect.ValueOfMessage(dynamicpb.NewMessage(valFld.Message()))
	}
	m := msg.Mutable(fld).Map()
	// TODO: error if key is already present
	m.Set(key.MapKey(), val)
}

// zeroValue returns the zero value for the field types as a *interpretedField.
// The given fld must NOT be a repeated field.
func zeroValue(fld protoreflect.FieldDescriptor) *interpretedField {
	var val protoreflect.Value
	var msgVal []*interpretedField
	switch fld.Kind() {
	case protoreflect.MessageKind, protoreflect.GroupKind:
		// needs to be non-nil, but empty
		msgVal = []*interpretedField{}
		msg := dynamicpb.NewMessage(fld.Message())
		val = protoreflect.ValueOfMessage(msg)
	case protoreflect.EnumKind:
		val = protoreflect.ValueOfEnum(0)
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		val = protoreflect.ValueOfInt32(0)
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		val = protoreflect.ValueOfUint32(0)
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		val = protoreflect.ValueOfInt64(0)
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		val = protoreflect.ValueOfUint64(0)
	case protoreflect.BoolKind:
		val = protoreflect.ValueOfBool(false)
	case protoreflect.FloatKind:
		val = protoreflect.ValueOfFloat32(0)
	case protoreflect.DoubleKind:
		val = protoreflect.ValueOfFloat64(0)
	case protoreflect.BytesKind:
		val = protoreflect.ValueOfBytes(nil)
	case protoreflect.StringKind:
		val = protoreflect.ValueOfString("")
	}
	return &interpretedField{
		number: int32(fld.Number()),
		kind:   fld.Kind(),
		value: interpretedFieldValue{
			val:    val,
			msgVal: msgVal,
		},
	}
}

type listValue []protoreflect.Value

var _ protoreflect.List = (*listValue)(nil)

func (l listValue) Len() int {
	return len(l)
}

func (l listValue) Get(i int) protoreflect.Value {
	return l[i]
}

func (l listValue) Set(i int, value protoreflect.Value) {
	l[i] = value
}

func (l *listValue) Append(value protoreflect.Value) {
	*l = append(*l, value)
}

func (l listValue) AppendMutable() protoreflect.Value {
	panic("AppendMutable not supported")
}

func (l *listValue) Truncate(i int) {
	*l = (*l)[:i]
}

func (l listValue) NewElement() protoreflect.Value {
	panic("NewElement not supported")
}

func (l listValue) IsValid() bool {
	return true
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

// fieldValue computes a compile-time value (constant or list or message literal) for the given
// AST node val. The value in val must be assignable to the field fld.
func (interp *interpreter) fieldValue(mc *messageContext, fld protoreflect.FieldDescriptor, val ast.ValueNode) (interpretedFieldValue, error) {
	k := fld.Kind()
	switch k {
	case protoreflect.EnumKind:
		evd, err := interp.enumFieldValue(mc, fld.Enum(), val)
		if err != nil {
			return interpretedFieldValue{}, err
		}
		return interpretedFieldValue{val: protoreflect.ValueOfEnum(evd.Number())}, nil

	case protoreflect.MessageKind, protoreflect.GroupKind:
		v := val.Value()
		if aggs, ok := v.([]*ast.MessageFieldNode); ok {
			fmd := fld.Message()
			fdm := dynamicpb.NewMessage(fmd)
			origPath := mc.optAggPath
			defer func() {
				mc.optAggPath = origPath
			}()
			// NB: we don't want to leave this nil, even if the
			// message is literal, because that indicates to
			// caller that the result is not a message
			flds := make([]*interpretedField, 0, len(aggs))
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
						return interpretedFieldValue{}, reporter.Errorf(interp.nodeInfo(a.Name).Start(), "%vfield %s not found (did you mean the group named %s?)", mc, a.Name.Value(), ffld.Message().Name())
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
					return interpretedFieldValue{}, reporter.Errorf(interp.nodeInfo(a.Name).Start(), "%vfield %s not found", mc, string(a.Name.Name.AsIdentifier()))
				}
				if a.Sep == nil && ffld.Message() == nil {
					// If there is no separator, the field type should be a message.
					// Otherwise it is an error in the text format.
					return interpretedFieldValue{}, reporter.Errorf(interp.nodeInfo(a.Val).Start(), "syntax error: unexpected value, expecting ':'")
				}
				res, err := interp.setOptionField(mc, fdm, ffld, a.Name, a.Val)
				if err != nil {
					return interpretedFieldValue{}, err
				}
				flds = append(flds, &interpretedField{
					number:   int32(ffld.Number()),
					kind:     ffld.Kind(),
					repeated: ffld.Cardinality() == protoreflect.Repeated,
					packed:   ffld.IsPacked(),
					value:    res,
					// NB: no need to set index here, inside message literal
					// (it is only used for top-level options, for emitting
					// source code info)
				})
			}
			return interpretedFieldValue{
				val:    protoreflect.ValueOfMessage(fdm),
				msgVal: flds,
			}, nil
		}
		return interpretedFieldValue{}, reporter.Errorf(interp.nodeInfo(val).Start(), "%vexpecting message, got %s", mc, valueKind(v))

	default:
		v, err := interp.scalarFieldValue(mc, descriptorpb.FieldDescriptorProto_Type(k), val, false)
		if err != nil {
			return interpretedFieldValue{}, err
		}
		return interpretedFieldValue{val: protoreflect.ValueOf(v)}, nil
	}
}

// enumFieldValue resolves the given AST node val as an enum value descriptor. If the given
// value is not a valid identifier, an error is returned instead.
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

// scalarFieldValue resolves the given AST node val as a value whose type is assignable to a
// field with the given fldType.
func (interp *interpreter) scalarFieldValue(mc *messageContext, fldType descriptorpb.FieldDescriptorProto_Type, val ast.ValueNode, insideMsgLiteral bool) (interface{}, error) {
	v := val.Value()
	switch fldType {
	case descriptorpb.FieldDescriptorProto_TYPE_BOOL:
		if b, ok := v.(bool); ok {
			return b, nil
		}
		if id, ok := v.(ast.Identifier); ok {
			if insideMsgLiteral {
				// inside a message literal, values use the protobuf text format,
				// which is lenient in that it accepts "t" and "f" or "True" and "False"
				switch id {
				case "t", "true", "True":
					return true, nil
				case "f", "false", "False":
					return false, nil
				}
			} else {
				// options with simple scalar values (no message literal) are stricter
				switch id {
				case "true":
					return true, nil
				case "false":
					return false, nil
				}
			}
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
		if id, ok := v.(ast.Identifier); ok {
			switch id {
			case "inf":
				return math.Inf(1), nil
			case "nan":
				return math.NaN(), nil
			}
		}
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
		if id, ok := v.(ast.Identifier); ok {
			switch id {
			case "inf":
				return float32(math.Inf(1)), nil
			case "nan":
				return float32(math.NaN()), nil
			}
		}
		if d, ok := v.(float64); ok {
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
