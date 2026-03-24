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

// Package sourceinfo contains the logic for computing source code info for a
// file descriptor.
//
// The inputs to the computation are an AST for a file as well as the index of
// interpreted options for that file.
package sourceinfo

import (
	"bytes"
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/bufbuild/protocompile/ast"
	"github.com/bufbuild/protocompile/internal"
	"github.com/bufbuild/protocompile/internal/tags"
)

// OptionIndex is a mapping of AST nodes that define options to corresponding
// paths into the containing file descriptor. The path is a sequence of field
// tags and indexes that define a traversal path from the root (the file
// descriptor) to the resolved option field. The info also includes similar
// information about child elements, for options whose values are composite
// (like a list or message literal).
type OptionIndex map[*ast.OptionNode]*OptionSourceInfo

// OptionSourceInfo describes the source info path for an option value and
// contains information about the value's descendants in the AST.
type OptionSourceInfo struct {
	// The source info path to this element. If this element represents a
	// declaration with an array-literal value, the last element of the
	// path is the index of the first item in the array.
	//
	// This path is relative to the options message. So the first element
	// is a field number of the options message.
	//
	// If the first element is negative, it indicates the number of path
	// components to remove from the path to the relevant options. This is
	// used for field pseudo-options, so that the path indicates a field on
	// the descriptor, which is a parent of the options message (since that
	// is how the pseudo-options are actually stored).
	Path []int32
	// Children can be an *ArrayLiteralSourceInfo, a *MessageLiteralSourceInfo,
	// or nil, depending on whether the option's value is an
	// [*ast.ArrayLiteralNode], an [*ast.MessageLiteralNode], or neither.
	// For [*ast.ArrayLiteralNode] values, this is only populated if the
	// value is a non-empty array of messages. (Empty arrays and arrays
	// of scalar values do not need any additional info.)
	Children OptionChildrenSourceInfo
}

// OptionChildrenSourceInfo represents source info paths for child elements of
// an option value.
type OptionChildrenSourceInfo interface {
	isChildSourceInfo()
}

// ArrayLiteralSourceInfo represents source info paths for the child
// elements of an [*ast.ArrayLiteralNode]. This value is only useful for
// non-empty array literals that contain messages.
type ArrayLiteralSourceInfo struct {
	Elements []OptionSourceInfo
}

func (*ArrayLiteralSourceInfo) isChildSourceInfo() {}

// MessageLiteralSourceInfo represents source info paths for the child
// elements of an [*ast.MessageLiteralNode].
type MessageLiteralSourceInfo struct {
	Fields map[*ast.MessageFieldNode]*OptionSourceInfo
}

func (*MessageLiteralSourceInfo) isChildSourceInfo() {}

// GenerateSourceInfo generates source code info for the given AST. If the given
// opts is present, it can generate source code info for interpreted options.
// Otherwise, any options in the AST will get source code info as uninterpreted
// options.
func GenerateSourceInfo(file *ast.FileNode, opts OptionIndex, genOpts ...GenerateOption) *descriptorpb.SourceCodeInfo {
	if file == nil {
		return nil
	}
	sci := sourceCodeInfo{file: file, commentsUsed: map[ast.SourcePos]struct{}{}}
	for _, sourceInfoOpt := range genOpts {
		sourceInfoOpt.apply(&sci)
	}
	generateSourceInfoForFile(opts, &sci, file)
	return &descriptorpb.SourceCodeInfo{Location: sci.locs}
}

// GenerateOption represents an option for how source code info is generated.
type GenerateOption interface {
	apply(*sourceCodeInfo)
}

// WithExtraComments will result in source code info that contains extra comments.
// By default, comments are only generated for full declarations. Inline comments
// around elements of a declaration are not included in source code info. This option
// changes that behavior so that as many comments as possible are described in the
// source code info.
func WithExtraComments() GenerateOption {
	return extraCommentsOption{}
}

// WithExtraOptionLocations will result in source code info that contains extra
// locations to describe elements inside of a message literal. By default, option
// values are treated as opaque, so the only locations included are for the entire
// option value. But with this option, paths to the various fields set inside a
// message literal will also have locations. This makes it possible for usages of
// the source code info to report precise locations for specific fields inside the
// value.
func WithExtraOptionLocations() GenerateOption {
	return extraOptionLocationsOption{}
}

type extraCommentsOption struct{}

func (e extraCommentsOption) apply(info *sourceCodeInfo) {
	info.extraComments = true
}

type extraOptionLocationsOption struct{}

func (e extraOptionLocationsOption) apply(info *sourceCodeInfo) {
	info.extraOptionLocs = true
}

