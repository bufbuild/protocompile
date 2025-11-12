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

package ir

import (
	"fmt"
	"iter"

	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/id"
	"github.com/bufbuild/protocompile/experimental/internal/taxa"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/experimental/token/keyword"
	"github.com/bufbuild/protocompile/internal/ext/iterx"
	"github.com/bufbuild/protocompile/internal/intern"
)

// resolveEarlyOptions resolves options whose values must be discovered very
// early during compilation. This does not create option values, nor does it
// generate diagnostics; it simply records this information to special fields
// in [Type].
func resolveEarlyOptions(file *File) {
	builtins := &file.session.builtins
	for ty := range seq.Values(file.AllTypes()) {
		for decl := range seq.Values(ty.AST().Body().Decls()) {
			def := decl.AsDef()
			if def.IsZero() || def.Classify() != ast.DefKindOption {
				continue
			}
			option := def.AsOption().Option

			// If this option's path has more than one component, skip.
			first, ok := iterx.OnlyOne(option.Path.Components)
			if !ok || !first.Separator().IsZero() {
				continue
			}

			// Resolve the name of this option.
			var name intern.ID
			if ident := first.AsIdent(); !ident.IsZero() {
				switch ident.Text() {
				case "message_set_wire_format":
					name = builtins.MessageSet
				case "allow_alias":
					name = builtins.AllowAlias
				}
			} else if extn := first.AsExtension(); !extn.IsZero() {
				sym, _ := file.imported.resolve(
					file,
					ty.Scope(),
					FullName(extn.Canonicalized()),
					nil,
					nil,
				)
				name = GetRef(file, sym).AsMember().InternedFullName()
			}

			// Get the value of this option. We only care about a value of
			// "true" for both options.
			value := option.Value.AsPath().AsKeyword() == keyword.True

			switch name {
			case builtins.MessageSet:
				ty.Raw().isMessageSet = ty.IsMessage() && value
			case builtins.AllowAlias:
				ty.Raw().allowsAlias = ty.IsEnum() && value
			}
		}
	}
}

// resolveOptions resolves all of the options in a file.
func resolveOptions(file *File, r *report.Report) {
	builtins := file.builtins()
	bodyOptions := func(decls seq.Inserter[ast.DeclAny]) iter.Seq[ast.Option] {
		return iterx.FilterMap(seq.Values(decls), func(d ast.DeclAny) (ast.Option, bool) {
			def := d.AsDef()
			if def.IsZero() || def.Classify() != ast.DefKindOption {
				return ast.Option{}, false
			}
			return def.AsOption().Option, true
		})
	}

	for def := range bodyOptions(file.AST().Decls()) {
		optionRef{
			File:   file,
			Report: r,

			scope: file.Package(),
			def:   def,

			field: builtins.FileOptions,
			raw:   &file.options,
		}.resolve()
	}

	// Reusable space for duplicating options values between extension ranges.
	extnOpts := make(map[ast.DeclRange]id.ID[Value])
	for ty := range seq.Values(file.AllTypes()) {
		if !ty.MapField().IsZero() {
			// Map entries already come with options pre-calculated.
			continue
		}

		for def := range bodyOptions(ty.AST().Body().Decls()) {
			options := builtins.MessageOptions
			if ty.IsEnum() {
				options = builtins.EnumOptions
			}
			optionRef{
				File:   file,
				Report: r,

				scope: ty.Scope(),
				def:   def,

				field: options,
				raw:   &ty.Raw().options,
			}.resolve()
		}

		for field := range seq.Values(ty.Members()) {
			for def := range seq.Values(field.AST().Options().Entries()) {
				options := builtins.FieldOptions
				if ty.IsEnum() {
					options = builtins.EnumValueOptions
				}
				optionRef{
					File:   file,
					Report: r,

					scope: field.Scope(),
					def:   def,

					field:  options,
					raw:    &field.Raw().options,
					target: field,
				}.resolve()
			}
		}
		for oneof := range seq.Values(ty.Oneofs()) {
			for def := range bodyOptions(oneof.AST().Body().Decls()) {
				optionRef{
					File:   file,
					Report: r,

					scope: ty.Scope(),
					def:   def,

					field: builtins.OneofOptions,
					raw:   &oneof.Raw().options,
				}.resolve()
			}
		}

		clear(extnOpts)
		for extns := range seq.Values(ty.ExtensionRanges()) {
			decl := extns.DeclAST()
			if p := extnOpts[decl]; !p.IsZero() {
				extns.Raw().options = p
				continue
			}

			for def := range seq.Values(extns.DeclAST().Options().Entries()) {
				optionRef{
					File:   file,
					Report: r,

					scope: ty.Scope(),
					def:   def,

					field: builtins.RangeOptions,
					raw:   &extns.Raw().options,
				}.resolve()
			}

			extnOpts[decl] = extns.Raw().options
		}
	}
	for field := range seq.Values(file.AllExtensions()) {
		for def := range seq.Values(field.AST().Options().Entries()) {
			optionRef{
				File:   file,
				Report: r,

				scope: field.Scope(),
				def:   def,

				field:  builtins.FieldOptions,
				raw:    &field.Raw().options,
				target: field,
			}.resolve()
		}
	}
	for service := range seq.Values(file.Services()) {
		for def := range bodyOptions(service.AST().Body().Decls()) {
			optionRef{
				File:   file,
				Report: r,

				scope: service.FullName(),
				def:   def,

				field: builtins.ServiceOptions,
				raw:   &service.Raw().options,
			}.resolve()
		}

		for method := range seq.Values(service.Methods()) {
			for def := range bodyOptions(method.AST().Body().Decls()) {
				optionRef{
					File:   file,
					Report: r,

					scope: service.FullName(),
					def:   def,

					field: builtins.MethodOptions,
					raw:   &method.Raw().options,
				}.resolve()
			}
		}
	}
}

