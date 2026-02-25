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

package printer_test

import (
	"fmt"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/ast/printer"
	"github.com/bufbuild/protocompile/experimental/parser"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/experimental/token/keyword"
	"github.com/bufbuild/protocompile/internal/golden"
)

func TestPrinter(t *testing.T) {
	t.Parallel()

	corpus := golden.Corpus{
		Root:       "testdata",
		Extensions: []string{"yaml"},
		Outputs: []golden.Output{
			{Extension: "txt"},
		},
	}

	corpus.Run(t, func(t *testing.T, path, text string, outputs []string) {
		var testCase struct {
			Source       string `yaml:"source"`
			TabstopWidth int    `yaml:"indent"`
			Edits        []Edit `yaml:"edits"`
		}

		if err := yaml.Unmarshal([]byte(text), &testCase); err != nil {
			t.Fatalf("failed to parse test case %q: %v", path, err)
		}

		if testCase.Source == "" {
			t.Fatalf("test case %q missing 'source' field", path)
		}

		// Parse the source
		errs := &report.Report{}
		file, _ := parser.Parse(path, source.NewFile(path, testCase.Source), errs)
		for diagnostic := range errs.Diagnostics {
			t.Logf("parse error: %q", diagnostic)
		}

		// Apply edits if any
		for _, edit := range testCase.Edits {
			if err := applyEdit(file, edit); err != nil {
				t.Fatalf("failed to apply edit in %q: %v", path, err)
			}
		}

		options := printer.Options{
			TabstopWidth: testCase.TabstopWidth,
		}
		outputs[0] = printer.PrintFile(options, file)
	})
}

// Edit represents an edit to apply to the AST.
type Edit struct {
	Kind   string `yaml:"kind"`   // Edit operation type
	Target string `yaml:"target"` // Target path (e.g., "M" or "M.Inner" or "M.field_name")
	Name   string `yaml:"name"`   // Name for new element (message, field, enum, etc.)
	Type   string `yaml:"type"`   // Type for fields
	Tag    string `yaml:"tag"`    // Tag number for fields/enum values
	Option string `yaml:"option"` // Option name (e.g., "deprecated")
	Value  string `yaml:"value"`  // Option value (e.g., "true")
}

// applyEdit applies an edit to the file.
func applyEdit(file *ast.File, edit Edit) error {
	switch edit.Kind {
	case "add_option":
		return addOptionToMessage(file, edit.Target, edit.Option, edit.Value)
	case "add_compact_option":
		return addCompactOption(file, edit.Target, edit.Option, edit.Value)
	case "add_message":
		return addMessage(file, edit.Target, edit.Name)
	case "add_field":
		return addField(file, edit.Target, edit.Name, edit.Type, edit.Tag)
	case "add_enum":
		return addEnum(file, edit.Target, edit.Name)
	case "add_enum_value":
		return addEnumValue(file, edit.Target, edit.Name, edit.Tag)
	case "add_service":
		return addService(file, edit.Name)
	case "delete_decl":
		return deleteDecl(file, edit.Target)
	case "move_decl":
		return moveDecl(file, edit.Target, edit.Name)
	default:
		return fmt.Errorf("unknown edit kind: %s", edit.Kind)
	}
}

// findMessageBody finds a message body by path (e.g., "M" or "M.Inner").
func findMessageBody(file *ast.File, targetPath string) ast.DeclBody {
	parts := strings.Split(targetPath, ".")

	var searchDecls func(decls seq.Indexer[ast.DeclAny], depth int) ast.DeclBody
	searchDecls = func(decls seq.Indexer[ast.DeclAny], depth int) ast.DeclBody {
		if depth >= len(parts) {
			return ast.DeclBody{}
		}

		for decl := range seq.Values(decls) {
			def := decl.AsDef()
			if def.IsZero() {
				continue
			}
			if def.Classify() != ast.DefKindMessage {
				continue
			}

			msg := def.AsMessage()
			if msg.Name.Text() != parts[depth] {
				continue
			}

			// Found matching message at this level
			if depth == len(parts)-1 {
				return msg.Body
			}

			// Need to go deeper
			if !msg.Body.IsZero() {
				if result := searchDecls(msg.Body.Decls(), depth+1); !result.IsZero() {
					return result
				}
			}
		}
		return ast.DeclBody{}
	}

	return searchDecls(file.Decls(), 0)
}