func generateSourceInfoForFile(opts OptionIndex, sci *sourceCodeInfo, file *ast.FileNode) {
	path := make([]int32, 0, 16)

	sci.newLocWithoutComments(file, nil)

	if file.Syntax != nil {
		sci.newLocWithComments(file.Syntax, append(path, tags.File_Syntax))
	}
	if file.Edition != nil {
		// Despite editions having its own field, protoc behavior sets the path in source code
		// info as [tag.File_Syntax] and this is vaguely outlined in descriptor.proto
		// https://github.com/protocolbuffers/protobuf/blob/22e1e6bd90aa8dc35f8cc28b5d7fc03858060f0b/src/google/protobuf/descriptor.proto#L137-L144
		sci.newLocWithComments(file.Edition, append(path, tags.File_Syntax))
	}

	var depIndex, pubDepIndex, weakDepIndex, optIndex, msgIndex, enumIndex, extendIndex, svcIndex int32

	for _, child := range file.Decls {
		switch child := child.(type) {
		case *ast.ImportNode:
			sci.newLocWithComments(child, append(path, tags.File_Dependency, depIndex))
			depIndex++
			if child.Public != nil {
				sci.newLoc(child.Public, append(path, tags.File_PublicDependency, pubDepIndex))
				pubDepIndex++
			} else if child.Weak != nil {
				sci.newLoc(child.Weak, append(path, tags.File_WeakDependency, weakDepIndex))
				weakDepIndex++
			}
		case *ast.PackageNode:
			sci.newLocWithComments(child, append(path, tags.File_Package))
		case *ast.OptionNode:
			generateSourceCodeInfoForOption(opts, sci, child, false, &optIndex, append(path, tags.File_Options))
		case *ast.MessageNode:
			generateSourceCodeInfoForMessage(opts, sci, child, nil, append(path, tags.File_MessageType, msgIndex))
			msgIndex++
		case *ast.EnumNode:
			generateSourceCodeInfoForEnum(opts, sci, child, append(path, tags.File_EnumType, enumIndex))
			enumIndex++
		case *ast.ExtendNode:
			extsPath := append(path, tags.File_Extension) //nolint:gocritic // intentionally creating new slice var
			// we clone the path here so that append can't mutate extsPath, since they may share storage
			msgsPath := append(internal.ClonePath(path), tags.File_MessageType)
			generateSourceCodeInfoForExtensions(opts, sci, child, &extendIndex, &msgIndex, extsPath, msgsPath)
		case *ast.ServiceNode:
			generateSourceCodeInfoForService(opts, sci, child, append(path, tags.File_Service, svcIndex))
			svcIndex++
		}
	}
}

func generateSourceCodeInfoForOption(opts OptionIndex, sci *sourceCodeInfo, n *ast.OptionNode, compact bool, uninterpIndex *int32, path []int32) {
	if !compact {
		sci.newLocWithoutComments(n, path)
	}
	optInfo := opts[n]
	if optInfo != nil {
		fullPath := combinePathsForOption(path, optInfo.Path)
		if compact {
			sci.newLoc(n, fullPath)
		} else {
			sci.newLocWithComments(n, fullPath)
		}
		if sci.extraOptionLocs {
			generateSourceInfoForOptionChildren(sci, n.Val, path, fullPath, optInfo.Children)
		}
		return
	}

	// it's an uninterpreted option
	optPath := path
	optPath = append(optPath, tags.UninterpretedOption, *uninterpIndex)
	*uninterpIndex++
	sci.newLoc(n, optPath)
	var valTag int32
	switch n.Val.(type) {
	case ast.IdentValueNode:
		valTag = tags.UninterpretedOption_IdentifierValue
	case *ast.NegativeIntLiteralNode:
		valTag = tags.UninterpretedOption_NegativeIntValue
	case ast.IntValueNode:
		valTag = tags.UninterpretedOption_PositiveIntValue
	case ast.FloatValueNode:
		valTag = tags.UninterpretedOption_DoubleValue
	case ast.StringValueNode:
		valTag = tags.UninterpretedOption_StringValue
	case *ast.MessageLiteralNode:
		valTag = tags.UninterpretedOption_AggregateValue
	}
	if valTag != 0 {
		sci.newLoc(n.Val, append(optPath, valTag))
	}
	for j, nn := range n.Name.Parts {
		optNmPath := optPath
		optNmPath = append(optNmPath, tags.UninterpretedOption_Name, int32(j))
		sci.newLoc(nn, optNmPath)
		sci.newLoc(nn.Name, append(optNmPath, tags.UninterpretedOption_NamePart_NamePart))
	}
}

func combinePathsForOption(prefix, optionPath []int32) []int32 {
	fullPath := make([]int32, len(prefix), len(prefix)+len(optionPath))
	copy(fullPath, prefix)
	if optionPath[0] == -1 {
		// used by "default" and "json_name" field pseudo-options
		// to attribute path to parent element (since those are
		// stored directly on the descriptor, not its options)
		optionPath = optionPath[1:]
		fullPath = fullPath[:len(prefix)-1]
	}
	return append(fullPath, optionPath...)
}