// populateOptionTargets builds option target sets for each field in a file.
func populateOptionTargets(file *File, _ *report.Report) {
	targets := file.builtins().OptionTargets
	populate := func(m Member) {
		for target := range seq.Values(m.Options().Field(targets).Elements()) {
			n, _ := target.AsInt()
			target := OptionTarget(n)
			if target == OptionTargetInvalid || target >= optionTargetMax {
				continue
			}

			m.Raw().optionTargets |= 1 << target
		}
	}

	for ty := range seq.Values(file.AllTypes()) {
		if !ty.IsMessage() {
			continue
		}

		for field := range seq.Values(ty.Members()) {
			populate(field)
		}
	}

	for extn := range seq.Values(file.AllExtensions()) {
		populate(extn)
	}
}

func validateOptionTargets(f *File, r *report.Report) {
	validateOptionTargetsInValue(f.Options(), source.Span{}, OptionTargetFile, r)

	for ty := range seq.Values(f.AllTypes()) {
		tyTarget, memberTarget := OptionTargetMessage, OptionTargetField
		if ty.IsEnum() {
			tyTarget, memberTarget = OptionTargetEnum, OptionTargetEnumValue
		}
		validateOptionTargetsInValue(ty.Options(), ty.AST().Name().Span(), tyTarget, r)
		for member := range seq.Values(ty.Members()) {
			validateOptionTargetsInValue(member.Options(), member.AST().Name().Span(), memberTarget, r)
		}
		for oneof := range seq.Values(ty.Oneofs()) {
			validateOptionTargetsInValue(oneof.Options(), oneof.AST().Name().Span(), OptionTargetOneof, r)
		}
	}

	for extn := range seq.Values(f.AllExtensions()) {
		validateOptionTargetsInValue(extn.Options(), extn.AST().Name().Span(), OptionTargetField, r)
	}
}

