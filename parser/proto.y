%{
package parser

//lint:file-ignore SA4006 generated parser has unused values

import (
	"math"

	"github.com/bufbuild/protocompile/ast"
)

%}

// fields inside this union end up as the fields in a structure known
// as ${PREFIX}SymType, of which a reference is passed to the lexer.
%union{
	file         *ast.FileNode
	syn          *ast.SyntaxNode
	fileElement  ast.FileElement
	fileElements []ast.FileElement
	pkg          *ast.PackageNode
	imprt        *ast.ImportNode
	msg          *ast.MessageNode
	msgElement   ast.MessageElement
	msgElements  []ast.MessageElement
	fld          *ast.FieldNode
	fldCard      *ast.FieldLabel
	mapFld       *ast.MapFieldNode
	mapType      *ast.MapTypeNode
	grp          *ast.GroupNode
	oo           *ast.OneOfNode
	ooElement    ast.OneOfElement
	ooElements   []ast.OneOfElement
	ext          *ast.ExtensionRangeNode
	resvd        *ast.ReservedNode
	en           *ast.EnumNode
	enElement    ast.EnumElement
	enElements   []ast.EnumElement
	env          *ast.EnumValueNode
	extend       *ast.ExtendNode
	extElement   ast.ExtendElement
	extElements  []ast.ExtendElement
	svc          *ast.ServiceNode
	svcElement   ast.ServiceElement
	svcElements  []ast.ServiceElement
	mtd          *ast.RPCNode
	rpcType      *ast.RPCTypeNode
	rpcElement   ast.RPCElement
	rpcElements  []ast.RPCElement
	opt          *ast.OptionNode
	opts         *compactOptionList
	ref          *ast.FieldReferenceNode
	optNms       *fieldRefList
	cmpctOpts    *ast.CompactOptionsNode
	rng          *ast.RangeNode
	rngs         *rangeList
	rngStart     *ast.RangeStartNode
	rngEnd       *ast.RangeEndNode
	names        *nameList
	cid          *identList
	tid          ast.IdentValueNode
	sl           *valueList
	msgLitFld    *ast.MessageFieldNode
	msgLitFlds   *messageFieldList
	v            ast.ValueNode
	il           ast.IntValueNode
	str          *stringList
	s            *ast.StringLiteralNode
	i            *ast.UintLiteralNode
	f            *ast.FloatLiteralNode
	id           *ast.IdentNode
	b            *ast.RuneNode
	err          error
}

// any non-terminal which returns a value needs a type, which is
// really a field name in the above union struct
%type <file>         file
%type <syn>          syntaxDecl
%type <fileElement>  fileElement
%type <fileElements> fileElements
%type <imprt>        importDecl
%type <pkg>          packageDecl
%type <opt>          optionDecl compactOption
%type <opts>         compactOptionDecls
%type <rpcElement>   methodElement
%type <rpcElements>  methodElements
%type <ref>          extensionName messageLiteralFieldName
%type <optNms>       optionName
%type <cmpctOpts>    compactOptions
%type <v>            value optionValue scalarValue messageLiteralWithBraces messageLiteral numLit listLiteral listElement listOfMessagesLiteral messageValue
%type <il>           enumValueNumber
%type <id>           identifier mapKeyType msgElementName extElementName oneofElementName enumValueName
%type <cid>          qualifiedIdentifier msgElementIdent extElementIdent oneofElementIdent
%type <tid>          typeName msgElementTypeIdent extElementTypeIdent oneofElementTypeIdent
%type <sl>           listElements messageLiterals
%type <msgLitFld>    messageLiteralField
%type <msgLitFlds>   messageLiteralFields messageTextFormat
%type <fld>          fieldDecl oneofFieldDecl
%type <fldCard>      fieldCardinality
%type <oo>           oneofDecl
%type <grp>          groupDecl oneofGroupDecl
%type <mapFld>       mapFieldDecl
%type <mapType>      mapType
%type <msg>          messageDecl
%type <msgElement>   messageElement
%type <msgElements>  messageElements
%type <ooElement>    oneofElement
%type <ooElements>   oneofElements
%type <names>        fieldNames
%type <resvd>        msgReserved enumReserved reservedNames
%type <rng>          tagRange enumValueRange
%type <rngs>         tagRanges enumValueRanges
%type <rngStart>     enumValueRangeStart tagRangeStart
%type <rngEnd>       enumValueRangeEnd tagRangeEnd
%type <ext>          extensionRangeDecl
%type <en>           enumDecl
%type <enElement>    enumElement
%type <enElements>   enumElements
%type <env>          enumValueDecl
%type <extend>       extensionDecl
%type <extElement>   extensionElement
%type <extElements>  extensionElements
%type <str>          stringLit
%type <svc>          serviceDecl
%type <svcElement>   serviceElement
%type <svcElements>  serviceElements
%type <mtd>          methodDecl
%type <rpcType>      methodMessageType

