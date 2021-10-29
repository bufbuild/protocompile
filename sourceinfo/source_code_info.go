// Package sourceinfo contains the logic for computing source code info for a
// file descriptor.
//
// The inputs to the computation are an AST for a file as well as the index of
// interpreted options for that file.
package sourceinfo

import (
	"bytes"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/jhump/protocompile/ast"
	"github.com/jhump/protocompile/internal"
	"github.com/jhump/protocompile/options"
)

// GenerateSourceInfo generates source code info for the given AST. If the given
// opts is present, it can generate source code info for interpreted options.
// Otherwise, any options in the AST will get source code info as uninterpreted
// options.
func GenerateSourceInfo(file *ast.FileNode, opts options.Index) *descriptorpb.SourceCodeInfo {
	if file == nil {
		return nil
	}

	sci := sourceCodeInfo{file: file, commentsUsed: map[ast.SourcePos]struct{}{}}
	path := make([]int32, 0, 10)

	sci.newLocWithoutComments(file, nil)

	if file.Syntax != nil {
		sci.newLoc(file.Syntax, append(path, internal.File_syntaxTag))
	}

	var depIndex, optIndex, msgIndex, enumIndex, extendIndex, svcIndex int32

	for _, child := range file.Decls {
		switch child := child.(type) {
		case *ast.ImportNode:
			sci.newLoc(child, append(path, internal.File_dependencyTag, depIndex))
			depIndex++
		case *ast.PackageNode:
			sci.newLoc(child, append(path, internal.File_packageTag))
		case *ast.OptionNode:
			generateSourceCodeInfoForOption(opts, &sci, child, false, &optIndex, append(path, internal.File_optionsTag))
		case *ast.MessageNode:
			generateSourceCodeInfoForMessage(opts, &sci, child, nil, append(path, internal.File_messagesTag, msgIndex))
			msgIndex++
		case *ast.EnumNode:
			generateSourceCodeInfoForEnum(opts, &sci, child, append(path, internal.File_enumsTag, enumIndex))
			enumIndex++
		case *ast.ExtendNode:
			generateSourceCodeInfoForExtensions(opts, &sci, child, &extendIndex, &msgIndex, append(path, internal.File_extensionsTag), append(dup(path), internal.File_messagesTag))
		case *ast.ServiceNode:
			generateSourceCodeInfoForService(opts, &sci, child, append(path, internal.File_servicesTag, svcIndex))
			svcIndex++
		}
	}

	return &descriptorpb.SourceCodeInfo{Location: sci.locs}
}

func generateSourceCodeInfoForOption(opts options.Index, sci *sourceCodeInfo, n *ast.OptionNode, compact bool, uninterpIndex *int32, path []int32) {
	if !compact {
		sci.newLocWithoutComments(n, path)
	}
	subPath := opts[n]
	if len(subPath) > 0 {
		p := path
		if subPath[0] == -1 {
			// used by "default" and "json_name" field pseudo-options
			// to attribute path to parent element (since those are
			// stored directly on the descriptor, not its options)
			p = make([]int32, len(path)-1)
			copy(p, path)
			subPath = subPath[1:]
		}
		sci.newLoc(n, append(p, subPath...))
		return
	}

	// it's an uninterpreted option
	optPath := append(path, internal.UninterpretedOptionsTag, *uninterpIndex)
	*uninterpIndex++
	sci.newLoc(n, optPath)
	var valTag int32
	switch n.Val.(type) {
	case ast.IdentValueNode:
		valTag = internal.Uninterpreted_identTag
	case *ast.NegativeIntLiteralNode:
		valTag = internal.Uninterpreted_negIntTag
	case ast.IntValueNode:
		valTag = internal.Uninterpreted_posIntTag
	case ast.FloatValueNode:
		valTag = internal.Uninterpreted_doubleTag
	case ast.StringValueNode:
		valTag = internal.Uninterpreted_stringTag
	case *ast.MessageLiteralNode:
		valTag = internal.Uninterpreted_aggregateTag
	}
	if valTag != 0 {
		sci.newLoc(n.Val, append(optPath, valTag))
	}
	for j, nn := range n.Name.Parts {
		optNmPath := append(optPath, internal.Uninterpreted_nameTag, int32(j))
		sci.newLoc(nn, optNmPath)
		sci.newLoc(nn.Name, append(optNmPath, internal.UninterpretedName_nameTag))
	}
}