func validateOptionTargetsInValue(m MessageValue, decl source.Span, target OptionTarget, r *report.Report) {
	if m.IsZero() {
		return
	}

	if c := m.Concrete(); c != m {
		validateOptionTargetsInValue(c, decl, target, r)
	}

	for value := range m.Fields() {
		field := value.Field()
		if !field.CanTarget(target) {
			var nouns taxa.Set
			var targets int
			for target := range field.Targets() {
				switch target {
				case OptionTargetFile:
					nouns = nouns.With(taxa.TopLevel)
				case OptionTargetRange:
					nouns = nouns.With(taxa.Extensions)
				case OptionTargetMessage:
					nouns = nouns.With(taxa.Message)
				case OptionTargetEnum:
					nouns = nouns.With(taxa.Enum)
				case OptionTargetField:
					nouns = nouns.With(taxa.Field)
				case OptionTargetEnumValue:
					nouns = nouns.With(taxa.EnumValue)
				case OptionTargetOneof:
					nouns = nouns.With(taxa.Oneof)
				case OptionTargetService:
					nouns = nouns.With(taxa.Service)
				case OptionTargetMethod:
					nouns = nouns.With(taxa.Method)
				}
				targets++
			}

			// Pull out the place where this option was set so we can show it to
			// the user.
			constraints := field.Options().Field(m.Context().builtins().OptionTargets)

			key := value.KeyAST()
			span := key.Span()
			if path := key.AsPath(); !path.IsZero() {
				// Pull out the last component.
				// TODO: write a function on Path that does this cheaply.
				last, _ := iterx.Last(path.Components)
				span = last.Name().Span()
			}

			d := r.Errorf("unsupported option target for `%s`", field.Name()).Apply(
				report.Snippetf(span, "option set here"),
				report.Snippetf(decl, "applied to this"),
				report.Snippetf(constraints.ValueAST(), "targets constrained here"),
			)
			if targets == 1 {
				d.Apply(report.Helpf(
					"`%s` is constrained to %ss",
					field.FullName(),
					nouns.Join("or")))
			} else {
				d.Apply(report.Helpf(
					"`%s` is constrained to one of %s",
					field.FullName(),
					nouns.Join("or")))
			}

			continue // Don't recurse and generate a mess of diagnostics.
		}

		validateOptionTargetsInValue(value.AsMessage(), decl, target, r)
	}
}

// symbolRef is all of the information necessary to resolve an option reference.
type optionRef struct {
	*File
	*report.Report

	scope FullName
	def   ast.Option

	field Member
	raw   *id.ID[Value]

	// A member being annotated. This is used for pseudo-option resolution.
	target Member
}