// same for terminals
%token <s>   _STRING_LIT
%token <i>   _INT_LIT
%token <f>   _FLOAT_LIT
%token <id>  _NAME
%token <id>  _SYNTAX _IMPORT _WEAK _PUBLIC _PACKAGE _OPTION _TRUE _FALSE _INF _NAN _REPEATED _OPTIONAL _REQUIRED
%token <id>  _DOUBLE _FLOAT _INT32 _INT64 _UINT32 _UINT64 _SINT32 _SINT64 _FIXED32 _FIXED64 _SFIXED32 _SFIXED64
%token <id>  _BOOL _STRING _BYTES _GROUP _ONEOF _MAP _EXTENSIONS _TO _MAX _RESERVED _ENUM _MESSAGE _EXTEND
%token <id>  _SERVICE _RPC _STREAM _RETURNS
%token <err> _ERROR
// we define all of these, even ones that aren't used, to improve error messages
// so it shows the unexpected symbol instead of showing "$unk"
%token <b>   '=' ';' ':' '{' '}' '\\' '/' '?' '.' ',' '>' '<' '+' '-' '(' ')' '[' ']' '*' '&' '^' '%' '$' '#' '@' '!' '~' '`'

%%

file : syntaxDecl {
		lex := protolex.(*protoLex)
		$$ = ast.NewFileNode(lex.info, $1, nil, lex.eof)
		lex.res = $$
	}
	| fileElements  {
		lex := protolex.(*protoLex)
		$$ = ast.NewFileNode(lex.info, nil, $1, lex.eof)
		lex.res = $$
	}
	| syntaxDecl fileElements {
		lex := protolex.(*protoLex)
		$$ = ast.NewFileNode(lex.info, $1, $2, lex.eof)
		lex.res = $$
	}
	| {
		lex := protolex.(*protoLex)
		$$ = ast.NewFileNode(lex.info, nil, nil, lex.eof)
		lex.res = $$
	}

fileElements : fileElements fileElement {
		if $2 != nil {
			$$ = append($1, $2)
		} else {
			$$ = $1
		}
	}
	| fileElement {
		if $1 != nil {
			$$ = []ast.FileElement{$1}
		} else {
			$$ = nil
		}
	}

fileElement : importDecl {
		$$ = $1
	}
	| packageDecl {
		$$ = $1
	}
	| optionDecl {
		$$ = $1
	}
	| messageDecl {
		$$ = $1
	}
	| enumDecl {
		$$ = $1
	}
	| extensionDecl {
		$$ = $1
	}
	| serviceDecl {
		$$ = $1
	}
	| ';' {
		$$ = ast.NewEmptyDeclNode($1)
	}
	| error ';' {
		$$ = nil
	}
	| error {
		$$ = nil
	}

syntaxDecl : _SYNTAX '=' stringLit ';' {
		$$ = ast.NewSyntaxNode($1.ToKeyword(), $2, $3.toStringValueNode(), $4)
	}

importDecl : _IMPORT stringLit ';' {
		$$ = ast.NewImportNode($1.ToKeyword(), nil, nil, $2.toStringValueNode(), $3)
	}
	| _IMPORT _WEAK stringLit ';' {
		$$ = ast.NewImportNode($1.ToKeyword(), nil, $2.ToKeyword(), $3.toStringValueNode(), $4)
	}
	| _IMPORT _PUBLIC stringLit ';' {
		$$ = ast.NewImportNode($1.ToKeyword(), $2.ToKeyword(), nil, $3.toStringValueNode(), $4)
	}

packageDecl : _PACKAGE qualifiedIdentifier ';' {
		$$ = ast.NewPackageNode($1.ToKeyword(), $2.toIdentValueNode(nil), $3)
	}

qualifiedIdentifier : identifier {
		$$ = &identList{$1, nil, nil}
	}
	| identifier '.' qualifiedIdentifier {
		$$ = &identList{$1, $2, $3}
	}