// findFieldDef finds a field definition by path (e.g., "M.field_name" or "M.Inner.field_name").
func findFieldDef(file *ast.File, targetPath string) ast.DeclDef {
	parts := strings.Split(targetPath, ".")
	if len(parts) < 2 {
		return ast.DeclDef{}
	}

	// Find the containing message
	msgPath := strings.Join(parts[:len(parts)-1], ".")
	fieldName := parts[len(parts)-1]

	msgBody := findMessageBody(file, msgPath)
	if msgBody.IsZero() {
		return ast.DeclDef{}
	}

	// Find the field in the message
	for decl := range seq.Values(msgBody.Decls()) {
		def := decl.AsDef()
		if def.IsZero() {
			continue
		}
		if def.Classify() != ast.DefKindField {
			continue
		}
		if def.Name().AsIdent().Text() == fieldName {
			return def
		}
	}

	return ast.DeclDef{}
}

// findEnumBody finds an enum body by path (e.g., "Status" or "M.Status").
func findEnumBody(file *ast.File, targetPath string) ast.DeclBody {
	parts := strings.Split(targetPath, ".")

	var searchDecls func(decls seq.Indexer[ast.DeclAny], depth int) ast.DeclBody
	searchDecls = func(decls seq.Indexer[ast.DeclAny], depth int) ast.DeclBody {
		if depth >= len(parts) {
			return ast.DeclBody{}
		}

		for decl := range seq.Values(decls) {
			def := decl.AsDef()
			if def.IsZero() {
				continue
			}

			// Check for enum at final level
			if depth == len(parts)-1 && def.Classify() == ast.DefKindEnum {
				enum := def.AsEnum()
				if enum.Name.Text() == parts[depth] {
					return enum.Body
				}
			}

			// Check for message to recurse into
			if def.Classify() == ast.DefKindMessage {
				msg := def.AsMessage()
				if msg.Name.Text() == parts[depth] && !msg.Body.IsZero() {
					if result := searchDecls(msg.Body.Decls(), depth+1); !result.IsZero() {
						return result
					}
				}
			}
		}
		return ast.DeclBody{}
	}

	return searchDecls(file.Decls(), 0)
}

// findEnumValueDef finds an enum value definition by path (e.g., "Status.UNKNOWN").
func findEnumValueDef(file *ast.File, targetPath string) ast.DeclDef {
	parts := strings.Split(targetPath, ".")
	if len(parts) < 2 {
		return ast.DeclDef{}
	}

	// Find the containing enum
	enumPath := strings.Join(parts[:len(parts)-1], ".")
	valueName := parts[len(parts)-1]

	enumBody := findEnumBody(file, enumPath)
	if enumBody.IsZero() {
		return ast.DeclDef{}
	}

	// Find the value in the enum
	for decl := range seq.Values(enumBody.Decls()) {
		def := decl.AsDef()
		if def.IsZero() {
			continue
		}
		if def.Classify() != ast.DefKindEnumValue {
			continue
		}
		if def.Name().AsIdent().Text() == valueName {
			return def
		}
	}

	return ast.DeclDef{}
}