func generateSourceInfoForOptionChildren(sci *sourceCodeInfo, n ast.ValueNode, pathPrefix, path []int32, childInfo OptionChildrenSourceInfo) {
	switch childInfo := childInfo.(type) {
	case *ArrayLiteralSourceInfo:
		if arrayLiteral, ok := n.(*ast.ArrayLiteralNode); ok {
			for i, val := range arrayLiteral.Elements {
				elementInfo := childInfo.Elements[i]
				fullPath := combinePathsForOption(pathPrefix, elementInfo.Path)
				sci.newLoc(val, fullPath)
				generateSourceInfoForOptionChildren(sci, val, pathPrefix, fullPath, elementInfo.Children)
			}
		}
	case *MessageLiteralSourceInfo:
		if msgLiteral, ok := n.(*ast.MessageLiteralNode); ok {
			for _, fieldNode := range msgLiteral.Elements {
				fieldInfo, ok := childInfo.Fields[fieldNode]
				if !ok {
					continue
				}
				fullPath := combinePathsForOption(pathPrefix, fieldInfo.Path)
				locationNode := ast.Node(fieldNode)
				if fieldNode.Name.IsAnyTypeReference() && fullPath[len(fullPath)-1] == tags.Any_Value {
					// This is a special expanded Any. So also insert a location
					// for the type URL field.
					typeURLPath := make([]int32, len(fullPath))
					copy(typeURLPath, fullPath)
					typeURLPath[len(typeURLPath)-1] = tags.Any_TypeUrl
					sci.newLoc(fieldNode.Name, fullPath)
					// And create the next location so it's just the value,
					// not the full field definition.
					locationNode = fieldNode.Val
				}
				_, isArrayLiteral := fieldNode.Val.(*ast.ArrayLiteralNode)
				if !isArrayLiteral {
					// We don't include this with an array literal since the path
					// is to the first element of the array. If we added it here,
					// it would be redundant with the child info we add next, and
					// it wouldn't be entirely correct since it only indicates the
					// index of the first element in the array (and not the others).
					sci.newLoc(locationNode, fullPath)
				}
				generateSourceInfoForOptionChildren(sci, fieldNode.Val, pathPrefix, fullPath, fieldInfo.Children)
			}
		}
	case nil:
		if arrayLiteral, ok := n.(*ast.ArrayLiteralNode); ok {
			// an array literal without child source info is an array of scalars
			for i, val := range arrayLiteral.Elements {
				// last element of path is starting index for array literal
				elementPath := append(([]int32)(nil), path...)
				elementPath[len(elementPath)-1] += int32(i)
				sci.newLoc(val, elementPath)
			}
		}
	}
}