// resolve performs symbol resolution.
func (r optionRef) resolve() {
	ids := &r.session.builtins
	root := r.field.Element()

	if r.raw.IsZero() {
		*r.raw = newMessage(r.File, r.field.toRef(r.File)).AsValue().ID()
	}

	current := id.Wrap(r.File, *r.raw)
	field := current.Field()
	var path ast.Path
	var raw slot
	for pc := range r.def.Path.Components {
		// If this is the first iteration, use the *Options value as the current
		// message.
		message := field.Element()
		if message.IsZero() {
			message = root
		}

		// Calculate the corresponding member for this path component, which may
		// be either a simple path or an extension name.
		prev := field
		pseudo := pc.IsFirst() &&
			r.field.InternedFullName() == ids.FieldOptions &&
			pc.AsIdent().Keyword().IsPseudoOption()
		if pseudo {
			// Check if this is a pseudo-option.
			m := current.AsMessage()
			switch pc.AsIdent().Keyword() {
			case keyword.Default:
				field = r.target
				raw = slot{m, &m.Raw().pseudo.defaultValue}

			case keyword.JsonName:
				field = r.builtins().JSONName
				raw = slot{m, &current.AsMessage().Raw().pseudo.jsonName}
			}
		} else if extn := pc.AsExtension(); !extn.IsZero() {
			sym := symbolRef{
				File:   r.File,
				Report: r.Report,

				span:  extn,
				scope: r.scope,
				name:  FullName(extn.Canonicalized()),

				accept: SymbolKind.IsMessageField,
				want:   taxa.Extension,

				allowScalars:  false,
				suggestImport: true,
			}.resolve()

			if !sym.Kind().IsMessageField() {
				// Already diagnosed by resolve().
				return
			}

			field = sym.AsMember()
			if field.Container() != message {
				d := r.Errorf("expected `%s` extension, found %s in `%s`",
					message.FullName(), field.noun(), field.Container().FullName(),
				).Apply(
					report.Snippetf(pc, "because of this %s", taxa.FieldSelector),
					report.Snippetf(field.AST().Name(), "`%s` defined here", field.FullName()),
				)
				if field.IsExtension() {
					d.Apply(report.Snippetf(field.Extend().AST(), "... within this %s", taxa.Extend))
				} else {
					d.Apply(report.Snippetf(field.Container().AST(), "... within this %s", taxa.Message))
				}

				return
			}

			if !field.IsExtension() {
				// Protoc accepts this! The horror!
				r.Warnf("redundant %s syntax", taxa.CustomOption).Apply(
					report.Snippetf(pc, "this field is not a %s", taxa.Extension),
					report.Snippetf(field.AST().Name(), "field declared inside of `%s` here", field.Parent().FullName()),
					report.Helpf("%s syntax should only be used with %ss", taxa.CustomOption, taxa.Extension),
					report.SuggestEdits(pc.Name(), fmt.Sprintf("replace %s with a field name", taxa.Parens), report.Edit{
						Start: 0, End: pc.Name().Span().Len(),
						Replace: field.Name(),
					}),
				)
			}
		} else if ident := pc.AsIdent(); !ident.IsZero() {
			field = message.MemberByName(ident.Text())
			if field.IsZero() {
				d := r.Errorf("cannot find %s `%s` in `%s`", taxa.Field, ident.Text(), message.FullName()).Apply(
					report.Snippetf(pc, "because of this %s", taxa.FieldSelector),
				)
				if !pc.IsFirst() {
					d.Apply(report.Snippetf(prev.AST().Type(), "`%s` specified here", message.FullName()))
				}
				return
			}
		}

		if pc.IsFirst() {
			switch field.InternedFullName() {
			case ids.MapEntry:
				r.Errorf("`map_entry` cannot be set explicitly").Apply(
					report.Snippet(pc),
					report.Helpf("`map_entry` is set automatically for synthetic map "+
						"entry types, and cannot be set with an %s", taxa.Option),
				)

			case ids.FileUninterpreted,
				ids.MessageUninterpreted, ids.FieldUninterpreted, ids.OneofUninterpreted, ids.RangeUninterpreted,
				ids.EnumUninterpreted, ids.EnumValueUninterpreted,
				ids.MethodUninterpreted, ids.ServiceUninterpreted:
				r.Errorf("`uninterpreted_option` cannot be set explicitly").Apply(
					report.Snippet(pc),
					report.Helpf("`uninterpreted_option` is an implementation detail of protoc"),
				)

			case ids.FileFeatures,
				ids.MessageFeatures, ids.FieldFeatures, ids.OneofFeatures,
				ids.EnumFeatures, ids.EnumValueFeatures:
				if syn := r.Syntax(); !syn.IsEdition() {
					r.Errorf("`features` cannot be set in %s", prettyEdition(syn)).Apply(
						report.Snippet(pc),
						report.Snippetf(r.AST().Syntax().Value(), "syntax specified here"),
					)
				}
			}
		}

		path, _ = pc.SplitAfter()

		// Check to see if this value has already been set in the parent message.
		// We have already validated current as a singular message by this point.
		parent := current.AsMessage()

		// Check if this field is already set. The only cases where this is
		// allowed is if:
		//
		// 1. The current field is repeated and this is the last component.
		// 2. The current field is of message type and this is not the last
		//    component.
		if !pseudo {
			raw = parent.slot(field)
		}
		if !raw.IsZero() {
			value := raw.Value()
			switch {
			case field.IsRepeated():
				break // Handled below.

			case value.Field() != field:
				// A different member of a oneof was set.
				r.Error(errSetMultipleTimes{
					member: field.Oneof(),
					first:  value.OptionPaths().At(0),
					second: path,
					root:   pc.IsFirst(),
				})
				return

			case prev.Element().IsMessage():
				if !pc.IsLast() {
					current = value
					continue
				}
				fallthrough

			default:
				r.Error(errSetMultipleTimes{
					member: field,
					first:  value.OptionPaths().At(0),
					second: path,
					root:   pc.IsFirst(),
				})
				return
			}
		}

		if pc.IsLast() {
			break
		}

		// Handle a non-final component in an option path. That must be
		// a singular message value, which the successive elements of the
		// path index into as field names.
		message = field.Element()

		// This diagnoses that people do not write option a.b.c where b is
		// a repeated field.
		if field.IsRepeated() {
			r.Error(errOptionMustBeMessage{
				selector: pc.Next().Name(),
				prev:     pc.Name(),
				got:      "repeated",
				gotName:  message.FullName(),
				spec:     field.TypeAST(),
			})
			return
		}

		// This diagnoses that people do not write option a.b.c where b is
		// not a message field.
		if !message.IsZero() && !message.IsMessage() {
			r.Error(errOptionMustBeMessage{
				selector: pc.Next().Name(),
				prev:     pc.Name(),
				got:      message.noun(),
				gotName:  message.FullName(),
				spec:     field.TypeAST().RemovePrefixes(),
			})
			return
		}

		value := newMessage(r.File, field.toRef(r.File)).AsValue()
		value.Raw().optionPaths = append(value.Raw().optionPaths, path.ID())
		value.Raw().exprs = append(value.Raw().exprs, ast.ExprPath{Path: path}.AsAny().ID())

		raw.Insert(value)
		current = value
	}

	// Now, evaluate the expression and assign it to the field we found.
	evaluator := evaluator{
		File:   r.File,
		Report: r.Report,
		scope:  r.scope,
	}
	args := evalArgs{
		expr:       r.def.Value,
		field:      field,
		annotation: field.AST().Type(),
		optionPath: path,
	}

	if !raw.IsZero() {
		args.target = raw.Value()
	}

	v := evaluator.eval(args)
	if raw.IsZero() && !v.IsZero() {
		raw.Insert(v)
	}
}