// addOptionToMessage adds an option declaration to a message or method.
func addOptionToMessage(file *ast.File, targetPath, optionName, optionValue string) error {
	stream := file.Stream()
	nodes := file.Nodes()

	// Try finding a message body first
	body := findMessageBody(file, targetPath)

	// If not found, try finding a method body (Service.Method pattern)
	if body.IsZero() {
		body = findOrCreateMethodBody(file, targetPath)
	}

	if body.IsZero() {
		return fmt.Errorf("message or method %q not found", targetPath)
	}

	// Create the option declaration
	optionDecl := createOptionDecl(stream, nodes, optionName, optionValue)

	// Find the right position to insert (after existing options, before fields)
	insertPos := 0
	for i := range body.Decls().Len() {
		decl := body.Decls().At(i)
		def := decl.AsDef()
		if def.IsZero() {
			continue
		}
		if def.Classify() == ast.DefKindOption {
			insertPos = i + 1
		} else {
			break
		}
	}
	body.Decls().Insert(insertPos, optionDecl.AsAny())
	return nil
}

// findOrCreateMethodBody finds a method and returns its body, creating one if needed.
func findOrCreateMethodBody(file *ast.File, targetPath string) ast.DeclBody {
	parts := strings.Split(targetPath, ".")
	if len(parts) != 2 {
		return ast.DeclBody{}
	}
	serviceName, methodName := parts[0], parts[1]

	for decl := range seq.Values(file.Decls()) {
		def := decl.AsDef()
		if def.IsZero() || def.Classify() != ast.DefKindService {
			continue
		}
		if def.Name().AsIdent().Text() != serviceName {
			continue
		}

		svcBody := def.Body()
		for i := range svcBody.Decls().Len() {
			methodDecl := svcBody.Decls().At(i).AsDef()
			if methodDecl.IsZero() || methodDecl.Classify() != ast.DefKindMethod {
				continue
			}
			if methodDecl.Name().AsIdent().Text() != methodName {
				continue
			}

			// Found the method - get or create body
			if methodDecl.Body().IsZero() {
				stream := file.Stream()
				nodes := file.Nodes()
				openBrace := stream.NewPunct(keyword.LBrace.String())
				closeBrace := stream.NewPunct(keyword.RBrace.String())
				stream.NewFused(openBrace, closeBrace)
				body := nodes.NewDeclBody(openBrace)
				methodDecl.SetBody(body)
			}
			return methodDecl.Body()
		}
	}
	return ast.DeclBody{}
}

// addCompactOption adds a compact option to a field or enum value.
func addCompactOption(file *ast.File, targetPath, optionName, optionValue string) error {
	stream := file.Stream()
	nodes := file.Nodes()

	// Try to find as a field first
	fieldDef := findFieldDef(file, targetPath)
	if !fieldDef.IsZero() {
		return addCompactOptionToDef(stream, nodes, fieldDef, optionName, optionValue)
	}

	// Try to find as an enum value
	enumValueDef := findEnumValueDef(file, targetPath)
	if !enumValueDef.IsZero() {
		return addCompactOptionToDef(stream, nodes, enumValueDef, optionName, optionValue)
	}

	return fmt.Errorf("target %q not found", targetPath)
}

// addCompactOptionToDef adds a compact option to a definition (field or enum value).
func addCompactOptionToDef(stream *token.Stream, nodes *ast.Nodes, def ast.DeclDef, optionName, optionValue string) error {
	// Create the option entry
	nameIdent := stream.NewIdent(optionName)
	equals := stream.NewPunct(keyword.Assign.String())
	valueIdent := stream.NewIdent(optionValue)

	optionNamePath := nodes.NewPath(
		nodes.NewPathComponent(token.Zero, nameIdent),
	)

	optionValueExpr := ast.ExprPath{
		Path: nodes.NewPath(nodes.NewPathComponent(token.Zero, valueIdent)),
	}

	// Get or create compact options
	options := def.Options()
	if options.IsZero() {
		// Create new compact options with fused brackets
		openBracket := stream.NewPunct(keyword.LBracket.String())
		closeBracket := stream.NewPunct(keyword.RBracket.String())
		stream.NewFused(openBracket, closeBracket)
		options = nodes.NewCompactOptions(openBracket)
		def.SetOptions(options)
	}

	// Add the option
	opt := ast.Option{
		Path:   optionNamePath,
		Equals: equals,
		Value:  optionValueExpr.AsAny(),
	}

	entries := options.Entries()
	if entries.Len() > 0 && entries.Comma(entries.Len()-1).IsZero() {
		// Add a comma after the last existing entry (only if it doesn't already have one)
		comma := stream.NewPunct(keyword.Comma.String())
		entries.SetComma(entries.Len()-1, comma)
	}
	seq.Append(entries, opt)
	return nil
}