func generateSourceCodeInfoForMessage(opts OptionIndex, sci *sourceCodeInfo, n ast.MessageDeclNode, fieldPath []int32, path []int32) {
	var openBrace ast.Node

	var decls []ast.MessageElement
	switch n := n.(type) {
	case *ast.MessageNode:
		openBrace = n.OpenBrace
		decls = n.Decls
	case *ast.SyntheticGroupMessageNode:
		openBrace = n.OpenBrace
		decls = n.Decls
	case *ast.SyntheticMapEntryNode:
		sci.newLoc(n, path)
		// map entry so nothing else to do
		return
	}
	sci.newBlockLocWithComments(n, openBrace, path)

	sci.newLoc(n.MessageName(), append(path, tags.Message_Name))
	// matching protoc, which emits the corresponding field type name (for group fields)
	// right after the source location for the group message name
	if fieldPath != nil {
		sci.newLoc(n.MessageName(), append(fieldPath, tags.Field_TypeName))
	}

	var optIndex, fieldIndex, oneofIndex, extendIndex, nestedMsgIndex int32
	var nestedEnumIndex, extRangeIndex, reservedRangeIndex, reservedNameIndex int32
	for _, child := range decls {
		switch child := child.(type) {
		case *ast.OptionNode:
			generateSourceCodeInfoForOption(opts, sci, child, false, &optIndex, append(path, tags.Message_Options))
		case *ast.FieldNode:
			generateSourceCodeInfoForField(opts, sci, child, append(path, tags.Message_Field, fieldIndex))
			fieldIndex++
		case *ast.GroupNode:
			fldPath := append(path, tags.Message_Field, fieldIndex) //nolint:gocritic // intentionally creating new slice var
			generateSourceCodeInfoForField(opts, sci, child, fldPath)
			fieldIndex++
			// we clone the path here so that append can't mutate fldPath, since they may share storage
			msgPath := append(internal.ClonePath(path), tags.Message_NestedType, nestedMsgIndex)
			generateSourceCodeInfoForMessage(opts, sci, child.AsMessage(), fldPath, msgPath)
			nestedMsgIndex++
		case *ast.MapFieldNode:
			generateSourceCodeInfoForField(opts, sci, child, append(path, tags.Message_Field, fieldIndex))
			fieldIndex++
			nestedMsgIndex++
		case *ast.OneofNode:
			fldsPath := append(path, tags.Message_Field) //nolint:gocritic // intentionally creating new slice var
			// we clone the path here and below so that append ops can't mutate
			// fldPath or msgsPath, since they may otherwise share storage
			msgsPath := append(internal.ClonePath(path), tags.Message_NestedType)
			ooPath := append(internal.ClonePath(path), tags.Message_OneofDecl, oneofIndex)
			generateSourceCodeInfoForOneof(opts, sci, child, &fieldIndex, &nestedMsgIndex, fldsPath, msgsPath, ooPath)
			oneofIndex++
		case *ast.MessageNode:
			generateSourceCodeInfoForMessage(opts, sci, child, nil, append(path, tags.Message_NestedType, nestedMsgIndex))
			nestedMsgIndex++
		case *ast.EnumNode:
			generateSourceCodeInfoForEnum(opts, sci, child, append(path, tags.Message_EnumType, nestedEnumIndex))
			nestedEnumIndex++
		case *ast.ExtendNode:
			extsPath := append(path, tags.Message_Extension) //nolint:gocritic // intentionally creating new slice var
			// we clone the path here so that append can't mutate extsPath, since they may share storage
			msgsPath := append(internal.ClonePath(path), tags.Message_NestedType)
			generateSourceCodeInfoForExtensions(opts, sci, child, &extendIndex, &nestedMsgIndex, extsPath, msgsPath)
		case *ast.ExtensionRangeNode:
			generateSourceCodeInfoForExtensionRanges(opts, sci, child, &extRangeIndex, append(path, tags.Message_ExtensionRange))
		case *ast.ReservedNode:
			if len(child.Names) > 0 {
				resPath := path
				resPath = append(resPath, tags.Message_ReservedName)
				sci.newLocWithComments(child, resPath)
				for _, rn := range child.Names {
					sci.newLoc(rn, append(resPath, reservedNameIndex))
					reservedNameIndex++
				}
			}
			// For editions, reserved names are identifiers.
			if len(child.Identifiers) > 0 {
				resPath := path
				resPath = append(resPath, tags.Message_ReservedName)
				sci.newLocWithComments(child, resPath)
				for _, rn := range child.Identifiers {
					sci.newLoc(rn, append(resPath, reservedNameIndex))
					reservedNameIndex++
				}
			}
			if len(child.Ranges) > 0 {
				resPath := path
				resPath = append(resPath, tags.Message_ReservedRange)
				sci.newLocWithComments(child, resPath)
				for _, rr := range child.Ranges {
					generateSourceCodeInfoForReservedRange(sci, rr, append(resPath, reservedRangeIndex))
					reservedRangeIndex++
				}
			}
		}
	}
}

func generateSourceCodeInfoForEnum(opts OptionIndex, sci *sourceCodeInfo, n *ast.EnumNode, path []int32) {
	sci.newBlockLocWithComments(n, n.OpenBrace, path)
	sci.newLoc(n.Name, append(path, tags.Enum_Name))

	var optIndex, valIndex, reservedNameIndex, reservedRangeIndex int32
	for _, child := range n.Decls {
		switch child := child.(type) {
		case *ast.OptionNode:
			generateSourceCodeInfoForOption(opts, sci, child, false, &optIndex, append(path, tags.Enum_Options))
		case *ast.EnumValueNode:
			generateSourceCodeInfoForEnumValue(opts, sci, child, append(path, tags.Enum_Value, valIndex))
			valIndex++
		case *ast.ReservedNode:
			if len(child.Names) > 0 {
				resPath := path
				resPath = append(resPath, tags.Enum_ReservedName)
				sci.newLocWithComments(child, resPath)
				for _, rn := range child.Names {
					sci.newLoc(rn, append(resPath, reservedNameIndex))
					reservedNameIndex++
				}
			}
			if len(child.Ranges) > 0 {
				resPath := path
				resPath = append(resPath, tags.Enum_ReservedRange)
				sci.newLocWithComments(child, resPath)
				for _, rr := range child.Ranges {
					generateSourceCodeInfoForReservedRange(sci, rr, append(resPath, reservedRangeIndex))
					reservedRangeIndex++
				}
			}
		}
	}
}

func generateSourceCodeInfoForEnumValue(opts OptionIndex, sci *sourceCodeInfo, n *ast.EnumValueNode, path []int32) {
	sci.newLocWithComments(n, path)
	sci.newLoc(n.Name, append(path, tags.EnumValue_Name))
	sci.newLoc(n.Number, append(path, tags.EnumValue_Number))

	// enum value options
	if n.Options != nil {
		optsPath := path
		optsPath = append(optsPath, tags.EnumValue_Options)
		sci.newLoc(n.Options, optsPath)
		var optIndex int32
		for _, opt := range n.Options.GetElements() {
			generateSourceCodeInfoForOption(opts, sci, opt, true, &optIndex, optsPath)
		}
	}
}