func generateSourceCodeInfoForMessage(opts options.Index, sci *sourceCodeInfo, n ast.MessageDeclNode, fieldPath []int32, path []int32) {
	sci.newLoc(n, path)

	var decls []ast.MessageElement
	switch n := n.(type) {
	case *ast.MessageNode:
		decls = n.Decls
	case *ast.GroupNode:
		decls = n.Decls
	case *ast.MapFieldNode:
		// map entry so nothing else to do
		return
	}

	sci.newLoc(n.MessageName(), append(path, internal.Message_nameTag))
	// matching protoc, which emits the corresponding field type name (for group fields)
	// right after the source location for the group message name
	if fieldPath != nil {
		sci.newLoc(n.MessageName(), append(fieldPath, internal.Field_typeNameTag))
	}

	var optIndex, fieldIndex, oneOfIndex, extendIndex, nestedMsgIndex int32
	var nestedEnumIndex, extRangeIndex, reservedRangeIndex, reservedNameIndex int32
	for _, child := range decls {
		switch child := child.(type) {
		case *ast.OptionNode:
			generateSourceCodeInfoForOption(opts, sci, child, false, &optIndex, append(path, internal.Message_optionsTag))
		case *ast.FieldNode:
			generateSourceCodeInfoForField(opts, sci, child, append(path, internal.Message_fieldsTag, fieldIndex))
			fieldIndex++
		case *ast.GroupNode:
			fldPath := append(path, internal.Message_fieldsTag, fieldIndex)
			generateSourceCodeInfoForField(opts, sci, child, fldPath)
			fieldIndex++
			generateSourceCodeInfoForMessage(opts, sci, child, fldPath, append(dup(path), internal.Message_nestedMessagesTag, nestedMsgIndex))
			nestedMsgIndex++
		case *ast.MapFieldNode:
			generateSourceCodeInfoForField(opts, sci, child, append(path, internal.Message_fieldsTag, fieldIndex))
			fieldIndex++
			nestedMsgIndex++
		case *ast.OneOfNode:
			generateSourceCodeInfoForOneOf(opts, sci, child, &fieldIndex, &nestedMsgIndex, append(path, internal.Message_fieldsTag), append(dup(path), internal.Message_nestedMessagesTag), append(dup(path), internal.Message_oneOfsTag, oneOfIndex))
			oneOfIndex++
		case *ast.MessageNode:
			generateSourceCodeInfoForMessage(opts, sci, child, nil, append(path, internal.Message_nestedMessagesTag, nestedMsgIndex))
			nestedMsgIndex++
		case *ast.EnumNode:
			generateSourceCodeInfoForEnum(opts, sci, child, append(path, internal.Message_enumsTag, nestedEnumIndex))
			nestedEnumIndex++
		case *ast.ExtendNode:
			generateSourceCodeInfoForExtensions(opts, sci, child, &extendIndex, &nestedMsgIndex, append(path, internal.Message_extensionsTag), append(dup(path), internal.Message_nestedMessagesTag))
		case *ast.ExtensionRangeNode:
			generateSourceCodeInfoForExtensionRanges(opts, sci, child, &extRangeIndex, append(path, internal.Message_extensionRangeTag))
		case *ast.ReservedNode:
			if len(child.Names) > 0 {
				resPath := append(path, internal.Message_reservedNameTag)
				sci.newLoc(child, resPath)
				for _, rn := range child.Names {
					sci.newLoc(rn, append(resPath, reservedNameIndex))
					reservedNameIndex++
				}
			}
			if len(child.Ranges) > 0 {
				resPath := append(path, internal.Message_reservedRangeTag)
				sci.newLoc(child, resPath)
				for _, rr := range child.Ranges {
					generateSourceCodeInfoForReservedRange(sci, rr, append(resPath, reservedRangeIndex))
					reservedRangeIndex++
				}
			}
		}
	}
}