// to mimic limitations of protoc recursive-descent parser,
// we don't allowed message statement keywords as identifiers
// (or oneof statement keywords [e.g. "option"] below)

msgElementIdent : msgElementName {
		$$ = &identList{$1, nil, nil}
	}
	| msgElementName '.' qualifiedIdentifier {
		$$ = &identList{$1, $2, $3}
	}

extElementIdent : extElementName {
		$$ = &identList{$1, nil, nil}
	}
	| extElementName '.' qualifiedIdentifier {
		$$ = &identList{$1, $2, $3}
	}

oneofElementIdent : oneofElementName {
		$$ = &identList{$1, nil, nil}
	}
	| oneofElementName '.' qualifiedIdentifier {
		$$ = &identList{$1, $2, $3}
	}

optionDecl : _OPTION optionName '=' optionValue ';' {
		refs, dots := $2.toNodes()
		optName := ast.NewOptionNameNode(refs, dots)
		$$ = ast.NewOptionNode($1.ToKeyword(), optName, $3, $4, $5)
	}

optionName : identifier {
		fieldReferenceNode := ast.NewFieldReferenceNode($1)
		$$ = &fieldRefList{fieldReferenceNode, nil, nil}
	}
	| identifier '.' optionName {
		fieldReferenceNode := ast.NewFieldReferenceNode($1)
                $$ = &fieldRefList{fieldReferenceNode, $2, $3}
	}
	| extensionName {
		$$ = &fieldRefList{$1, nil, nil}
	}
	| extensionName '.' optionName {
		$$ = &fieldRefList{$1, $2, $3}
	}

extensionName : '(' typeName ')' {
		$$ = ast.NewExtensionFieldReferenceNode($1, $2, $3)
	}

optionValue : scalarValue
	| messageLiteralWithBraces

scalarValue : stringLit {
		$$ = $1.toStringValueNode()
	}
	| numLit
	| identifier {
		$$ = $1
	}

numLit : _FLOAT_LIT {
		$$ = $1
	}
	| '-' _FLOAT_LIT {
		$$ = ast.NewSignedFloatLiteralNode($1, $2)
	}
	| '+' _FLOAT_LIT {
		$$ = ast.NewSignedFloatLiteralNode($1, $2)
	}
	| '+' _INF {
		f := ast.NewSpecialFloatLiteralNode($2.ToKeyword())
		$$ = ast.NewSignedFloatLiteralNode($1, f)
	}
	| '-' _INF {
		f := ast.NewSpecialFloatLiteralNode($2.ToKeyword())
		$$ = ast.NewSignedFloatLiteralNode($1, f)
	}
	| _INT_LIT {
		$$ = $1
	}
	| '+' _INT_LIT {
		$$ = ast.NewPositiveUintLiteralNode($1, $2)
	}
	| '-' _INT_LIT {
		if $2.Val > math.MaxInt64 + 1 {
			// can't represent as int so treat as float literal
			$$ = ast.NewSignedFloatLiteralNode($1, $2)
		} else {
			$$ = ast.NewNegativeIntLiteralNode($1, $2)
		}
	}

stringLit : _STRING_LIT {
		$$ = &stringList{$1, nil}
	}
	| _STRING_LIT stringLit  {
		$$ = &stringList{$1, $2}
	}

messageLiteralWithBraces : '{' messageTextFormat '}' {
		fields, delims := $2.toNodes()
		$$ = ast.NewMessageLiteralNode($1, fields, delims, $3)
	}
	| '{' error '}' {
	    $$ = nil
	}

messageTextFormat : messageLiteralFields {
		$$ = $1
	}

messageLiteralFields : messageLiteralField ',' messageLiteralFields {
		if $1 != nil {
			entry := &messageFieldEntry{$1, $2}
			$$ = &messageFieldList{entry, $3}
		} else {
			$$ = nil
		}
	}
	| messageLiteralField ';' messageLiteralFields {
		if $1 != nil {
			entry := &messageFieldEntry{$1, $2}
                	$$ = &messageFieldList{entry, $3}
                } else {
                	$$ = nil
       		}
	}
	| messageLiteralField messageLiteralFields {
		if $1 != nil {
			entry := &messageFieldEntry{$1, nil}
                	$$ = &messageFieldList{entry, $2}
		} else {
			$$ = nil
		}
	}
	| messageLiteralField ',' {
		if $1 != nil {
			entry := &messageFieldEntry{$1, $2}
                	$$ = &messageFieldList{entry, nil}
		} else {
			$$ = nil
		}
	}
	| messageLiteralField ';' {
		if $1 != nil {
			entry := &messageFieldEntry{$1, $2}
			$$ = &messageFieldList{entry, nil}
		} else {
			$$ = nil
		}
	}
	| messageLiteralField {
		if $1 != nil {
			entry := &messageFieldEntry{$1, nil}
			$$ = &messageFieldList{entry, nil}
		} else {
			$$ = nil
		}
	}
	| {
		$$ = nil
	}

