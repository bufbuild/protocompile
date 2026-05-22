// Copyright 2020-2026 Buf Technologies, Inc.
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

package edit_test

import (
	"fmt"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/ast/edit"
	"github.com/bufbuild/protocompile/experimental/ast/printer"
	"github.com/bufbuild/protocompile/experimental/parser"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/experimental/token/keyword"
	"github.com/bufbuild/protocompile/internal/golden"
)

// TestApplyEdits exercises [edit.ApplyEdits] against testdata/edits.
//
// Each <name>.yaml fixture defines a `source` proto and an ordered list
// of `edits` to apply. The test parses the source, converts the YAML
// edits into [edit.Edit] values, applies them via [edit.ApplyEdits],
// then renders the result with [printer.Default] to compare against
// the <name>.yaml.txt golden. The rendered output must re-parse
// cleanly.
//
// To regenerate goldens:
//
//	PROTOCOMPILE_REFRESH=** go test ./experimental/ast/edit/... -run TestApplyEdits
func TestApplyEdits(t *testing.T) {
	t.Parallel()

	corpus := golden.Corpus{
		Root:       "testdata/edits",
		Extensions: []string{"yaml"},
		Refresh:    "PROTOCOMPILE_REFRESH",
		Outputs: []golden.Output{
			{Extension: "txt"},
		},
	}

	opts := printer.Options{
		Format:     true,
		Formatting: printer.Default(),
	}

	corpus.Run(t, func(t *testing.T, path, text string, outputs []string) {
		var spec struct {
			Source string     `yaml:"source"`
			Edits  []editSpec `yaml:"edits"`
		}
		if err := yaml.Unmarshal([]byte(text), &spec); err != nil {
			t.Fatalf("parsing yaml spec: %v", err)
		}

		errs := &report.Report{}
		file, _ := parser.Parse(path, source.NewFile(path, spec.Source), errs)
		hasParseErrors := false
		for _, d := range errs.Diagnostics {
			if d.Level() <= report.Error {
				hasParseErrors = true
			}
			if d.Level() <= report.Warning {
				t.Logf("source parse: %q", d)
			}
		}

		// Build all edits eagerly. Forward references between edits
		// (a later edit targets a decl that an earlier edit adds)
		// are handled via a pending-decls map: each `add_*` registers
		// its constructed decl under the would-be path, and lookups
		// fall back to the map when the file lookup misses. The
		// stashed [ast.DeclAny] is the same ref [edit.ApplyEdits]
		// will insert into the file, so it stays valid as a Target.
		pending := pendingDecls{}
		edits := make([]edit.Edit, 0, len(spec.Edits))
		for i, espec := range spec.Edits {
			e, err := buildEdit(file, pending, espec)
			if err != nil {
				t.Fatalf("building edit[%d] %+v: %v", i, espec, err)
			}
			edits = append(edits, e)
		}

		if err := edit.ApplyEdits(file, edits); err != nil {
			t.Fatalf("ApplyEdits: %v", err)
		}

		got, err := printer.PrintFile(opts, file)
		if err != nil {
			t.Fatalf("PrintFile: %v", err)
		}
		outputs[0] = got

		// Skip the re-parse check when the source had parse errors.
		if hasParseErrors {
			return
		}

		// Re-parse the formatted output to verify validity: edits
		// should not produce an AST that formats to invalid
		// protobuf.
		errs2 := &report.Report{}
		_, _ = parser.Parse(path, source.NewFile(path, got), errs2)
		for _, d := range errs2.Diagnostics {
			if d.Level() <= report.Error {
				t.Errorf("formatted output does not re-parse: %v", d)
			}
		}
	})
}

// editSpec is the YAML shape used by testdata/edits/*.yaml. It is
// converted to [edit.Edit] by [buildEdit] using the file's stream
// and AST helpers.
type editSpec struct {
	Kind   string `yaml:"kind"`
	Target string `yaml:"target"`
	Name   string `yaml:"name"`
	Type   string `yaml:"type"`
	Tag    string `yaml:"tag"`
	Option string `yaml:"option"`
	Value  string `yaml:"value"`
	// Before names the existing decl in Target before which the
	// insertion should land. Honored by `add_option`. The value is
	// the simple name of an existing decl in the target body (or
	// dotted path for `move_decl`). When empty, the insertion
	// appends.
	Before string `yaml:"before"`
}