// createOptionDecl creates an option declaration.
func createOptionDecl(stream *token.Stream, nodes *ast.Nodes, optionName, optionValue string) ast.DeclDef {
	optionKw := stream.NewIdent(keyword.Option.String())
	nameIdent := stream.NewIdent(optionName)
	equals := stream.NewPunct(keyword.Assign.String())
	valueIdent := stream.NewIdent(optionValue)
	semi := stream.NewPunct(keyword.Semi.String())

	optionType := ast.TypePath{
		Path: nodes.NewPath(nodes.NewPathComponent(token.Zero, optionKw)),
	}
	optionNamePath := nodes.NewPath(
		nodes.NewPathComponent(token.Zero, nameIdent),
	)
	optionValuePath := ast.ExprPath{
		Path: nodes.NewPath(nodes.NewPathComponent(token.Zero, valueIdent)),
	}
	return nodes.NewDeclDef(ast.DeclDefArgs{
		Type:      optionType.AsAny(),
		Name:      optionNamePath,
		Equals:    equals,
		Value:     optionValuePath.AsAny(),
		Semicolon: semi,
	})
}

// addMessage adds a new message to the file or to a target message.
func addMessage(file *ast.File, target, name string) error {
	stream := file.Stream()
	nodes := file.Nodes()

	msgDecl := createMessageDecl(stream, nodes, name)

	if target == "" {
		// Add to file level
		seq.Append(file.Decls(), msgDecl.AsAny())
	} else {
		// Add to target message
		msgBody := findMessageBody(file, target)
		if msgBody.IsZero() {
			return fmt.Errorf("message %q not found", target)
		}
		seq.Append(msgBody.Decls(), msgDecl.AsAny())
	}
	return nil
}

// createMessageDecl creates a new message declaration.
func createMessageDecl(stream *token.Stream, nodes *ast.Nodes, name string) ast.DeclDef {
	msgKw := stream.NewIdent(keyword.Message.String())
	nameIdent := stream.NewIdent(name)

	// Create fused braces for the body
	openBrace := stream.NewPunct(keyword.LBrace.String())
	closeBrace := stream.NewPunct(keyword.RBrace.String())
	stream.NewFused(openBrace, closeBrace)
	body := nodes.NewDeclBody(openBrace)

	msgType := ast.TypePath{
		Path: nodes.NewPath(nodes.NewPathComponent(token.Zero, msgKw)),
	}
	msgNamePath := nodes.NewPath(
		nodes.NewPathComponent(token.Zero, nameIdent),
	)

	return nodes.NewDeclDef(ast.DeclDefArgs{
		Type: msgType.AsAny(),
		Name: msgNamePath,
		Body: body,
	})
}

// addField adds a new field to a message.
func addField(file *ast.File, target, name, typeName, tag string) error {
	stream := file.Stream()
	nodes := file.Nodes()

	msgBody := findMessageBody(file, target)
	if msgBody.IsZero() {
		return fmt.Errorf("message %q not found", target)
	}

	fieldDecl := createFieldDecl(stream, nodes, typeName, name, tag)
	seq.Append(msgBody.Decls(), fieldDecl.AsAny())
	return nil
}