messageLiteralField : messageLiteralFieldName ':' value {
		if $1 != nil {
			$$ = ast.NewMessageFieldNode($1, $2, $3)
		} else {
			$$ = nil
		}
	}
	| messageLiteralFieldName messageValue {
		if $1 != nil {
			$$ = ast.NewMessageFieldNode($1, nil, $2)
		} else {
			$$ = nil
		}
	}
	| error ',' {
		$$ = nil
	}
	| error ';' {
		$$ = nil
	}
	| error {
		$$ = nil
	}

messageLiteralFieldName : identifier {
		$$ = ast.NewFieldReferenceNode($1)
	}
	| '[' qualifiedIdentifier ']' {
		$$ = ast.NewExtensionFieldReferenceNode($1, $2.toIdentValueNode(nil), $3)
	}
	| '[' qualifiedIdentifier '/' qualifiedIdentifier ']' {
		$$ = ast.NewAnyTypeReferenceNode($1, $2.toIdentValueNode(nil), $3, $4.toIdentValueNode(nil), $5)
	}
	| '[' error ']' {
		$$ = nil
	}

value : scalarValue {
		$$ = $1
	}
	| messageLiteral {
		$$ = $1
	}
	| listLiteral {
		$$ = $1
	}

messageValue : messageLiteral {
		$$ = $1
	}
	| listOfMessagesLiteral {
		$$ = $1
	}

messageLiterals : messageLiteral ',' messageLiterals {
		$$ = &valueList{$1, $2, $3}
	}
	| messageLiteral {
		$$ = &valueList{$1, nil, nil}
	}

messageLiteral : messageLiteralWithBraces {
		$$ = $1
	}
	| '<' messageTextFormat '>' {
		fields, delims := $2.toNodes()
		$$ = ast.NewMessageLiteralNode($1, fields, delims, $3)
	}
	| '<' error '>' {
		$$ = nil
	}

listLiteral : '[' listElements ']' {
		vals, commas := $2.toNodes()
		$$ = ast.NewArrayLiteralNode($1, vals, commas, $3)
	}
	| '[' ']' {
	 	$$ = ast.NewArrayLiteralNode($1, nil, nil, $2)
	}
	| '[' error ']' {
		$$ = nil
	}

listElements : listElement ',' listElements {
		$$ = &valueList{$1, $2, $3}
	}
	| listElement {
		$$ = &valueList{$1, nil, nil}
	}

listElement : scalarValue {
		$$ = $1
	}
	| messageLiteral {
		$$ = $1
	}

listOfMessagesLiteral : '[' messageLiterals ']' {
		if $2 != nil {
			vals, commas := $2.toNodes()
			$$ = ast.NewArrayLiteralNode($1, vals, commas, $3)
		} else {

		}
	}
	| '[' ']' {
		$$ = ast.NewArrayLiteralNode($1, nil, nil, $1)
	}
	| '[' error ']' {
		$$ = nil
	}

typeName : qualifiedIdentifier {
		$$ = $1.toIdentValueNode(nil)
	}
	| '.' qualifiedIdentifier {
		$$ = $2.toIdentValueNode($1)
	}

msgElementTypeIdent : msgElementIdent {
		$$ = $1.toIdentValueNode(nil)
	}
	| '.' qualifiedIdentifier {
		$$ = $2.toIdentValueNode($1)
	}

extElementTypeIdent : extElementIdent {
		$$ = $1.toIdentValueNode(nil)
	}
	| '.' qualifiedIdentifier {
		$$ = $2.toIdentValueNode($1)
	}

oneofElementTypeIdent : oneofElementIdent {
		$$ = $1.toIdentValueNode(nil)
	}
	| '.' qualifiedIdentifier {
		$$ = $2.toIdentValueNode($1)
	}