type errSetMultipleTimes struct {
	member        any
	first, second source.Spanner
	root          bool
}

func (e errSetMultipleTimes) Diagnose(d *report.Diagnostic) {
	var what any
	var name FullName
	var note string
	var def source.Spanner
	switch member := e.member.(type) {
	case Member:
		if !member.IsExtension() && e.root {
			// For non-custom options, use the short name and call it
			// an "option".
			name = FullName(member.Name())
			what = "option"
		} else {
			name = member.FullName()
			what = member.noun()
		}
		note = "a non-`repeated` option may be set at most once"
		def = member.AST().Name()
	case Oneof:
		name = member.FullName()
		what = "oneof"
		note = "at most one member of a oneof may be set by an option"
		def = member.AST().Name()
	default:
		panic("unreachable")
	}

	d.Apply(
		report.Message("%v `%v` set multiple times", what, name),
		report.Snippetf(e.second, "... also set here"),
		report.Snippetf(e.first, "first set here..."),
		report.Snippetf(def, "not a repeated field"),
		report.Notef(note),
	)
}

type errOptionMustBeMessage struct {
	selector, prev, spec source.Spanner
	got, gotName         any
}

func (e errOptionMustBeMessage) Diagnose(d *report.Diagnostic) {
	got := e.got
	if e.gotName != nil {
		got = fmt.Sprintf("%v `%v`", got, e.gotName)
	}

	d.Apply(
		report.Message("expected singular message, found %s", got),
		report.Snippetf(e.selector, "%s requires singular message", taxa.FieldSelector),
		report.Snippetf(e.prev, "found %s", got),
	)

	if e.spec != nil {
		d.Apply(report.Snippetf(e.spec, "type specified here"))
	}
}
