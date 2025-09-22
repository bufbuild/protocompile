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
	"slices"

	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/internal/taxa"
	"github.com/bufbuild/protocompile/experimental/ir/presence"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/internal/arena"
	"github.com/bufbuild/protocompile/internal/ext/iterx"
)

// resolveOptions resolves all of the options in a file.
func resolveOptions(f File, r *report.Report) {
	builtins := f.Context().builtins()
	bodyOptions := func(b ast.DeclBody) iter.Seq[ast.Option] {
		return iterx.FilterMap(seq.Values(b.Decls()), func(d ast.DeclAny) (ast.Option, bool) {
			def := d.AsDef()
			if def.IsZero() || def.Classify() != ast.DefKindOption {
				return ast.Option{}, false
			}
			return def.AsOption().Option, true
		})
	}

	for def := range bodyOptions(f.AST().DeclBody) {
		optionRef{
			Context: f.Context(),
			Report:  r,

			scope: f.Package(),
			def:   def,

			field: builtins.FileOptions,
			raw:   &f.Context().options,
		}.resolve()
	}

	for ty := range seq.Values(f.AllTypes()) {
		if !ty.MapField().IsZero() {
			// Map entries already come with options pre-calculated.
			continue
		}

		for def := range bodyOptions(ty.AST().Body()) {
			options := builtins.MessageOptions
			if ty.IsEnum() {
				options = builtins.EnumOptions
			}
			optionRef{
				Context: f.Context(),
				Report:  r,

				scope: ty.Scope(),
				def:   def,

				field: options,
				raw:   &ty.raw.options,
			}.resolve()
		}
		for field := range seq.Values(ty.Members()) {
			for def := range seq.Values(field.AST().Options().Entries()) {
				options := builtins.FieldOptions
				if ty.IsEnum() {
					options = builtins.EnumValueOptions
				}
				optionRef{
					Context: f.Context(),
					Report:  r,

					scope: field.Scope(),
					def:   def,

					field: options,
					raw:   &field.raw.options,
				}.resolve()
			}
		}
		for oneof := range seq.Values(ty.Oneofs()) {
			for def := range bodyOptions(oneof.AST().Body()) {
				optionRef{
					Context: f.Context(),
					Report:  r,

					scope: ty.Scope(),
					def:   def,

					field: builtins.OneofOptions,
					raw:   &oneof.raw.options,
				}.resolve()
			}
		}
	}
	for field := range seq.Values(f.AllExtensions()) {
		for def := range seq.Values(field.AST().Options().Entries()) {
			optionRef{
				Context: f.Context(),
				Report:  r,

				scope: field.Scope(),
				def:   def,

				field: builtins.FieldOptions,
				raw:   &field.raw.options,
			}.resolve()
		}
	}
	for service := range seq.Values(f.Services()) {
		for def := range bodyOptions(service.AST().Body()) {
			optionRef{
				Context: f.Context(),
				Report:  r,

				scope: service.FullName(),
				def:   def,

				field: builtins.ServiceOptions,
				raw:   &service.raw.options,
			}.resolve()
		}

		for method := range seq.Values(service.Methods()) {
			for def := range bodyOptions(method.AST().Body()) {
				optionRef{
					Context: f.Context(),
					Report:  r,

					scope: service.FullName(),
					def:   def,

					field: builtins.MethodOptions,
					raw:   &method.raw.options,
				}.resolve()
			}
		}
	}
}

// symbolRef is all of the information necessary to resolve an option reference.
type optionRef struct {
	*Context
	*report.Report

	scope FullName
	def   ast.Option

	field Member
	raw   *arena.Pointer[rawValue]
}

