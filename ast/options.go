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

package ast

import "fmt"

// OptionDeclNode is a placeholder interface for AST nodes that represent
// options. This allows NoSourceNode to be used in place of *OptionNode
// for some usages.
type OptionDeclNode interface {
	Node
	GetName() Node
	GetValue() ValueNode
}

var _ OptionDeclNode = (*OptionNode)(nil)
var _ OptionDeclNode = (*NoSourceNode)(nil)

// OptionNode represents the declaration of a single option for an element.
// It is used both for normal option declarations (start with "option" keyword
// and end with semicolon) and for compact options found in fields, enum values,
// and extension ranges. Example:
//
//	option (custom.option) = "foo";
type OptionNode struct {
	compositeNode
	Keyword   *KeywordNode // absent for compact options
	Name      *OptionNameNode
	Equals    *RuneNode
	Val       ValueNode
	Semicolon *RuneNode // absent for compact options
}

func (*OptionNode) fileElement()    {}
func (*OptionNode) msgElement()     {}
func (*OptionNode) oneofElement()   {}
func (*OptionNode) enumElement()    {}
func (*OptionNode) serviceElement() {}
func (*OptionNode) methodElement()  {}

// NewOptionNode creates a new *OptionNode for a full option declaration (as
// used in files, messages, oneofs, enums, services, and methods). All arguments
// must be non-nil. (Also see NewCompactOptionNode.)
//   - keyword: The token corresponding to the "option" keyword.
//   - name: The token corresponding to the name of the option.
//   - equals: The token corresponding to the "=" rune after the name.
//   - val: The token corresponding to the option value.
//   - semicolon: The token corresponding to the ";" rune that ends the declaration.
func NewOptionNode(keyword *KeywordNode, name *OptionNameNode, equals *RuneNode, val ValueNode, semicolon *RuneNode) *OptionNode {
	if keyword == nil {
		panic("keyword is nil")
	}
	if name == nil {
		panic("name is nil")
	}
	if equals == nil {
		panic("equals is nil")
	}
	if val == nil {
		panic("val is nil")
	}
	var children []Node
	if semicolon == nil {
		children = []Node{keyword, name, equals, val}
	} else {
		children = []Node{keyword, name, equals, val, semicolon}
	}

	return &OptionNode{
		compositeNode: compositeNode{
			children: children,
		},
		Keyword:   keyword,
		Name:      name,
		Equals:    equals,
		Val:       val,
		Semicolon: semicolon,
	}
}

// NewCompactOptionNode creates a new *OptionNode for a full compact declaration
// (as used in fields, enum values, and extension ranges). All arguments must be
// non-nil.
//   - name: The token corresponding to the name of the option.
//   - equals: The token corresponding to the "=" rune after the name.
//   - val: The token corresponding to the option value.
func NewCompactOptionNode(name *OptionNameNode, equals *RuneNode, val ValueNode) *OptionNode {
	if name == nil {
		panic("name is nil")
	}
	if equals == nil && val != nil {
		panic("equals is nil but val is not")
	}
	if val == nil && equals != nil {
		panic("val is nil but equals is not")
	}
	var children []Node
	if equals == nil && val == nil {
		children = []Node{name}
	} else {
		children = []Node{name, equals, val}
	}
	return &OptionNode{
		compositeNode: compositeNode{
			children: children,
		},
		Name:   name,
		Equals: equals,
		Val:    val,
	}
}

func (n *OptionNode) GetName() Node {
	return n.Name
}

func (n *OptionNode) GetValue() ValueNode {
	return n.Val
}

// OptionNameNode represents an option name or even a traversal through message
// types to name a nested option field. Example:
//
//	(foo.bar).baz.(bob)
type OptionNameNode struct {
	compositeNode
	Parts []*FieldReferenceNode
	// Dots represent the separating '.' characters between name parts. The
	// length of this slice must be exactly len(Parts)-1, each item in Parts
	// having a corresponding item in this slice *except the last* (since a
	// trailing dot is not allowed).
	//
	// These do *not* include dots that are inside of an extension name. For
	// example: (foo.bar).baz.(bob) has three parts:
	//    1. (foo.bar)  - an extension name
	//    2. baz        - a regular field in foo.bar
	//    3. (bob)      - an extension field in baz
	// Note that the dot in foo.bar will thus not be present in Dots but is
	// instead in Parts[0].
	Dots []*RuneNode
}