// pendingDecls maps a dotted path to a [ast.DeclAny] that has been
// constructed for a yet-to-be-applied [edit.Edit]. It lets later
// build steps resolve targets that earlier edits will create. The
// stashed [ast.DeclAny] is the same ref [edit.ApplyEdits] will
// eventually insert into the file, so it stays valid as a Target.
type pendingDecls map[string]ast.DeclAny

// resolve looks up a decl by path: first in the file (already-present
// decls), then in pending (decls awaiting Edit application).
func (p pendingDecls) resolve(file *ast.File, targetPath string) (ast.DeclAny, bool) {
	if d, ok := findDeclByPath(file, targetPath); ok {
		return d, true
	}
	if d, ok := p[targetPath]; ok {
		return d, true
	}
	return ast.DeclAny{}, false
}

// register records a newly-constructed decl at fullPath so subsequent
// edits can reference it as a target.
func (p pendingDecls) register(fullPath string, d ast.DeclAny) {
	p[fullPath] = d
}

// buildEdit converts a YAML edit spec into an [edit.Edit] by
// performing any pre-edit AST setup (constructing insertion decls,
// looking up targets) and packaging them into the appropriate edit
// shape. Targets are always passed to [edit.Edit] as the parent
// definition (DeclDef.AsAny()) — never the bare body — so
// [edit.ApplyEdits] can classify the container via
// [ast.DeclDef.Classify].
func buildEdit(file *ast.File, pending pendingDecls, spec editSpec) (edit.Edit, error) {
	stream := file.Stream()
	nodes := file.Nodes()

	switch spec.Kind {
	case "add_option":
		target, err := ensureOptionTarget(file, pending, spec.Target)
		if err != nil {
			return edit.Edit{}, err
		}
		opt := createOptionDecl(stream, nodes, spec.Option, spec.Value)
		var before ast.DeclAny
		if spec.Before != "" {
			anchor, err := resolveAnchor(file, target, spec.Before)
			if err != nil {
				return edit.Edit{}, fmt.Errorf("before %q: %w", spec.Before, err)
			}
			before = anchor
		}
		return edit.Edit{
			Kind:       edit.KindAdd,
			Target:     target,
			Insertions: []ast.DeclAny{opt.AsAny()},
			Before:     before,
		}, nil

	case "add_message":
		msg := createMessageDecl(stream, nodes, spec.Name)
		return buildAdd(file, pending, spec.Target, spec.Name, msg.AsAny())

	case "add_field":
		field := createFieldDecl(stream, nodes, spec.Type, spec.Name, spec.Tag)
		return buildAdd(file, pending, spec.Target, spec.Name, field.AsAny())

	case "add_enum":
		e := createEnumDecl(stream, nodes, spec.Name)
		return buildAdd(file, pending, spec.Target, spec.Name, e.AsAny())

	case "add_enum_value":
		ev := createEnumValueDecl(stream, nodes, spec.Name, spec.Tag)
		return buildAdd(file, pending, spec.Target, spec.Name, ev.AsAny())

	case "add_service":
		svc := createServiceDecl(stream, nodes, spec.Name)
		return buildAdd(file, pending, "", spec.Name, svc.AsAny())

	case "delete_decl":
		target, ok := pending.resolve(file, spec.Target)
		if !ok {
			return edit.Edit{}, fmt.Errorf("decl %q not found", spec.Target)
		}
		return edit.Edit{
			Kind:   edit.KindDelete,
			Target: target,
		}, nil

	case "move_decl":
		target, ok := findTopLevelDeclByName(file, spec.Target)
		if !ok {
			return edit.Edit{}, fmt.Errorf("decl %q not found", spec.Target)
		}
		before, ok := findTopLevelDeclByName(file, spec.Name)
		if !ok {
			return edit.Edit{}, fmt.Errorf("decl %q not found", spec.Name)
		}
		return edit.Edit{
			Kind:   edit.KindMove,
			Target: target,
			Before: before,
		}, nil

	default:
		return edit.Edit{}, fmt.Errorf("unknown edit kind: %q", spec.Kind)
	}
}