// resolve performs symbol resolution.
func (r optionRef) resolve() {
	root := r.field.Element()

	// Check if this is a pseudo-option, and diagnose if it has multiple
	// components. The values of pseudo-options are calculated elsewhere; this
	// is only for diagnostics.
	if r.field.InternedFullName() == r.session.builtins.FieldOptions {
		var buf [2]ast.PathComponent
		prefix := slices.AppendSeq(buf[:0], iterx.Take(r.def.Path.Components, 2))

		if kw := buf[0].AsIdent().Keyword(); kw.IsPseudoOption() {
			if len(prefix) > 1 {
				r.Error(errOptionMustBeMessage{
					selector: buf[1],
					got:      taxa.PseudoOption,
					gotName:  kw,
				}).Apply(report.Notef(
					"`%s` is a %s and does not correspond to a field in `%s`",
					kw, taxa.PseudoOption, root.FullName(),
				))
			}

			return
		}
	}

	if r.raw.Nil() {
		v := newMessage(r.Context, r.field.toRef(r.Context)).AsValue()
		*r.raw = r.arenas.values.Compress(v.raw)
	}

	current := wrapValue(r.Context, *r.raw)
	field := current.Field()
	var path ast.Path
	var raw *arena.Pointer[rawValue]
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
		if extn := pc.AsExtension(); !extn.IsZero() {
			sym := symbolRef{
				Context: r.Context,
				Report:  r.Report,

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
					extendee := r.arenas.extendees.Deref(field.raw.extendee)
					d.Apply(report.Snippetf(extendee.def, "... within this %s", taxa.Extend))
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

		// TODO: Forbid any of the uninterpreted_option options from being set,
		// and any of the features options from being set if not in editions mode.
		if pc.IsFirst() && field.InternedFullName() == r.session.builtins.MapEntry {
			r.Errorf("`map_entry` cannot be set explicitly").Apply(
				report.Snippet(pc),
				report.Helpf("map_entry is set automatically for synthetic map "+
					"entry types, and cannot be set with an %s", taxa.Option),
			)
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
		raw = parent.insert(field)
		if !raw.Nil() {
			value := wrapValue(r.Context, *raw)
			switch {
			case field.Presence() == presence.Repeated:
				break // Handled below.

			case value.Field() != field:
				// A different member of a oneof was set.
				r.Error(errSetMultipleTimes{
					member: field.Oneof(),
					first:  value.OptionPath(),
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
					first:  value.OptionPath(),
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
		// not a message field.
		if !message.IsZero() && !message.IsMessage() {
			r.Error(errOptionMustBeMessage{
				selector: pc.Next(),
				got:      message.noun(),
				gotName:  message.FullName(),
				spec:     field.AST().Type(),
			})
			return
		}

		// This diagnoses that people do not write option a.b.c where b is
		// a repeated field.
		if field.Presence() == presence.Repeated {
			r.Error(errOptionMustBeMessage{
				selector: pc.Next(),
				got:      "repeated",
				gotName:  message.FullName(),
				spec:     field.AST().Type(),
			})
			return
		}

		value := newMessage(r.Context, field.toRef(r.Context)).AsValue()
		if value.raw.optionPath.IsZero() {
			value.raw.optionPath = path
		}

		*raw = r.arenas.values.Compress(value.raw)
		current = value
	}

	// Now, evaluate the expression and assign it to the field we found.
	evaluator := evaluator{
		Context: r.Context,
		Report:  r.Report,
		scope:   r.scope,
	}
	args := evalArgs{
		expr:       r.def.Value,
		field:      field,
		annotation: field.AST().Type(),
		optionPath: path,
	}

	if !raw.Nil() {
		args.target = wrapValue(r.Context, *raw)
	}

	v := evaluator.eval(args)
	if !v.IsZero() {
		*raw = r.arenas.values.Compress(v.raw)
	}
}

type errSetMultipleTimes struct {
	member        any
	first, second report.Spanner
	root          bool
}

func (e errSetMultipleTimes) Diagnose(d *report.Diagnostic) {
	var what any
	var name FullName
	var note string
	var def report.Spanner
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
	selector, spec report.Spanner
	got, gotName   any
}

func (e errOptionMustBeMessage) Diagnose(d *report.Diagnostic) {
	got := e.got
	if e.gotName != nil {
		got = fmt.Sprintf("%v `%v`", got, e.gotName)
	}

	d.Apply(
		report.Message("expected singular message, found %s", got),
		report.Snippetf(e.selector, "%s requires singular message", taxa.FieldSelector),
	)

	if e.spec != nil {
		d.Apply(report.Snippetf(e.spec, "type specified here"))
	}
}