func generateSourceCodeInfoForEnum(opts options.Index, sci *sourceCodeInfo, n *ast.EnumNode, path []int32) {
	sci.newLoc(n, path)
	sci.newLoc(n.Name, append(path, internal.Enum_nameTag))

	var optIndex, valIndex, reservedNameIndex, reservedRangeIndex int32
	for _, child := range n.Decls {
		switch child := child.(type) {
		case *ast.OptionNode:
			generateSourceCodeInfoForOption(opts, sci, child, false, &optIndex, append(path, internal.Enum_optionsTag))
		case *ast.EnumValueNode:
			generateSourceCodeInfoForEnumValue(opts, sci, child, append(path, internal.Enum_valuesTag, valIndex))
			valIndex++
		case *ast.ReservedNode:
			if len(child.Names) > 0 {
				resPath := append(path, internal.Enum_reservedNameTag)
				sci.newLoc(child, resPath)
				for _, rn := range child.Names {
					sci.newLoc(rn, append(resPath, reservedNameIndex))
					reservedNameIndex++
				}
			}
			if len(child.Ranges) > 0 {
				resPath := append(path, internal.Enum_reservedRangeTag)
				sci.newLoc(child, resPath)
				for _, rr := range child.Ranges {
					generateSourceCodeInfoForReservedRange(sci, rr, append(resPath, reservedRangeIndex))
					reservedRangeIndex++
				}
			}
		}
	}
}

func generateSourceCodeInfoForEnumValue(opts options.Index, sci *sourceCodeInfo, n *ast.EnumValueNode, path []int32) {
	sci.newLoc(n, path)
	sci.newLoc(n.Name, append(path, internal.EnumVal_nameTag))
	sci.newLoc(n.Number, append(path, internal.EnumVal_numberTag))

	// enum value options
	if n.Options != nil {
		optsPath := append(path, internal.EnumVal_optionsTag)
		sci.newLoc(n.Options, optsPath)
		var optIndex int32
		for _, opt := range n.Options.GetElements() {
			generateSourceCodeInfoForOption(opts, sci, opt, true, &optIndex, optsPath)
		}
	}
}

func generateSourceCodeInfoForReservedRange(sci *sourceCodeInfo, n *ast.RangeNode, path []int32) {
	sci.newLoc(n, path)
	sci.newLoc(n.StartVal, append(path, internal.ReservedRange_startTag))
	if n.EndVal != nil {
		sci.newLoc(n.EndVal, append(path, internal.ReservedRange_endTag))
	} else if n.Max != nil {
		sci.newLoc(n.Max, append(path, internal.ReservedRange_endTag))
	}
}

func generateSourceCodeInfoForExtensions(opts options.Index, sci *sourceCodeInfo, n *ast.ExtendNode, extendIndex, msgIndex *int32, extendPath, msgPath []int32) {
	sci.newLoc(n, extendPath)
	for _, decl := range n.Decls {
		switch decl := decl.(type) {
		case *ast.FieldNode:
			generateSourceCodeInfoForField(opts, sci, decl, append(extendPath, *extendIndex))
			*extendIndex++
		case *ast.GroupNode:
			fldPath := append(extendPath, *extendIndex)
			generateSourceCodeInfoForField(opts, sci, decl, fldPath)
			*extendIndex++
			generateSourceCodeInfoForMessage(opts, sci, decl, fldPath, append(msgPath, *msgIndex))
			*msgIndex++
		}
	}
}

func generateSourceCodeInfoForOneOf(opts options.Index, sci *sourceCodeInfo, n *ast.OneOfNode, fieldIndex, nestedMsgIndex *int32, fieldPath, nestedMsgPath, oneOfPath []int32) {
	sci.newLoc(n, oneOfPath)
	sci.newLoc(n.Name, append(oneOfPath, internal.OneOf_nameTag))

	var optIndex int32
	for _, child := range n.Decls {
		switch child := child.(type) {
		case *ast.OptionNode:
			generateSourceCodeInfoForOption(opts, sci, child, false, &optIndex, append(oneOfPath, internal.OneOf_optionsTag))
		case *ast.FieldNode:
			generateSourceCodeInfoForField(opts, sci, child, append(fieldPath, *fieldIndex))
			*fieldIndex++
		case *ast.GroupNode:
			fldPath := append(fieldPath, *fieldIndex)
			generateSourceCodeInfoForField(opts, sci, child, fldPath)
			*fieldIndex++
			generateSourceCodeInfoForMessage(opts, sci, child, fldPath, append(nestedMsgPath, *nestedMsgIndex))
			*nestedMsgIndex++
		}
	}
}