// NewOptionNameNode creates a new *OptionNameNode. The dots arg must have a
// length that is one less than the length of parts. The parts arg must not be
// empty.
func NewOptionNameNode(parts []*FieldReferenceNode, dots []*RuneNode) *OptionNameNode {
	if len(parts) == 0 {
		panic("must have at least one part")
	}
	if len(dots) != len(parts)-1 && len(dots) != len(parts) {
		panic(fmt.Sprintf("%d parts requires %d dots, not %d", len(parts), len(parts)-1, len(dots)))
	}
	children := make([]Node, 0, len(parts)+len(dots))
	for i, part := range parts {
		if part == nil {
			panic(fmt.Sprintf("parts[%d] is nil", i))
		}
		if i > 0 {
			if dots[i-1] == nil {
				panic(fmt.Sprintf("dots[%d] is nil", i-1))
			}
			children = append(children, dots[i-1])
		}
		children = append(children, part)
	}
	if len(dots) == len(parts) { // Add the erroneous, but tolerated trailing dot.
		if dots[len(dots)-1] == nil {
			panic(fmt.Sprintf("dots[%d] is nil", len(dots)-1))
		}
		children = append(children, dots[len(dots)-1])
	}
	return &OptionNameNode{
		compositeNode: compositeNode{
			children: children,
		},
		Parts: parts,
		Dots:  dots,
	}
}

// FieldReferenceNode is a reference to a field name. It can indicate a regular
// field (simple unqualified name), an extension field (possibly-qualified name
// that is enclosed either in brackets or parentheses), or an "any" type
// reference (a type URL in the form "server.host/fully.qualified.Name" that is
// enclosed in brackets).
//
// Extension names are used in options to refer to custom options (which are
// actually extensions), in which case the name is enclosed in parentheses "("
// and ")". They can also be used to refer to extension fields of options.
//
// Extension names are also used in message literals to set extension fields,
// in which case the name is enclosed in square brackets "[" and "]".
//
// "Any" type references can only be used in message literals, and are not
// allowed in option names. They are always enclosed in square brackets. An
// "any" type reference is distinguished from an extension name by the presence
// of a slash, which must be present in an "any" type reference and must be
// absent in an extension name.
//
// Examples:
//
//	foobar
//	(foo.bar)
//	[foo.bar]
//	[type.googleapis.com/foo.bar]
type FieldReferenceNode struct {
	compositeNode
	Open *RuneNode // only present for extension names and "any" type references

	// only present for "any" type references
	URLPrefix IdentValueNode
	Slash     *RuneNode

	Name IdentValueNode

	Close *RuneNode // only present for extension names and "any" type references
}

// NewFieldReferenceNode creates a new *FieldReferenceNode for a regular field.
// The name arg must not be nil.
func NewFieldReferenceNode(name *IdentNode) *FieldReferenceNode {
	if name == nil {
		panic("name is nil")
	}
	children := []Node{name}
	return &FieldReferenceNode{
		compositeNode: compositeNode{
			children: children,
		},
		Name: name,
	}
}

// NewExtensionFieldReferenceNode creates a new *FieldReferenceNode for an
// extension field. All args must be non-nil. The openSym and closeSym runes
// should be "(" and ")" or "[" and "]".
func NewExtensionFieldReferenceNode(openSym *RuneNode, name IdentValueNode, closeSym *RuneNode) *FieldReferenceNode {
	if name == nil {
		panic("name is nil")
	}
	if openSym == nil {
		panic("openSym is nil")
	}
	if closeSym == nil {
		panic("closeSym is nil")
	}
	children := []Node{openSym, name, closeSym}
	return &FieldReferenceNode{
		compositeNode: compositeNode{
			children: children,
		},
		Open:  openSym,
		Name:  name,
		Close: closeSym,
	}
}

// NewAnyTypeReferenceNode creates a new *FieldReferenceNode for an "any"
// type reference. All args must be non-nil. The openSym and closeSym runes
// should be "[" and "]". The slashSym run should be "/".
func NewAnyTypeReferenceNode(openSym *RuneNode, urlPrefix IdentValueNode, slashSym *RuneNode, name IdentValueNode, closeSym *RuneNode) *FieldReferenceNode {
	if name == nil {
		panic("name is nil")
	}
	if openSym == nil {
		panic("openSym is nil")
	}
	if closeSym == nil {
		panic("closeSym is nil")
	}
	if urlPrefix == nil {
		panic("urlPrefix is nil")
	}
	if slashSym == nil {
		panic("slashSym is nil")
	}
	children := []Node{openSym, urlPrefix, slashSym, name, closeSym}
	return &FieldReferenceNode{
		compositeNode: compositeNode{
			children: children,
		},
		Open:      openSym,
		URLPrefix: urlPrefix,
		Slash:     slashSym,
		Name:      name,
		Close:     closeSym,
	}
}