func generateSourceCodeInfoForReservedRange(sci *sourceCodeInfo, n *ast.RangeNode, path []int32) {
	sci.newLoc(n, path)
	sci.newLoc(n.StartVal, append(path, tags.Message_ReservedRange_Start))
	switch {
	case n.EndVal != nil:
		sci.newLoc(n.EndVal, append(path, tags.Message_ReservedRange_End))
	case n.Max != nil:
		sci.newLoc(n.Max, append(path, tags.Message_ReservedRange_End))
	default:
		sci.newLoc(n.StartVal, append(path, tags.Message_ReservedRange_End))
	}
}

func generateSourceCodeInfoForExtensions(opts OptionIndex, sci *sourceCodeInfo, n *ast.ExtendNode, extendIndex, msgIndex *int32, extendPath, msgPath []int32) {
	sci.newBlockLocWithComments(n, n.OpenBrace, extendPath)
	for _, decl := range n.Decls {
		switch decl := decl.(type) {
		case *ast.FieldNode:
			generateSourceCodeInfoForField(opts, sci, decl, append(extendPath, *extendIndex))
			*extendIndex++
		case *ast.GroupNode:
			fldPath := extendPath
			fldPath = append(fldPath, *extendIndex)
			generateSourceCodeInfoForField(opts, sci, decl, fldPath)
			*extendIndex++
			generateSourceCodeInfoForMessage(opts, sci, decl.AsMessage(), fldPath, append(msgPath, *msgIndex))
			*msgIndex++
		}
	}
}

func generateSourceCodeInfoForOneof(opts OptionIndex, sci *sourceCodeInfo, n *ast.OneofNode, fieldIndex, nestedMsgIndex *int32, fieldPath, nestedMsgPath, oneofPath []int32) {
	sci.newBlockLocWithComments(n, n.OpenBrace, oneofPath)
	sci.newLoc(n.Name, append(oneofPath, tags.Oneof_Name))

	var optIndex int32
	for _, child := range n.Decls {
		switch child := child.(type) {
		case *ast.OptionNode:
			generateSourceCodeInfoForOption(opts, sci, child, false, &optIndex, append(oneofPath, tags.Oneof_Options))
		case *ast.FieldNode:
			generateSourceCodeInfoForField(opts, sci, child, append(fieldPath, *fieldIndex))
			*fieldIndex++
		case *ast.GroupNode:
			fldPath := fieldPath
			fldPath = append(fldPath, *fieldIndex)
			generateSourceCodeInfoForField(opts, sci, child, fldPath)
			*fieldIndex++
			generateSourceCodeInfoForMessage(opts, sci, child.AsMessage(), fldPath, append(nestedMsgPath, *nestedMsgIndex))
			*nestedMsgIndex++
		}
	}
}

func generateSourceCodeInfoForField(opts OptionIndex, sci *sourceCodeInfo, n ast.FieldDeclNode, path []int32) {
	var fieldType string
	if f, ok := n.(*ast.FieldNode); ok {
		fieldType = string(f.FldType.AsIdentifier())
	}

	if n.GetGroupKeyword() != nil {
		// comments will appear on group message
		sci.newLocWithoutComments(n, path)
		if n.FieldExtendee() != nil {
			sci.newLoc(n.FieldExtendee(), append(path, tags.Field_Extendee))
		}
		if n.FieldLabel() != nil {
			// no comments here either (label is first token for group, so we want
			// to leave the comments to be associated with the group message instead)
			sci.newLocWithoutComments(n.FieldLabel(), append(path, tags.Field_Label))
		}
		sci.newLoc(n.FieldType(), append(path, tags.Field_Type))
		// let the name comments be attributed to the group name
		sci.newLocWithoutComments(n.FieldName(), append(path, tags.Field_Name))
	} else {
		sci.newLocWithComments(n, path)
		if n.FieldExtendee() != nil {
			sci.newLoc(n.FieldExtendee(), append(path, tags.Field_Extendee))
		}
		if n.FieldLabel() != nil {
			sci.newLoc(n.FieldLabel(), append(path, tags.Field_Label))
		}
		var t int32
		if _, isScalar := internal.FieldTypes[fieldType]; isScalar {
			t = tags.Field_Type
		} else {
			// this is a message or an enum, so attribute type location
			// to the type name field
			t = tags.Field_TypeName
		}
		sci.newLoc(n.FieldType(), append(path, t))
		sci.newLoc(n.FieldName(), append(path, tags.Field_Name))
	}
	sci.newLoc(n.FieldTag(), append(path, tags.Field_Number))

	if n.GetOptions() != nil {
		optsPath := path
		optsPath = append(optsPath, tags.Field_Options)
		sci.newLoc(n.GetOptions(), optsPath)
		var optIndex int32
		for _, opt := range n.GetOptions().GetElements() {
			generateSourceCodeInfoForOption(opts, sci, opt, true, &optIndex, optsPath)
		}
	}
}