func generateSourceCodeInfoForField(opts options.Index, sci *sourceCodeInfo, n ast.FieldDeclNode, path []int32) {
	var fieldType string
	if f, ok := n.(*ast.FieldNode); ok {
		fieldType = string(f.FldType.AsIdentifier())
	}

	if n.GetGroupKeyword() != nil {
		// comments will appear on group message
		sci.newLocWithoutComments(n, path)
		if n.FieldExtendee() != nil {
			sci.newLoc(n.FieldExtendee(), append(path, internal.Field_extendeeTag))
		}
		if n.FieldLabel() != nil {
			// no comments here either (label is first token for group, so we want
			// to leave the comments to be associated with the group message instead)
			sci.newLocWithoutComments(n.FieldLabel(), append(path, internal.Field_labelTag))
		}
		sci.newLoc(n.FieldType(), append(path, internal.Field_typeTag))
		// let the name comments be attributed to the group name
		sci.newLocWithoutComments(n.FieldName(), append(path, internal.Field_nameTag))
	} else {
		sci.newLoc(n, path)
		if n.FieldExtendee() != nil {
			sci.newLoc(n.FieldExtendee(), append(path, internal.Field_extendeeTag))
		}
		if n.FieldLabel() != nil {
			sci.newLoc(n.FieldLabel(), append(path, internal.Field_labelTag))
		}
		var tag int32
		if _, isScalar := internal.FieldTypes[fieldType]; isScalar {
			tag = internal.Field_typeTag
		} else {
			// this is a message or an enum, so attribute type location
			// to the type name field
			tag = internal.Field_typeNameTag
		}
		sci.newLoc(n.FieldType(), append(path, tag))
		sci.newLoc(n.FieldName(), append(path, internal.Field_nameTag))
	}
	sci.newLoc(n.FieldTag(), append(path, internal.Field_numberTag))

	if n.GetOptions() != nil {
		optsPath := append(path, internal.Field_optionsTag)
		sci.newLoc(n.GetOptions(), optsPath)
		var optIndex int32
		for _, opt := range n.GetOptions().GetElements() {
			generateSourceCodeInfoForOption(opts, sci, opt, true, &optIndex, optsPath)
		}
	}
}

func generateSourceCodeInfoForExtensionRanges(opts options.Index, sci *sourceCodeInfo, n *ast.ExtensionRangeNode, extRangeIndex *int32, path []int32) {
	sci.newLoc(n, path)
	for _, child := range n.Ranges {
		path := append(path, *extRangeIndex)
		*extRangeIndex++
		sci.newLoc(child, path)
		sci.newLoc(child.StartVal, append(path, internal.ExtensionRange_startTag))
		if child.EndVal != nil {
			sci.newLoc(child.EndVal, append(path, internal.ExtensionRange_endTag))
		} else if child.Max != nil {
			sci.newLoc(child.Max, append(path, internal.ExtensionRange_endTag))
		}
		if n.Options != nil {
			optsPath := append(path, internal.ExtensionRange_optionsTag)
			sci.newLoc(n.Options, optsPath)
			var optIndex int32
			for _, opt := range n.Options.GetElements() {
				generateSourceCodeInfoForOption(opts, sci, opt, true, &optIndex, optsPath)
			}
		}
	}
}

func generateSourceCodeInfoForService(opts options.Index, sci *sourceCodeInfo, n *ast.ServiceNode, path []int32) {
	sci.newLoc(n, path)
	sci.newLoc(n.Name, append(path, internal.Service_nameTag))
	var optIndex, rpcIndex int32
	for _, child := range n.Decls {
		switch child := child.(type) {
		case *ast.OptionNode:
			generateSourceCodeInfoForOption(opts, sci, child, false, &optIndex, append(path, internal.Service_optionsTag))
		case *ast.RPCNode:
			generateSourceCodeInfoForMethod(opts, sci, child, append(path, internal.Service_methodsTag, rpcIndex))
			rpcIndex++
		}
	}
}

func generateSourceCodeInfoForMethod(opts options.Index, sci *sourceCodeInfo, n *ast.RPCNode, path []int32) {
	sci.newLoc(n, path)
	sci.newLoc(n.Name, append(path, internal.Method_nameTag))
	if n.Input.Stream != nil {
		sci.newLoc(n.Input.Stream, append(path, internal.Method_inputStreamTag))
	}
	sci.newLoc(n.Input.MessageType, append(path, internal.Method_inputTag))
	if n.Output.Stream != nil {
		sci.newLoc(n.Output.Stream, append(path, internal.Method_outputStreamTag))
	}
	sci.newLoc(n.Output.MessageType, append(path, internal.Method_outputTag))

	optsPath := append(path, internal.Method_optionsTag)
	var optIndex int32
	for _, decl := range n.Decls {
		if opt, ok := decl.(*ast.OptionNode); ok {
			generateSourceCodeInfoForOption(opts, sci, opt, false, &optIndex, optsPath)
		}
	}
}