// createFieldDecl creates a new field declaration.
func createFieldDecl(stream *token.Stream, nodes *ast.Nodes, typeName, name, tag string) ast.DeclDef {
	typeIdent := stream.NewIdent(typeName)
	nameIdent := stream.NewIdent(name)
	equals := stream.NewPunct(keyword.Assign.String())
	tagIdent := stream.NewIdent(tag)
	semi := stream.NewPunct(keyword.Semi.String())

	fieldType := ast.TypePath{
		Path: nodes.NewPath(nodes.NewPathComponent(token.Zero, typeIdent)),
	}
	fieldNamePath := nodes.NewPath(
		nodes.NewPathComponent(token.Zero, nameIdent),
	)
	tagExpr := ast.ExprPath{
		Path: nodes.NewPath(nodes.NewPathComponent(token.Zero, tagIdent)),
	}

	return nodes.NewDeclDef(ast.DeclDefArgs{
		Type:      fieldType.AsAny(),
		Name:      fieldNamePath,
		Equals:    equals,
		Value:     tagExpr.AsAny(),
		Semicolon: semi,
	})
}

// addEnum adds a new enum to the file or to a target message.
func addEnum(file *ast.File, target, name string) error {
	stream := file.Stream()
	nodes := file.Nodes()

	enumDecl := createEnumDecl(stream, nodes, name)

	if target == "" {
		// Add to file level
		seq.Append(file.Decls(), enumDecl.AsAny())
	} else {
		// Add to target message
		msgBody := findMessageBody(file, target)
		if msgBody.IsZero() {
			return fmt.Errorf("message %q not found", target)
		}
		seq.Append(msgBody.Decls(), enumDecl.AsAny())
	}
	return nil
}

// createEnumDecl creates a new enum declaration.
func createEnumDecl(stream *token.Stream, nodes *ast.Nodes, name string) ast.DeclDef {
	enumKw := stream.NewIdent(keyword.Enum.String())
	nameIdent := stream.NewIdent(name)

	// Create fused braces for the body
	openBrace := stream.NewPunct(keyword.LBrace.String())
	closeBrace := stream.NewPunct(keyword.RBrace.String())
	stream.NewFused(openBrace, closeBrace)
	body := nodes.NewDeclBody(openBrace)

	enumType := ast.TypePath{
		Path: nodes.NewPath(nodes.NewPathComponent(token.Zero, enumKw)),
	}
	enumNamePath := nodes.NewPath(
		nodes.NewPathComponent(token.Zero, nameIdent),
	)

	return nodes.NewDeclDef(ast.DeclDefArgs{
		Type: enumType.AsAny(),
		Name: enumNamePath,
		Body: body,
	})
}

// addEnumValue adds a new value to an enum.
func addEnumValue(file *ast.File, target, name, tag string) error {
	stream := file.Stream()
	nodes := file.Nodes()

	enumBody := findEnumBody(file, target)
	if enumBody.IsZero() {
		return fmt.Errorf("enum %q not found", target)
	}

	valueDecl := createEnumValueDecl(stream, nodes, name, tag)
	seq.Append(enumBody.Decls(), valueDecl.AsAny())
	return nil
}

// createEnumValueDecl creates a new enum value declaration.
func createEnumValueDecl(stream *token.Stream, nodes *ast.Nodes, name, tag string) ast.DeclDef {
	nameIdent := stream.NewIdent(name)
	equals := stream.NewPunct(keyword.Assign.String())
	tagIdent := stream.NewIdent(tag)
	semi := stream.NewPunct(keyword.Semi.String())

	// Enum values don't have a type keyword, just the name
	valueNamePath := nodes.NewPath(
		nodes.NewPathComponent(token.Zero, nameIdent),
	)
	tagExpr := ast.ExprPath{
		Path: nodes.NewPath(nodes.NewPathComponent(token.Zero, tagIdent)),
	}

	return nodes.NewDeclDef(ast.DeclDefArgs{
		Name:      valueNamePath,
		Equals:    equals,
		Value:     tagExpr.AsAny(),
		Semicolon: semi,
	})
}