func generateSourceCodeInfoForExtensionRanges(opts OptionIndex, sci *sourceCodeInfo, n *ast.ExtensionRangeNode, extRangeIndex *int32, path []int32) {
	sci.newLocWithComments(n, path)
	startExtRangeIndex := *extRangeIndex
	for _, child := range n.Ranges {
		path := append(path, *extRangeIndex)
		*extRangeIndex++
		sci.newLoc(child, path)
		sci.newLoc(child.StartVal, append(path, tags.Message_ExtensionRange_Start))
		switch {
		case child.EndVal != nil:
			sci.newLoc(child.EndVal, append(path, tags.Message_ExtensionRange_End))
		case child.Max != nil:
			sci.newLoc(child.Max, append(path, tags.Message_ExtensionRange_End))
		default:
			sci.newLoc(child.StartVal, append(path, tags.Message_ExtensionRange_End))
		}
	}
	// options for all ranges go after the start+end values
	for range n.Ranges {
		path := append(path, startExtRangeIndex)
		startExtRangeIndex++
		if n.Options != nil {
			optsPath := path
			optsPath = append(optsPath, tags.Message_ExtensionRange_Options)
			sci.newLoc(n.Options, optsPath)
			var optIndex int32
			for _, opt := range n.Options.GetElements() {
				generateSourceCodeInfoForOption(opts, sci, opt, true, &optIndex, optsPath)
			}
		}
	}
}

func generateSourceCodeInfoForService(opts OptionIndex, sci *sourceCodeInfo, n *ast.ServiceNode, path []int32) {
	sci.newBlockLocWithComments(n, n.OpenBrace, path)
	sci.newLoc(n.Name, append(path, tags.Service_Name))
	var optIndex, rpcIndex int32
	for _, child := range n.Decls {
		switch child := child.(type) {
		case *ast.OptionNode:
			generateSourceCodeInfoForOption(opts, sci, child, false, &optIndex, append(path, tags.Service_Options))
		case *ast.RPCNode:
			generateSourceCodeInfoForMethod(opts, sci, child, append(path, tags.Service_Method, rpcIndex))
			rpcIndex++
		}
	}
}

func generateSourceCodeInfoForMethod(opts OptionIndex, sci *sourceCodeInfo, n *ast.RPCNode, path []int32) {
	if n.OpenBrace != nil {
		sci.newBlockLocWithComments(n, n.OpenBrace, path)
	} else {
		sci.newLocWithComments(n, path)
	}
	sci.newLoc(n.Name, append(path, tags.Method_Name))
	if n.Input.Stream != nil {
		sci.newLoc(n.Input.Stream, append(path, tags.Method_ClientStreaming))
	}
	sci.newLoc(n.Input.MessageType, append(path, tags.Method_InputType))
	if n.Output.Stream != nil {
		sci.newLoc(n.Output.Stream, append(path, tags.Method_ServerStreaming))
	}
	sci.newLoc(n.Output.MessageType, append(path, tags.Method_OutputType))

	optsPath := path
	optsPath = append(optsPath, tags.Method_Options)
	var optIndex int32
	for _, decl := range n.Decls {
		if opt, ok := decl.(*ast.OptionNode); ok {
			generateSourceCodeInfoForOption(opts, sci, opt, false, &optIndex, optsPath)
		}
	}
}

type sourceCodeInfo struct {
	file            *ast.FileNode
	extraComments   bool
	extraOptionLocs bool
	locs            []*descriptorpb.SourceCodeInfo_Location
	commentsUsed    map[ast.SourcePos]struct{}
}

func (sci *sourceCodeInfo) newLocWithoutComments(n ast.Node, path []int32) {
	var start, end ast.SourcePos
	if n == sci.file {
		// For files, we don't want to consider trailing EOF token
		// as part of the span. We want the span to only include
		// actual lexical elements in the file (which also excludes
		// whitespace and comments).
		children := sci.file.Children()
		if len(children) > 0 && isEOF(children[len(children)-1]) {
			children = children[:len(children)-1]
		}
		if len(children) == 0 {
			start = ast.SourcePos{Filename: sci.file.Name(), Line: 1, Col: 1}
			end = start
		} else {
			start = sci.file.TokenInfo(n.Start()).Start()
			end = sci.file.TokenInfo(children[len(children)-1].End()).End()
		}
	} else {
		info := sci.file.NodeInfo(n)
		start, end = info.Start(), info.End()
	}
	sci.locs = append(sci.locs, &descriptorpb.SourceCodeInfo_Location{
		Path: internal.ClonePath(path),
		Span: makeSpan(start, end),
	})
}

func (sci *sourceCodeInfo) newLoc(n ast.Node, path []int32) {
	info := sci.file.NodeInfo(n)
	if !sci.extraComments {
		start, end := info.Start(), info.End()
		sci.locs = append(sci.locs, &descriptorpb.SourceCodeInfo_Location{
			Path: internal.ClonePath(path),
			Span: makeSpan(start, end),
		})
	} else {
		detachedComments, leadingComments := sci.getLeadingComments(n)
		trailingComments := sci.getTrailingComments(n)
		sci.newLocWithGivenComments(info, detachedComments, leadingComments, trailingComments, path)
	}
}