fieldDecl : fieldCardinality typeName identifier '=' _INT_LIT ';' {
		$$ = ast.NewFieldNode($1.KeywordNode, $2, $3, $4, $5, nil, $6)
	}
	| fieldCardinality typeName identifier '=' _INT_LIT compactOptions ';' {
		$$ = ast.NewFieldNode($1.KeywordNode, $2, $3, $4, $5, $6, $7)
	}
	| msgElementTypeIdent identifier '=' _INT_LIT ';' {
		$$ = ast.NewFieldNode(nil, $1, $2, $3, $4, nil, $5)
	}
	| msgElementTypeIdent identifier '=' _INT_LIT compactOptions ';' {
		$$ = ast.NewFieldNode(nil, $1, $2, $3, $4, $5, $6)
	}

fieldCardinality : _REQUIRED {
		$$ = ast.NewFieldLabel($1.ToKeyword())
	}
	| _OPTIONAL {
		$$ = ast.NewFieldLabel($1.ToKeyword())
	}
	| _REPEATED {
		$$ = ast.NewFieldLabel($1.ToKeyword())
	}

compactOptions: '[' compactOptionDecls ']' {
		opts, commas := $2.toNodes()
		$$ = ast.NewCompactOptionsNode($1, opts, commas, $3)
	}

compactOptionDecls : compactOption {
		$$ = &compactOptionList{$1, nil, nil}
	}
	| compactOption ',' compactOptionDecls {
		$$ = &compactOptionList{$1, $2, $3}
	}

compactOption: optionName '=' optionValue {
		refs, dots := $1.toNodes()
		optName := ast.NewOptionNameNode(refs, dots)
		$$ = ast.NewCompactOptionNode(optName, $2, $3)
	}

groupDecl : fieldCardinality _GROUP identifier '=' _INT_LIT '{' messageElements '}' {
		$$ = ast.NewGroupNode($1.KeywordNode, $2.ToKeyword(), $3, $4, $5, nil, $6, $7, $8)
	}
	| fieldCardinality _GROUP identifier '=' _INT_LIT compactOptions '{' messageElements '}' {
		$$ = ast.NewGroupNode($1.KeywordNode, $2.ToKeyword(), $3, $4, $5, $6, $7, $8, $9)
	}

oneofDecl : _ONEOF identifier '{' oneofElements '}' {
		$$ = ast.NewOneOfNode($1.ToKeyword(), $2, $3, $4, $5)
	}

oneofElements : oneofElements oneofElement {
		if $2 != nil {
			$$ = append($1, $2)
		} else {
			$$ = $1
		}
	}
	| oneofElement {
		if $1 != nil {
			$$ = []ast.OneOfElement{$1}
		} else {
			$$ = nil
		}
	}
	| {
		$$ = nil
	}

oneofElement : optionDecl {
		$$ = $1
	}
	| oneofFieldDecl {
		$$ = $1
	}
	| oneofGroupDecl {
		$$ = $1
	}
	| error ';' {
		$$ = nil
	}
	| error {
		$$ = nil
	}

oneofFieldDecl : oneofElementTypeIdent identifier '=' _INT_LIT ';' {
		$$ = ast.NewFieldNode(nil, $1, $2, $3, $4, nil, $5)
	}
	| oneofElementTypeIdent identifier '=' _INT_LIT compactOptions ';' {
		$$ = ast.NewFieldNode(nil, $1, $2, $3, $4, $5, $6)
	}

oneofGroupDecl : _GROUP identifier '=' _INT_LIT '{' messageElements '}' {
		$$ = ast.NewGroupNode(nil, $1.ToKeyword(), $2, $3, $4, nil, $5, $6, $7)
	}
	| _GROUP identifier '=' _INT_LIT compactOptions '{' messageElements '}' {
		$$ = ast.NewGroupNode(nil, $1.ToKeyword(), $2, $3, $4, $5, $6, $7, $8)
	}

mapFieldDecl : mapType identifier '=' _INT_LIT ';' {
		$$ = ast.NewMapFieldNode($1, $2, $3, $4, nil, $5)
	}
	| mapType identifier '=' _INT_LIT compactOptions ';' {
		$$ = ast.NewMapFieldNode($1, $2, $3, $4, $5, $6)
	}

mapType : _MAP '<' mapKeyType ',' typeName '>' {
		$$ = ast.NewMapTypeNode($1.ToKeyword(), $2, $3, $4, $5, $6)
	}

mapKeyType : _INT32
	| _INT64
	| _UINT32
	| _UINT64
	| _SINT32
	| _SINT64
	| _FIXED32
	| _FIXED64
	| _SFIXED32
	| _SFIXED64
	| _BOOL
	| _STRING