type sourceCodeInfo struct {
	file         *ast.FileNode
	locs         []*descriptorpb.SourceCodeInfo_Location
	commentsUsed map[ast.SourcePos]struct{}
}

func (sci *sourceCodeInfo) newLocWithoutComments(n ast.Node, path []int32) {
	dup := make([]int32, len(path))
	copy(dup, path)
	info := sci.file.NodeInfo(n)
	sci.locs = append(sci.locs, &descriptorpb.SourceCodeInfo_Location{
		Path: dup,
		Span: makeSpan(info.Start(), info.End()),
	})
}

func (sci *sourceCodeInfo) newLoc(n ast.Node, path []int32) {
	nodeInfo := sci.file.NodeInfo(n)
	leadingComments := nodeInfo.LeadingComments()
	trailingComments := nodeInfo.TrailingComments()
	if sci.commentUsed(leadingComments) {
		leadingComments = ast.Comments{}
	}
	if sci.commentUsed(trailingComments) {
		trailingComments = ast.Comments{}
	}
	detached := groupComments(leadingComments)
	var trail *string
	if str, ok := combineComments(trailingComments, 0, trailingComments.Len()); ok {
		trail = proto.String(str)
	}
	var lead *string
	if leadingComments.Len() > 0 && leadingComments.Index(leadingComments.Len()-1).End().Line >= nodeInfo.Start().Line-1 {
		lead = proto.String(detached[len(detached)-1])
		detached = detached[:len(detached)-1]
	}
	dup := make([]int32, len(path))
	copy(dup, path)
	sci.locs = append(sci.locs, &descriptorpb.SourceCodeInfo_Location{
		LeadingDetachedComments: detached,
		LeadingComments:         lead,
		TrailingComments:        trail,
		Path:                    dup,
		Span:                    makeSpan(nodeInfo.Start(), nodeInfo.End()),
	})
}

func makeSpan(start, end ast.SourcePos) []int32 {
	if start.Line == end.Line {
		return []int32{int32(start.Line) - 1, int32(start.Col) - 1, int32(end.Col) - 1}
	}
	return []int32{int32(start.Line) - 1, int32(start.Col) - 1, int32(end.Line) - 1, int32(end.Col) - 1}
}

func (sci *sourceCodeInfo) commentUsed(c ast.Comments) bool {
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

func groupComments(comments ast.Comments) []string {
	if comments.Len() == 0 {
		return nil
	}

	var groups []string
	singleLineStyle := comments.Index(0).RawText()[:2] == "//"
	line := comments.Index(0).End().Line
	start := 0
	for i := 1; i < comments.Len(); i++ {
		c := comments.Index(i)
		prevSingleLine := singleLineStyle
		singleLineStyle = strings.HasPrefix(c.RawText(), "//")
		if !singleLineStyle || prevSingleLine != singleLineStyle || c.Start().Line > line+1 {
			// new group!
			if str, ok := combineComments(comments, start, i); ok {
				groups = append(groups, str)
			}
			start = i
		}
		line = c.End().Line
	}
	// don't forget last group
	if str, ok := combineComments(comments, start, comments.Len()); ok {
		groups = append(groups, str)
	}
	return groups
}

func combineComments(comments ast.Comments, start, end int) (string, bool) {
	if start >= end {
		return "", false
	}
	var buf bytes.Buffer
	for i := start; i < end; i++ {
		c := comments.Index(i)
		txt := c.RawText()
		if txt[:2] == "//" {
			buf.WriteString(txt[2:])
		} else {
			lines := strings.Split(txt[2:len(txt)-2], "\n")
			first := true
			for _, l := range lines {
				if first {
					first = false
				} else {
					buf.WriteByte('\n')
				}

				// strip a prefix of whitespace followed by '*'
				j := 0
				for j < len(l) {
					if l[j] != ' ' && l[j] != '\t' {
						break
					}
					j++
				}
				if j == len(l) {
					l = ""
				} else if l[j] == '*' {
					l = l[j+1:]
				} else if j > 0 {
					l = " " + l[j:]
				}

				buf.WriteString(l)
			}
		}
	}
	return buf.String(), true
}

func dup(p []int32) []int32 {
	return append(([]int32)(nil), p...)
}