func isEOF(n ast.Node) bool {
	r, ok := n.(*ast.RuneNode)
	return ok && r.Rune == 0
}

func (sci *sourceCodeInfo) newBlockLocWithComments(n, openBrace ast.Node, path []int32) {
	// Block definitions use trailing comments after the open brace "{" as the
	// element's trailing comments. For example:
	//
	//    message Foo { // this is a trailing comment for a message
	//
	//    }             // not this
	//
	nodeInfo := sci.file.NodeInfo(n)
	detachedComments, leadingComments := sci.getLeadingComments(n)
	trailingComments := sci.getTrailingComments(openBrace)
	sci.newLocWithGivenComments(nodeInfo, detachedComments, leadingComments, trailingComments, path)
}

func (sci *sourceCodeInfo) newLocWithComments(n ast.Node, path []int32) {
	nodeInfo := sci.file.NodeInfo(n)
	detachedComments, leadingComments := sci.getLeadingComments(n)
	trailingComments := sci.getTrailingComments(n)
	sci.newLocWithGivenComments(nodeInfo, detachedComments, leadingComments, trailingComments, path)
}

func (sci *sourceCodeInfo) newLocWithGivenComments(nodeInfo ast.NodeInfo, detachedComments []comments, leadingComments comments, trailingComments comments, path []int32) {
	if (len(detachedComments) > 0 && sci.commentUsed(detachedComments[0])) ||
		(len(detachedComments) == 0 && sci.commentUsed(leadingComments)) {
		detachedComments = nil
		leadingComments = ast.EmptyComments
	}
	if sci.commentUsed(trailingComments) {
		trailingComments = ast.EmptyComments
	}

	var trail *string
	if trailingComments.Len() > 0 {
		trail = proto.String(sci.combineComments(trailingComments))
	}

	var lead *string
	if leadingComments.Len() > 0 {
		lead = proto.String(sci.combineComments(leadingComments))
	}

	detached := make([]string, len(detachedComments))
	for i, cmts := range detachedComments {
		detached[i] = sci.combineComments(cmts)
	}

	sci.locs = append(sci.locs, &descriptorpb.SourceCodeInfo_Location{
		LeadingDetachedComments: detached,
		LeadingComments:         lead,
		TrailingComments:        trail,
		Path:                    internal.ClonePath(path),
		Span:                    makeSpan(nodeInfo.Start(), nodeInfo.End()),
	})
}

type comments interface {
	Len() int
	Index(int) ast.Comment
}

type subComments struct {
	offs, n int
	c       ast.Comments
}

func (s subComments) Len() int {
	return s.n
}

func (s subComments) Index(i int) ast.Comment {
	if i < 0 || i >= s.n {
		panic(fmt.Errorf("runtime error: index out of range [%d] with length %d", i, s.n))
	}
	return s.c.Index(i + s.offs)
}

func (sci *sourceCodeInfo) getLeadingComments(n ast.Node) ([]comments, comments) {
	s := n.Start()
	info := sci.file.TokenInfo(s)
	var prevInfo ast.NodeInfo
	if prev, ok := sci.file.Tokens().Previous(s); ok {
		prevInfo = sci.file.TokenInfo(prev)
	}
	_, d, l := sci.attributeComments(prevInfo, info)
	return d, l
}

func (sci *sourceCodeInfo) getTrailingComments(n ast.Node) comments {
	e := n.End()
	next, ok := sci.file.Tokens().Next(e)
	if !ok {
		return ast.EmptyComments
	}
	info := sci.file.TokenInfo(e)
	nextInfo := sci.file.TokenInfo(next)
	t, _, _ := sci.attributeComments(info, nextInfo)
	return t
}

func (sci *sourceCodeInfo) attributeComments(prevInfo, info ast.NodeInfo) (t comments, d []comments, l comments) {
	detached := groupComments(info.LeadingComments())
	var trail comments
	if prevInfo.IsValid() {
		trail = comments(prevInfo.TrailingComments())
		if trail.Len() == 0 {
			trail, detached = sci.maybeDonate(prevInfo, info, detached)
		}
	} else {
		trail = ast.EmptyComments
	}
	detached, lead := sci.maybeAttach(prevInfo, info, trail.Len() > 0, detached)
	return trail, detached, lead
}