extensionRangeDecl : _EXTENSIONS tagRanges ';' {
		ranges, commas := $2.toNodes()
		$$ = ast.NewExtensionRangeNode($1.ToKeyword(), ranges, commas, nil, $3)
	}
	| _EXTENSIONS tagRanges compactOptions ';' {
		ranges, commas := $2.toNodes()
		$$ = ast.NewExtensionRangeNode($1.ToKeyword(), ranges, commas, $3, $4)
	}

tagRanges : tagRange {
		$$ = &rangeList{$1, nil, nil}
	}
	| tagRange ',' tagRanges {
		$$ = &rangeList{$1, $2, $3}
	}

tagRange : tagRangeStart {
		$$ = ast.NewRangeNode($1.StartVal, nil, nil, nil)
	}
	| tagRangeStart _TO tagRangeEnd {
		if $3.IsMax() {
			$$ = ast.NewRangeNode($1.StartVal, $2.ToKeyword(), nil, $3.Max)
		} else {
			$$ = ast.NewRangeNode($1.StartVal, $2.ToKeyword(), $3.EndVal, nil)
		}
	}

tagRangeStart : _INT_LIT {
		$$ = ast.NewRangeStartNode($1)
	}

tagRangeEnd : _INT_LIT {
		$$ = ast.NewRangeEndNode($1, nil)
	}
	| _MAX {
		$$ = ast.NewRangeEndNode(nil, $1.ToKeyword())
	}

enumValueRanges : enumValueRange {
		$$ = &rangeList{$1, nil, nil}
	}
	| enumValueRange ',' enumValueRanges {
		$$ = &rangeList{$1, $2, $3}
	}

enumValueRange : enumValueRangeStart {
		$$ = ast.NewRangeNode($1.StartVal, nil, nil, nil)
	}
	| enumValueRangeStart _TO enumValueRangeEnd {
		if $3.IsMax() {
       			$$ = ast.NewRangeNode($1.StartVal, $2.ToKeyword(), nil, $3.Max)
             	} else {
               		$$ = ast.NewRangeNode($1.StartVal, $2.ToKeyword(), $3.EndVal, nil)
                }
	}

enumValueRangeStart : enumValueNumber {
		$$ = ast.NewRangeStartNode($1)
	}

enumValueRangeEnd : enumValueNumber {
		$$ = ast.NewRangeEndNode($1, nil)
	}
	| _MAX {
		$$ = ast.NewRangeEndNode(nil, $1.ToKeyword())
	}

enumValueNumber : _INT_LIT {
		$$ = $1
	}
	| '-' _INT_LIT {
		$$ = ast.NewNegativeIntLiteralNode($1, $2)
	}

msgReserved : _RESERVED tagRanges ';' {
		ranges, commas := $2.toNodes()
		$$ = ast.NewReservedRangesNode($1.ToKeyword(), ranges, commas, $3)
	}
	| reservedNames

enumReserved : _RESERVED enumValueRanges ';' {
		ranges, commas := $2.toNodes()
		$$ = ast.NewReservedRangesNode($1.ToKeyword(), ranges, commas, $3)
	}
	| reservedNames

reservedNames : _RESERVED fieldNames ';' {
		names, commas := $2.toNodes()
		$$ = ast.NewReservedNamesNode($1.ToKeyword(), names, commas, $3)
	}

fieldNames : stringLit {
		$$ = &nameList{$1.toStringValueNode(), nil, nil}
	}
	| stringLit ',' fieldNames {
		$$ = &nameList{$1.toStringValueNode(), $2, $3}
	}

enumDecl : _ENUM identifier '{' enumElements '}' {
		$$ = ast.NewEnumNode($1.ToKeyword(), $2, $3, $4, $5)
	}

enumElements : enumElements enumElement {
		if $2 != nil {
			$$ = append($1, $2)
		} else {
			$$ = $1
		}
	}
	| enumElement {
		if $1 != nil {
			$$ = []ast.EnumElement{$1}
		} else {
			$$ = nil
		}
	}
	| {
		$$ = nil
	}

enumElement : optionDecl {
		$$ = $1
	}
	| enumValueDecl {
		$$ = $1
	}
	| enumReserved {
		$$ = $1
	}
	| ';' {
		$$ = ast.NewEmptyDeclNode($1)
	}
	| error ';' {
		$$ = nil
	}
	| error {
		$$ = nil
	}

