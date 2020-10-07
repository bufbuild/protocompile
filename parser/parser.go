package parser

import (
	"fmt"
	"io"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/jhump/protocompile/ast"
	"github.com/jhump/protocompile/reporter"
)

//go:generate goyacc -o proto.y.go -p proto proto.y

func init() {
	protoErrorVerbose = true

	// fix up the generated "token name" array so that error messages are nicer
	setTokenName(_STRING_LIT, "string literal")
	setTokenName(_INT_LIT, "int literal")
	setTokenName(_FLOAT_LIT, "float literal")
	setTokenName(_NAME, "identifier")
	setTokenName(_ERROR, "error")
	// for keywords, just show the keyword itself wrapped in quotes
	for str, i := range keywords {
		setTokenName(i, fmt.Sprintf(`"%s"`, str))
	}
}

func setTokenName(token int, text string) {
	// NB: this is based on logic in generated parse code that translates the
	// int returned from the lexer into an internal token number.
	var intern int
	if token < len(protoTok1) {
		intern = protoTok1[token]
	} else {
		if token >= protoPrivate {
			if token < protoPrivate+len(protoTok2) {
				intern = protoTok2[token-protoPrivate]
			}
		}
		if intern == 0 {
			for i := 0; i+1 < len(protoTok3); i += 2 {
				if protoTok3[i] == token {
					intern = protoTok3[i+1]
					break
				}
			}
		}
	}

	if intern >= 1 && intern-1 < len(protoToknames) {
		protoToknames[intern-1] = text
		return
	}

	panic(fmt.Sprintf("Unknown token value: %d", token))
}

func Parse(filename string, r io.Reader, handler *reporter.Handler) (*ast.FileNode, error) {
	lx := newLexer(r, filename, handler)
	protoParse(lx)
	if lx.res == nil || len(lx.res.Children()) == 0 {
		// nil AST means there was an error that prevented any parsing
		// or the file was empty; synthesize empty non-nil AST
		lx.res = ast.NewEmptyFileNode(filename)
	}
	if lx.eof != nil {
		lx.res.FinalComments = lx.eof.LeadingComments()
		lx.res.FinalWhitespace = lx.eof.LeadingWhitespace()
	}
	return lx.res, handler.Error()
}

type Result interface {
	AST() *ast.FileNode
	Proto() *descriptorpb.FileDescriptorProto

	// Methods for querying AST nodes corresponding to elements in the descriptor hierarchy

	FileNode() ast.FileDeclNode
	Node(proto.Message) ast.Node
	OptionNode(*descriptorpb.UninterpretedOption) ast.OptionDeclNode
	OptionNamePartNode(*descriptorpb.UninterpretedOption_NamePart) ast.Node
	MessageNode(*descriptorpb.DescriptorProto) ast.MessageDeclNode
	FieldNode(*descriptorpb.FieldDescriptorProto) ast.FieldDeclNode
	OneOfNode(*descriptorpb.OneofDescriptorProto) ast.Node
	ExtensionRangeNode(*descriptorpb.DescriptorProto_ExtensionRange) ast.RangeDeclNode
	MessageReservedRangeNode(*descriptorpb.DescriptorProto_ReservedRange) ast.RangeDeclNode
	EnumNode(*descriptorpb.EnumDescriptorProto) ast.Node
	EnumValueNode(*descriptorpb.EnumValueDescriptorProto) ast.EnumValueDeclNode
	EnumReservedRangeNode(*descriptorpb.EnumDescriptorProto_EnumReservedRange) ast.RangeDeclNode
	ServiceNode(*descriptorpb.ServiceDescriptorProto) ast.Node
	MethodNode(*descriptorpb.MethodDescriptorProto) ast.RPCDeclNode
}
