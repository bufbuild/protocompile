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

package edit

import (
	"errors"
	"fmt"

	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/seq"
)

// Kind is the kind of operation an [Edit] performs.
type Kind int

const (
	// KindAdd appends [Edit.Insertions] to the body decls of
	// [Edit.Target]. If Target is the zero value, insertions are
	// appended to the file's top-level decls.
	KindAdd Kind = iota
	// KindDelete removes [Edit.Target] from its parent's decl list.
	KindDelete
	// KindMove moves [Edit.Target] so that it appears immediately
	// before [Edit.Before] in their shared parent's decl list.
	// Currently both must be top-level decls.
	KindMove
)

// String returns a human-readable name for the kind, used in
// diagnostics.
func (k Kind) String() string {
	switch k {
	case KindAdd:
		return "add"
	case KindDelete:
		return "delete"
	case KindMove:
		return "move"
	default:
		return fmt.Sprintf("Kind(%d)", int(k))
	}
}

// Edit describes a single mutation applied to an [ast.File] by
// [ApplyEdits]. The mutation happens on the file in place; callers
// wishing to preserve the unedited AST must clone it first.
//
// Validity rules for each [Kind] are documented on the kind
// constants. An invalid edit (target not found, insertion not allowed
// in target container, etc.) causes [ApplyEdits] to return an error;
// any edits already applied remain in place.
//
// Edits currently operate on decl-bearing bodies (file, message,
// enum, service, oneof, extend, and method bodies). Modifying the
// compact-options bracket on a field or enum value (e.g. adding
// `[deprecated = true]`) is not yet supported.
type Edit struct {
	// Kind selects the operation.
	Kind Kind

	// Target serves a different role depending on Kind:
	//   - KindAdd:    the container to insert into. Zero means
	//                 the file's top-level decl list.
	//   - KindDelete: the decl to remove.
	//   - KindMove:   the decl to relocate.
	Target ast.DeclAny

	// Insertions are the decls to append to Target's body, in order.
	// Honored only by [KindAdd].
	//
	// Allowed insertion-vs-container pairings:
	//   - option:               file or any decl-bearing body
	//   - message, enum:        file or message body
	//   - field:                message body, oneof body
	//   - enum value:           enum body
	//   - service:              file
	//   - method (RPC):         service body
	//   - oneof, extend, group: message body
	Insertions []ast.DeclAny

	// Before is the destination anchor for KindMove: the moved decl
	// is reinserted immediately before Before. Honored only by
	// KindMove.
	Before ast.DeclAny
}

// ApplyEdits applies edits to file in order, stopping at the first
// error. Edits mutate file in place — clone it first if the caller
// needs the unedited AST.
//
// Typical use is to apply edits before rendering with the printer:
//
//	if err := edit.ApplyEdits(file, edits); err != nil {
//	    return err
//	}
//	out, err := printer.PrintFile(opts, file)
func ApplyEdits(file *ast.File, edits []Edit) error {
	for i, edit := range edits {
		if err := applyEdit(file, edit); err != nil {
			return fmt.Errorf("edit[%d] %s: %w", i, edit.Kind, err)
		}
	}
	return nil
}

func applyEdit(file *ast.File, edit Edit) error {
	switch edit.Kind {
	case KindAdd:
		return applyAdd(file, edit)
	case KindDelete:
		return applyDelete(file, edit)
	case KindMove:
		return applyMove(file, edit)
	default:
		return fmt.Errorf("unknown kind %d", edit.Kind)
	}
}

// applyAdd appends insertions to the target's decl list, validating
// each insertion against the target container.
func applyAdd(file *ast.File, edit Edit) error {
	decls, container, err := targetDecls(file, edit.Target)
	if err != nil {
		return err
	}
	for j, ins := range edit.Insertions {
		if ins.IsZero() {
			return fmt.Errorf("insertion[%d] is zero", j)
		}
		if err := validateInsertion(container, ins); err != nil {
			return fmt.Errorf("insertion[%d]: %w", j, err)
		}
		seq.Append(decls, ins)
	}
	return nil
}

// applyDelete removes the target decl from its parent's decl list.
func applyDelete(file *ast.File, edit Edit) error {
	if edit.Target.IsZero() {
		return errors.New("target is zero")
	}
	parent, idx, ok := findInFile(file, edit.Target)
	if !ok {
		return errors.New("target not found in file")
	}
	parent.Delete(idx)
	return nil
}

// applyMove relocates target to immediately before Before. Both must
// be top-level decls.
func applyMove(file *ast.File, edit Edit) error {
	if edit.Target.IsZero() {
		return errors.New("target is zero")
	}
	if edit.Before.IsZero() {
		return errors.New("before is zero")
	}
	decls := file.Decls()
	targetIdx := indexOf(decls, edit.Target)
	if targetIdx < 0 {
		return errors.New("target not found at file level")
	}
	saved := decls.At(targetIdx)
	decls.Delete(targetIdx)
	beforeIdx := indexOf(decls, edit.Before)
	if beforeIdx < 0 {
		// Restore on failure so the file is left unchanged.
		decls.Insert(targetIdx, saved)
		return errors.New("before not found at file level")
	}
	decls.Insert(beforeIdx, saved)
	return nil
}