enumValueDecl : enumValueName '=' enumValueNumber ';' {
		$$ = ast.NewEnumValueNode($1, $2, $3, nil, $4)
	}
	|  enumValueName '=' enumValueNumber compactOptions ';' {
		$$ = ast.NewEnumValueNode($1, $2, $3, $4, $5)
	}

messageDecl : _MESSAGE identifier '{' messageElements '}' {
		$$ = ast.NewMessageNode($1.ToKeyword(), $2, $3, $4, $5)
	}

messageElements : messageElements messageElement {
		if $2 != nil {
			$$ = append($1, $2)
		} else {
			$$ = $1
		}
	}
	| messageElement {
		if $1 != nil {
			$$ = []ast.MessageElement{$1}
		} else {
			$$ = nil
		}
	}
	| {
		$$ = nil
	}

messageElement : fieldDecl {
		$$ = $1
	}
	| enumDecl {
		$$ = $1
	}
	| messageDecl {
		$$ = $1
	}
	| extensionDecl {
		$$ = $1
	}
	| extensionRangeDecl {
		$$ = $1
	}
	| groupDecl {
		$$ = $1
	}
	| optionDecl {
		$$ = $1
	}
	| oneofDecl {
		$$ = $1
	}
	| mapFieldDecl {
		$$ = $1
	}
	| msgReserved {
		$$ = $1
	}
	| ';' {
		$$ = ast.NewEmptyDeclNode($1)
	}
	| error ';' {
		$$ = nil
	}
	| error {
		$$ = nil
	}

extensionDecl : _EXTEND typeName '{' extensionElements '}' {
		$$ = ast.NewExtendNode($1.ToKeyword(), $2, $3, $4, $5)
	}

extensionElements : extensionElements extensionElement {
		if $2 != nil {
			$$ = append($1, $2)
		} else {
			$$ = $1
		}
	}
	| extensionElement {
		if $1 != nil {
			$$ = []ast.ExtendElement{$1}
		} else {
			$$ = nil
		}
	}
	| {
		$$ = nil
	}

extensionElement : fieldDecl {
		$$ = $1
	}
	| groupDecl {
		$$ = $1
	}
	| error ';' {
		$$ = nil
	}
	| error {
		$$ = nil
	}

serviceDecl : _SERVICE identifier '{' serviceElements '}' {
		$$ = ast.NewServiceNode($1.ToKeyword(), $2, $3, $4, $5)
	}

serviceElements : serviceElements serviceElement {
		if $2 != nil {
			$$ = append($1, $2)
		} else {
			$$ = $1
		}
	}
	| serviceElement {
		if $1 != nil {
			$$ = []ast.ServiceElement{$1}
		} else {
			$$ = nil
		}
	}
	| {
		$$ = nil
	}

// NB: doc suggests support for "stream" declaration, separate from "rpc", but
// it does not appear to be supported in protoc (doc is likely from grammar for
// Google-internal version of protoc, with support for streaming stubby)
serviceElement : optionDecl {
		$$ = $1
	}
	| methodDecl {
		$$ = $1
	}
	| ';' {
		$$ = ast.NewEmptyDeclNode($1)
	}
	| error ';' {
		$$ = nil
	}
	| error {
		$$ = nil
	}

methodDecl : _RPC identifier methodMessageType _RETURNS methodMessageType ';' {
		$$ = ast.NewRPCNode($1.ToKeyword(), $2, $3, $4.ToKeyword(), $5, $6)
	}
	| _RPC identifier methodMessageType _RETURNS methodMessageType '{' methodElements '}' {
		$$ = ast.NewRPCNodeWithBody($1.ToKeyword(), $2, $3, $4.ToKeyword(), $5, $6, $7, $8)
	}

methodMessageType : '(' _STREAM typeName ')' {
		$$ = ast.NewRPCTypeNode($1, $2.ToKeyword(), $3, $4)
	}
	| '(' typeName ')' {
		$$ = ast.NewRPCTypeNode($1, nil, $2, $3)
	}

methodElements : methodElements methodElement {
		if $2 != nil {
			$$ = append($1, $2)
		} else {
			$$ = $1
		}
	}
	| methodElement {
		if $1 != nil {
			$$ = []ast.RPCElement{$1}
		} else {
			$$ = nil
		}
	}
	| {
		$$ = nil
	}

methodElement : optionDecl {
		$$ = $1
	}
	| ';' {
		$$ = ast.NewEmptyDeclNode($1)
	}
	| error ';' {
		$$ = nil
	}
	| error {
		$$ = nil
	}