// IsExtension reports if this is an extension name or not (e.g. enclosed in
// punctuation, such as parentheses or brackets).
func (a *FieldReferenceNode) IsExtension() bool {
	return a.Open != nil && a.Slash == nil
}

// IsAnyTypeReference reports if this is an Any type reference.
func (a *FieldReferenceNode) IsAnyTypeReference() bool {
	return a.Slash != nil
}

func (a *FieldReferenceNode) Value() string {
	if a.Open != nil {
		if a.Slash != nil {
			return string(a.Open.Rune) + string(a.URLPrefix.AsIdentifier()) + string(a.Slash.Rune) + string(a.Name.AsIdentifier()) + string(a.Close.Rune)
		}
		return string(a.Open.Rune) + string(a.Name.AsIdentifier()) + string(a.Close.Rune)
	}
	return string(a.Name.AsIdentifier())
}

// CompactOptionsNode represents a compact options declaration, as used with
// fields, enum values, and extension ranges. Example:
//
//	[deprecated = true, json_name = "foo_bar"]
type CompactOptionsNode struct {
	compositeNode
	OpenBracket *RuneNode
	Options     []*OptionNode
	// Commas represent the separating ',' characters between options. The
	// length of this slice must be exactly len(Options)-1, with each item
	// in Options having a corresponding item in this slice *except the last*
	// (since a trailing comma is not allowed).
	Commas       []*RuneNode
	CloseBracket *RuneNode
}

// NewCompactOptionsNode creates a *CompactOptionsNode. All args must be
// non-nil. The commas arg must have a length that is one less than the
// length of opts. The opts arg must not be empty.
func NewCompactOptionsNode(openBracket *RuneNode, opts []*OptionNode, commas []*RuneNode, closeBracket *RuneNode) *CompactOptionsNode {
	if openBracket == nil {
		panic("openBracket is nil")
	}
	if closeBracket == nil {
		panic("closeBracket is nil")
	}
	if len(opts) == 0 && len(commas) != 0 {
		panic("opts is empty but commas is not")
	}
	if len(opts) != len(commas) && len(opts) != len(commas)+1 {
		panic(fmt.Sprintf("%d opts requires %d commas, not %d", len(opts), len(opts)-1, len(commas)))
	}
	children := make([]Node, 0, len(opts)+len(commas)+2)
	children = append(children, openBracket)
	if len(opts) > 0 {
		for i, opt := range opts {
			if i > 0 {
				if commas[i-1] == nil {
					panic(fmt.Sprintf("commas[%d] is nil", i-1))
				}
				children = append(children, commas[i-1])
			}
			if opt == nil {
				panic(fmt.Sprintf("opts[%d] is nil", i))
			}
			children = append(children, opt)
		}
		if len(opts) == len(commas) { // Add the erroneous, but tolerated trailing comma.
			if commas[len(commas)-1] == nil {
				panic(fmt.Sprintf("commas[%d] is nil", len(commas)-1))
			}
			children = append(children, commas[len(commas)-1])
		}
	}
	children = append(children, closeBracket)

	return &CompactOptionsNode{
		compositeNode: compositeNode{
			children: children,
		},
		OpenBracket:  openBracket,
		Options:      opts,
		Commas:       commas,
		CloseBracket: closeBracket,
	}
}

func (e *CompactOptionsNode) GetElements() []*OptionNode {
	if e == nil {
		return nil
	}
	return e.Options
}

// NodeWithOptions represents a node in the AST that contains
// option statements.
type NodeWithOptions interface {
	Node
	RangeOptions(func(*OptionNode) bool)
}

var _ NodeWithOptions = FileDeclNode(nil)
var _ NodeWithOptions = MessageDeclNode(nil)
var _ NodeWithOptions = OneofDeclNode(nil)
var _ NodeWithOptions = (*EnumNode)(nil)
var _ NodeWithOptions = (*ServiceNode)(nil)
var _ NodeWithOptions = RPCDeclNode(nil)
var _ NodeWithOptions = FieldDeclNode(nil)
var _ NodeWithOptions = EnumValueDeclNode(nil)
var _ NodeWithOptions = (*ExtensionRangeNode)(nil)
var _ NodeWithOptions = (*NoSourceNode)(nil)