// buildAdd builds a KindAdd targeting the decl at targetPath (file
// when empty), inserting the prebuilt decl ins. The new decl is
// registered in pending under "<targetPath>.<name>" (or just "name"
// at file level) so subsequent edits can target it.
func buildAdd(file *ast.File, pending pendingDecls, targetPath, name string, ins ast.DeclAny) (edit.Edit, error) {
	var target ast.DeclAny
	if targetPath != "" {
		t, ok := pending.resolve(file, targetPath)
		if !ok {
			return edit.Edit{}, fmt.Errorf("target %q not found", targetPath)
		}
		target = t
	}
	fullPath := name
	if targetPath != "" {
		fullPath = targetPath + "." + name
	}
	pending.register(fullPath, ins)
	return edit.Edit{
		Kind:       edit.KindAdd,
		Target:     target,
		Insertions: []ast.DeclAny{ins},
	}, nil
}

// ensureOptionTarget returns the target for an "option foo = bar;"
// insertion. The path may identify a message, enum, service, or
// service method (resolved against file + pending). For a service
// method without an existing `{}` body, one is created and attached
// so the resulting target has a body.
func ensureOptionTarget(file *ast.File, pending pendingDecls, targetPath string) (ast.DeclAny, error) {
	if targetPath == "" {
		// File-level target: edit.Edit treats Target.IsZero() as the
		// file's top-level decl list.
		return ast.DeclAny{}, nil
	}
	if d, ok := pending.resolve(file, targetPath); ok {
		return d, nil
	}
	// Service.Method: pending.resolve descends only into
	// messages/enums via findDeclByPath. Locate the method
	// directly, ensuring it has a body for the option to land in.
	parts := strings.Split(targetPath, ".")
	if len(parts) == 2 {
		for d := range seq.Values(file.Decls()) {
			def := d.AsDef()
			if def.IsZero() || def.Classify() != ast.DefKindService {
				continue
			}
			if defName(def) != parts[0] {
				continue
			}
			for md := range seq.Values(def.Body().Decls()) {
				mdef := md.AsDef()
				if mdef.IsZero() || mdef.Classify() != ast.DefKindMethod {
					continue
				}
				if defName(mdef) != parts[1] {
					continue
				}
				if mdef.Body().IsZero() {
					stream := file.Stream()
					nodes := file.Nodes()
					openBrace := stream.NewPunct(keyword.LBrace.String())
					closeBrace := stream.NewPunct(keyword.RBrace.String())
					stream.NewFused(openBrace, closeBrace)
					body := nodes.NewDeclBody(openBrace)
					mdef.SetBody(body)
				}
				return md, nil
			}
		}
	}
	return ast.DeclAny{}, fmt.Errorf("option target %q not found", targetPath)
}

// findDeclByPath returns the [ast.DeclAny] for a decl at the given
// dotted path, searching messages and enums recursively.
func findDeclByPath(file *ast.File, targetPath string) (ast.DeclAny, bool) {
	parts := strings.Split(targetPath, ".")

	if len(parts) == 1 {
		for d := range seq.Values(file.Decls()) {
			def := d.AsDef()
			if def.IsZero() {
				continue
			}
			if defName(def) == parts[0] {
				return d, true
			}
		}
		return ast.DeclAny{}, false
	}

	// Nested path: find the parent body, then the named decl in it.
	parentPath := strings.Join(parts[:len(parts)-1], ".")
	name := parts[len(parts)-1]
	if body := findMessageBody(file, parentPath); !body.IsZero() {
		for d := range seq.Values(body.Decls()) {
			def := d.AsDef()
			if def.IsZero() {
				continue
			}
			if defName(def) == name {
				return d, true
			}
		}
	}
	if body := findEnumBody(file, parentPath); !body.IsZero() {
		for d := range seq.Values(body.Decls()) {
			def := d.AsDef()
			if def.IsZero() {
				continue
			}
			if defName(def) == name {
				return d, true
			}
		}
	}
	return ast.DeclAny{}, false
}

// resolveAnchor returns the decl named `name` in the target container.
// For a file-level target (zero), searches file.Decls(); otherwise
// searches the target def's body decls. Used to resolve `before:` in
// add_option specs.
func resolveAnchor(file *ast.File, target ast.DeclAny, name string) (ast.DeclAny, error) {
	var decls seq.Indexer[ast.DeclAny]
	if target.IsZero() {
		decls = file.Decls()
	} else {
		def := target.AsDef()
		if def.IsZero() || def.Body().IsZero() {
			return ast.DeclAny{}, fmt.Errorf("target has no body")
		}
		decls = def.Body().Decls()
	}
	for d := range seq.Values(decls) {
		dd := d.AsDef()
		if dd.IsZero() {
			continue
		}
		if defName(dd) == name {
			return d, nil
		}
	}
	return ast.DeclAny{}, fmt.Errorf("decl %q not found in target", name)
}