// addService adds a new service to the file.
func addService(file *ast.File, name string) error {
	stream := file.Stream()
	nodes := file.Nodes()

	svcDecl := createServiceDecl(stream, nodes, name)
	seq.Append(file.Decls(), svcDecl.AsAny())
	return nil
}

// createServiceDecl creates a new service declaration.
func createServiceDecl(stream *token.Stream, nodes *ast.Nodes, name string) ast.DeclDef {
	svcKw := stream.NewIdent(keyword.Service.String())
	nameIdent := stream.NewIdent(name)

	// Create fused braces for the body
	openBrace := stream.NewPunct(keyword.LBrace.String())
	closeBrace := stream.NewPunct(keyword.RBrace.String())
	stream.NewFused(openBrace, closeBrace)
	body := nodes.NewDeclBody(openBrace)

	svcType := ast.TypePath{
		Path: nodes.NewPath(nodes.NewPathComponent(token.Zero, svcKw)),
	}
	svcNamePath := nodes.NewPath(
		nodes.NewPathComponent(token.Zero, nameIdent),
	)

	return nodes.NewDeclDef(ast.DeclDefArgs{
		Type: svcType.AsAny(),
		Name: svcNamePath,
		Body: body,
	})
}

// deleteDecl deletes a declaration by path.
func deleteDecl(file *ast.File, targetPath string) error {
	parts := strings.Split(targetPath, ".")

	if len(parts) == 1 {
		// Top-level declaration
		return deleteFromDecls(file.Decls(), parts[0])
	}

	// Nested declaration - find the parent
	parentPath := strings.Join(parts[:len(parts)-1], ".")
	name := parts[len(parts)-1]

	// Try to find parent as message
	msgBody := findMessageBody(file, parentPath)
	if !msgBody.IsZero() {
		return deleteFromDecls(msgBody.Decls(), name)
	}

	// Try to find parent as enum
	enumBody := findEnumBody(file, parentPath)
	if !enumBody.IsZero() {
		return deleteFromDecls(enumBody.Decls(), name)
	}

	return fmt.Errorf("parent %q not found", parentPath)
}

// deleteFromDecls deletes a declaration with the given name from a decl list.
func deleteFromDecls(decls seq.Inserter[ast.DeclAny], name string) error {
	for i := range decls.Len() {
		decl := decls.At(i)
		def := decl.AsDef()
		if def.IsZero() {
			continue
		}
		defName := def.Name()
		if defName.IsZero() {
			continue
		}
		if defName.AsIdent().Text() == name {
			decls.Delete(i)
			return nil
		}
	}
	return fmt.Errorf("declaration %q not found", name)
}

// moveDecl moves the declaration named target so that it appears before the
// declaration named before. Both must be top-level declarations.
func moveDecl(file *ast.File, target, before string) error {
	decls := file.Decls()

	// Find the target declaration and save it.
	srcIdx := -1
	var saved ast.DeclAny
	for i := range decls.Len() {
		def := decls.At(i).AsDef()
		if def.IsZero() {
			continue
		}
		name := def.Name()
		if !name.IsZero() && name.AsIdent().Text() == target {
			srcIdx = i
			saved = decls.At(i)
			break
		}
	}
	if srcIdx < 0 {
		return fmt.Errorf("declaration %q not found", target)
	}

	// Delete the source declaration.
	decls.Delete(srcIdx)

	// Find the "before" declaration in the (now shorter) list.
	dstIdx := -1
	for i := range decls.Len() {
		def := decls.At(i).AsDef()
		if def.IsZero() {
			continue
		}
		name := def.Name()
		if !name.IsZero() && name.AsIdent().Text() == before {
			dstIdx = i
			break
		}
	}
	if dstIdx < 0 {
		return fmt.Errorf("declaration %q not found", before)
	}

	// Insert the saved declaration before the target position.
	decls.Insert(dstIdx, saved)
	return nil
}