func (sci *sourceCodeInfo) maybeDonate(prevInfo ast.NodeInfo, info ast.NodeInfo, lead []comments) (t comments, l []comments) {
	if len(lead) == 0 {
		// nothing to donate
		return ast.EmptyComments, nil
	}
	firstCommentPos := lead[0].Index(0)
	if firstCommentPos.Start().Line > prevInfo.End().Line+1 {
		// first comment is detached from previous token, so can't be a trailing comment
		return ast.EmptyComments, lead
	}
	if len(lead) > 1 {
		// multiple groups? then donate first comment to previous token
		return lead[0], lead[1:]
	}
	// there is only one element in lead
	comment := lead[0]
	lastCommentPos := comment.Index(comment.Len() - 1)
	if lastCommentPos.End().Line < info.Start().Line-1 {
		// there is a blank line between the comments and subsequent token, so
		// we can donate the comment to previous token
		return comment, nil
	}
	if txt := info.RawText(); txt == "" || (len(txt) == 1 && strings.ContainsAny(txt, "}]),;")) {
		// token is a symbol for the end of a scope or EOF, which doesn't need a leading comment
		if !sci.extraComments && txt != "" &&
			firstCommentPos.Start().Line == prevInfo.End().Line &&
			lastCommentPos.End().Line == info.Start().Line {
			// protoc does not donate if prev and next token are on the same line since it's
			// ambiguous which one should get the comment; so we mirror that here
			return ast.EmptyComments, lead
		}
		// But with extra comments, we always donate in this situation in order to capture
		// more comments. Because otherwise, these comments are lost since these symbols
		// don't map to a location in source code info.
		return comment, nil
	}
	// cannot donate
	return ast.EmptyComments, lead
}

func (sci *sourceCodeInfo) maybeAttach(prevInfo ast.NodeInfo, info ast.NodeInfo, hasTrail bool, lead []comments) (d []comments, l comments) {
	if len(lead) == 0 {
		return nil, ast.EmptyComments
	}

	if len(lead) == 1 && !hasTrail && prevInfo.IsValid() {
		// If the one comment appears attached to both previous and next tokens,
		// don't attach to either.
		comment := lead[0]
		attachedToPrevious := comment.Index(0).Start().Line == prevInfo.End().Line
		attachedToNext := comment.Index(comment.Len()-1).End().Line == info.Start().Line
		if attachedToPrevious && attachedToNext {
			// Since attachment is ambiguous, leave it detached.
			return lead, ast.EmptyComments
		}
	}

	lastComment := lead[len(lead)-1]
	if lastComment.Index(lastComment.Len()-1).End().Line >= info.Start().Line-1 {
		return lead[:len(lead)-1], lastComment
	}

	return lead, ast.EmptyComments
}

func makeSpan(start, end ast.SourcePos) []int32 {
	if start.Line == end.Line {
		return []int32{int32(start.Line) - 1, int32(start.Col) - 1, int32(end.Col) - 1}
	}
	return []int32{int32(start.Line) - 1, int32(start.Col) - 1, int32(end.Line) - 1, int32(end.Col) - 1}
}

func (sci *sourceCodeInfo) commentUsed(c comments) bool {
	if c.Len() == 0 {
		return false
	}
	pos := c.Index(0).Start()
	if _, ok := sci.commentsUsed[pos]; ok {
		return true
	}

	sci.commentsUsed[pos] = struct{}{}
	return false
}

func groupComments(cmts ast.Comments) []comments {
	if cmts.Len() == 0 {
		return nil
	}
	var groups []comments
	singleLineStyle := cmts.Index(0).RawText()[:2] == "//"
	line := cmts.Index(0).End().Line
	start := 0
	for i := 1; i < cmts.Len(); i++ {
		c := cmts.Index(i)
		prevSingleLine := singleLineStyle
		singleLineStyle = strings.HasPrefix(c.RawText(), "//")
		if !singleLineStyle || prevSingleLine != singleLineStyle || c.Start().Line > line+1 {
			// new group!
			groups = append(groups, subComments{offs: start, n: i - start, c: cmts})
			start = i
		}
		line = c.End().Line
	}
	// don't forget last group
	groups = append(groups, subComments{offs: start, n: cmts.Len() - start, c: cmts})
	return groups
}

func (sci *sourceCodeInfo) combineComments(comments comments) string {
	if comments.Len() == 0 {
		return ""
	}
	var buf bytes.Buffer
	for i, l := 0, comments.Len(); i < l; i++ {
		c := comments.Index(i)
		txt := c.RawText()
		if txt[:2] == "//" {
			buf.WriteString(txt[2:])
			// protoc includes trailing newline for line comments,
			// but it's not present in the AST comment. So we need
			// to add it if present.
			if i, ok := sci.file.Items().Next(c.AsItem()); ok {
				info := sci.file.ItemInfo(i)
				if strings.HasPrefix(info.LeadingWhitespace(), "\n") {
					buf.WriteRune('\n')
				}
			}
		} else {
			lines := strings.Split(txt[2:len(txt)-2], "\n")
			first := true
			for _, l := range lines {
				if first {
					first = false
					buf.WriteString(l)
					continue
				}
				buf.WriteByte('\n')

				// strip a prefix of whitespace followed by '*'
				j := 0
				for j < len(l) {
					if l[j] != ' ' && l[j] != '\t' {
						break
					}
					j++
				}
				switch {
				case j == len(l):
					l = ""
				case l[j] == '*':
					l = l[j+1:]
				case j > 0:
					l = l[j:]
				}

				buf.WriteString(l)
			}
		}
	}
	return buf.String()
}