// findTopLevelDeclByName returns the file-level decl with the given
// name.
func findTopLevelDeclByName(file *ast.File, name string) (ast.DeclAny, bool) {
	for d := range seq.Values(file.Decls()) {
		def := d.AsDef()
		if def.IsZero() {
			continue
		}
		if defName(def) == name {
			return d, true
		}
	}
	return ast.DeclAny{}, false
}

// defName returns the simple name of a definition, handling both natural
// paths (where AsIdent works) and synthetic paths (where we must iterate
// Components). Returns "" if the name cannot be determined.
func defName(def ast.DeclDef) string {
	name := def.Name()
	if name.IsZero() {
		return ""
	}
	if ident := name.AsIdent(); !ident.IsZero() {
		return ident.Text()
	}
	for pc := range name.Components() {
		if !pc.Name().IsZero() {
			return pc.Name().Text()
		}
	}
	return ""
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

			if defName(def) != parts[depth] {
				continue
			}

			// Found matching message at this level
			msg := def.AsMessage()
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
				if defName(def) == parts[depth] {
					return def.AsEnum().Body
				}
			}

			// Check for message to recurse into
			if def.Classify() == ast.DefKindMessage {
				if defName(def) == parts[depth] && !def.Body().IsZero() {
					if result := searchDecls(def.Body().Decls(), depth+1); !result.IsZero() {
						return result
					}
				}
			}
		}
		return ast.DeclBody{}
	}

	return searchDecls(file.Decls(), 0)
}

// createOptionDecl creates an option declaration.
func createOptionDecl(stream *token.Stream, nodes *ast.Nodes, optionName, optionValue string) ast.DeclDef {
	optionKw := stream.NewIdent(keyword.Option.String())
	nameIdent := stream.NewIdent(optionName)
	equals := stream.NewPunct(keyword.Assign.String())
	valueIdent := stream.NewIdent(optionValue)
	semi := stream.NewPunct(keyword.Semi.String())

	optionNamePath := nodes.NewPath(
		nodes.NewPathComponent(token.Zero, nameIdent),
	)
	optionValuePath := ast.ExprPath{
		Path: nodes.NewPath(nodes.NewPathComponent(token.Zero, valueIdent)),
	}
	return nodes.NewDeclDef(ast.DeclDefArgs{
		Keyword:   optionKw,
		Name:      optionNamePath,
		Equals:    equals,
		Value:     optionValuePath.AsAny(),
		Semicolon: semi,
	})
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

	msgNamePath := nodes.NewPath(
		nodes.NewPathComponent(token.Zero, nameIdent),
	)

	return nodes.NewDeclDef(ast.DeclDefArgs{
		Keyword: msgKw,
		Name:    msgNamePath,
		Body:    body,
	})
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

// createEnumDecl creates a new enum declaration.
func createEnumDecl(stream *token.Stream, nodes *ast.Nodes, name string) ast.DeclDef {
	enumKw := stream.NewIdent(keyword.Enum.String())
	nameIdent := stream.NewIdent(name)

	// Create fused braces for the body
	openBrace := stream.NewPunct(keyword.LBrace.String())
	closeBrace := stream.NewPunct(keyword.RBrace.String())
	stream.NewFused(openBrace, closeBrace)
	body := nodes.NewDeclBody(openBrace)

	enumNamePath := nodes.NewPath(
		nodes.NewPathComponent(token.Zero, nameIdent),
	)

	return nodes.NewDeclDef(ast.DeclDefArgs{
		Keyword: enumKw,
		Name:    enumNamePath,
		Body:    body,
	})
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

// createServiceDecl creates a new service declaration.
func createServiceDecl(stream *token.Stream, nodes *ast.Nodes, name string) ast.DeclDef {
	svcKw := stream.NewIdent(keyword.Service.String())
	nameIdent := stream.NewIdent(name)

	// Create fused braces for the body
	openBrace := stream.NewPunct(keyword.LBrace.String())
	closeBrace := stream.NewPunct(keyword.RBrace.String())
	stream.NewFused(openBrace, closeBrace)
	body := nodes.NewDeclBody(openBrace)

	svcNamePath := nodes.NewPath(
		nodes.NewPathComponent(token.Zero, nameIdent),
	)

	return nodes.NewDeclDef(ast.DeclDefArgs{
		Keyword: svcKw,
		Name:    svcNamePath,
		Body:    body,
	})
}