// excludes message, enum, oneof, extensions, reserved, extend,
//   option, optional, required, and repeated
msgElementName : _NAME
	| _SYNTAX
	| _IMPORT
	| _WEAK
	| _PUBLIC
	| _PACKAGE
	| _TRUE
	| _FALSE
	| _INF
	| _NAN
	| _DOUBLE
	| _FLOAT
	| _INT32
	| _INT64
	| _UINT32
	| _UINT64
	| _SINT32
	| _SINT64
	| _FIXED32
	| _FIXED64
	| _SFIXED32
	| _SFIXED64
	| _BOOL
	| _STRING
	| _BYTES
	| _GROUP
	| _MAP
	| _TO
	| _MAX
	| _SERVICE
	| _RPC
	| _STREAM
	| _RETURNS

// excludes optional, required, and repeated
extElementName : _NAME
	| _SYNTAX
	| _IMPORT
	| _WEAK
	| _PUBLIC
	| _PACKAGE
	| _OPTION
	| _TRUE
	| _FALSE
	| _INF
	| _NAN
	| _DOUBLE
	| _FLOAT
	| _INT32
	| _INT64
	| _UINT32
	| _UINT64
	| _SINT32
	| _SINT64
	| _FIXED32
	| _FIXED64
	| _SFIXED32
	| _SFIXED64
	| _BOOL
	| _STRING
	| _BYTES
	| _GROUP
	| _ONEOF
	| _MAP
	| _EXTENSIONS
	| _TO
	| _MAX
	| _RESERVED
	| _ENUM
	| _MESSAGE
	| _EXTEND
	| _SERVICE
	| _RPC
	| _STREAM
	| _RETURNS

// excludes reserved, option
enumValueName : _NAME
	| _SYNTAX
	| _IMPORT
	| _WEAK
	| _PUBLIC
	| _PACKAGE
	| _TRUE
	| _FALSE
	| _INF
	| _NAN
	| _REPEATED
	| _OPTIONAL
	| _REQUIRED
	| _DOUBLE
	| _FLOAT
	| _INT32
	| _INT64
	| _UINT32
	| _UINT64
	| _SINT32
	| _SINT64
	| _FIXED32
	| _FIXED64
	| _SFIXED32
	| _SFIXED64
	| _BOOL
	| _STRING
	| _BYTES
	| _GROUP
	| _ONEOF
	| _MAP
	| _EXTENSIONS
	| _TO
	| _MAX
	| _ENUM
	| _MESSAGE
	| _EXTEND
	| _SERVICE
	| _RPC
	| _STREAM
	| _RETURNS

// excludes option, optional, required, and repeated
oneofElementName : _NAME
	| _SYNTAX
	| _IMPORT
	| _WEAK
	| _PUBLIC
	| _PACKAGE
	| _TRUE
	| _FALSE
	| _INF
	| _NAN
	| _DOUBLE
	| _FLOAT
	| _INT32
	| _INT64
	| _UINT32
	| _UINT64
	| _SINT32
	| _SINT64
	| _FIXED32
	| _FIXED64
	| _SFIXED32
	| _SFIXED64
	| _BOOL
	| _STRING
	| _BYTES
	| _GROUP
	| _ONEOF
	| _MAP
	| _EXTENSIONS
	| _TO
	| _MAX
	| _RESERVED
	| _ENUM
	| _MESSAGE
	| _EXTEND
	| _SERVICE
	| _RPC
	| _STREAM
	| _RETURNS

identifier : _NAME
	| _SYNTAX
	| _IMPORT
	| _WEAK
	| _PUBLIC
	| _PACKAGE
	| _OPTION
	| _TRUE
	| _FALSE
	| _INF
	| _NAN
	| _REPEATED
	| _OPTIONAL
	| _REQUIRED
	| _DOUBLE
	| _FLOAT
	| _INT32
	| _INT64
	| _UINT32
	| _UINT64
	| _SINT32
	| _SINT64
	| _FIXED32
	| _FIXED64
	| _SFIXED32
	| _SFIXED64
	| _BOOL
	| _STRING
	| _BYTES
	| _GROUP
	| _ONEOF
	| _MAP
	| _EXTENSIONS
	| _TO
	| _MAX
	| _RESERVED
	| _ENUM
	| _MESSAGE
	| _EXTEND
	| _SERVICE
	| _RPC
	| _STREAM
	| _RETURNS

%%