// containerKind classifies a target container for insertion-rule checks.
type containerKind int

const (
	containerInvalid containerKind = iota
	containerFile
	containerMessage
	containerEnum
	containerService
	containerOneof
	containerExtend
	containerMethod
)

func (c containerKind) String() string {
	switch c {
	case containerFile:
		return "file"
	case containerMessage:
		return "message body"
	case containerEnum:
		return "enum body"
	case containerService:
		return "service body"
	case containerOneof:
		return "oneof body"
	case containerExtend:
		return "extend body"
	case containerMethod:
		return "method body"
	default:
		return "invalid container"
	}
}

// targetDecls returns the decl list of the target container plus its
// classification. Target zero means the file itself. A bare
// [ast.DeclBody] (not associated with a definition) is treated
// permissively as a message body.
func targetDecls(file *ast.File, target ast.DeclAny) (seq.Inserter[ast.DeclAny], containerKind, error) {
	if target.IsZero() {
		return file.Decls(), containerFile, nil
	}
	if body := target.AsBody(); !body.IsZero() {
		return body.Decls(), containerMessage, nil
	}
	if def := target.AsDef(); !def.IsZero() {
		body := def.Body()
		if body.IsZero() {
			return nil, containerInvalid, fmt.Errorf("target %s has no body", def.Classify())
		}
		var ck containerKind
		switch def.Classify() {
		case ast.DefKindMessage:
			ck = containerMessage
		case ast.DefKindEnum:
			ck = containerEnum
		case ast.DefKindService:
			ck = containerService
		case ast.DefKindOneof:
			ck = containerOneof
		case ast.DefKindExtend:
			ck = containerExtend
		case ast.DefKindMethod:
			ck = containerMethod
		default:
			return nil, containerInvalid,
				fmt.Errorf("target def kind %s has no decl-list body", def.Classify())
		}
		return body.Decls(), ck, nil
	}
	return nil, containerInvalid, errors.New("target is not a body or definition")
}

// validateInsertion checks that an insertion is allowed in the given
// container per the rules documented on [Edit.Insertions].
func validateInsertion(container containerKind, ins ast.DeclAny) error {
	def := ins.AsDef()
	if def.IsZero() {
		// Non-definition decls (syntax, package, import, range, body,
		// empty) are not valid Edit insertions.
		return fmt.Errorf("only definition decls may be inserted (got %s)", ins.Kind())
	}
	kind := def.Classify()
	switch kind {
	case ast.DefKindOption:
		// Options are valid in any body.
		return nil
	case ast.DefKindMessage, ast.DefKindEnum:
		if container == containerFile || container == containerMessage {
			return nil
		}
	case ast.DefKindField:
		if container == containerMessage || container == containerOneof {
			return nil
		}
	case ast.DefKindEnumValue:
		if container == containerEnum {
			return nil
		}
	case ast.DefKindService:
		if container == containerFile {
			return nil
		}
	case ast.DefKindMethod:
		if container == containerService {
			return nil
		}
	case ast.DefKindOneof, ast.DefKindExtend, ast.DefKindGroup:
		if container == containerMessage {
			return nil
		}
	default:
		return fmt.Errorf("unsupported insertion kind: %s", kind)
	}
	return fmt.Errorf("cannot insert %s into %s", kind, container)
}

// findInFile recursively searches the file for target, returning the
// containing decl-list inserter and the index, or false if not found.
func findInFile(file *ast.File, target ast.DeclAny) (seq.Inserter[ast.DeclAny], int, bool) {
	if idx := indexOf(file.Decls(), target); idx >= 0 {
		return file.Decls(), idx, true
	}
	for d := range seq.Values(file.Decls()) {
		if found, idx, ok := findInDecl(d, target); ok {
			return found, idx, true
		}
	}
	return nil, 0, false
}

// findInDecl is the recursive worker for [findInFile]: searches decl's
// own body and any nested body decls for target.
func findInDecl(decl, target ast.DeclAny) (seq.Inserter[ast.DeclAny], int, bool) {
	var body ast.DeclBody
	if b := decl.AsBody(); !b.IsZero() {
		body = b
	} else if def := decl.AsDef(); !def.IsZero() {
		body = def.Body()
	}
	if body.IsZero() {
		return nil, 0, false
	}
	if idx := indexOf(body.Decls(), target); idx >= 0 {
		return body.Decls(), idx, true
	}
	for d := range seq.Values(body.Decls()) {
		if found, idx, ok := findInDecl(d, target); ok {
			return found, idx, true
		}
	}
	return nil, 0, false
}

// indexOf returns the index of target in decls, or -1.
func indexOf(decls seq.Indexer[ast.DeclAny], target ast.DeclAny) int {
	for i := range decls.Len() {
		if decls.At(i) == target {
			return i
		}
	}
	return -1
}
