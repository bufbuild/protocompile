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
	dpIdx := int32(len(f.Context().imports.files))
	dp := f.Context().imports.DescriptorProto().Context()
	fileOptions := ref[rawField]{dpIdx, dp.langSymbols.fileOptions}
	messageOptions := ref[rawField]{dpIdx, dp.langSymbols.messageOptions}
	fieldOptions := ref[rawField]{dpIdx, dp.langSymbols.fieldOptions}
	oneofOptions := ref[rawField]{dpIdx, dp.langSymbols.oneofOptions}
	enumOptions := ref[rawField]{dpIdx, dp.langSymbols.enumOptions}
	enumValueOptions := ref[rawField]{dpIdx, dp.langSymbols.enumValueOptions}

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

			field: fileOptions,
			raw:   &f.Context().options,
		}.resolve()
	}

	for ty := range seq.Values(f.AllTypes()) {
		for ast := range bodyOptions(ty.AST().Body()) {
			options := messageOptions
			if ty.IsEnum() {
				options = enumOptions
			}
			optionRef{
				Context: f.Context(),
				Report:  r,

				scope: ty.Scope(),
				def:   ast,

				field: options,
				raw:   &ty.raw.options,
			}.resolve()
		}
		for field := range seq.Values(ty.Fields()) {
			for ast := range seq.Values(field.AST().Options().Entries()) {
				options := fieldOptions
				if ty.IsEnum() {
					options = enumValueOptions
				}
				optionRef{
					Context: f.Context(),
					Report:  r,

					scope: field.Scope(),
					def:   ast,

					field: options,
					raw:   &field.raw.options,
				}.resolve()
			}
		}
		for oneof := range seq.Values(ty.Oneofs()) {
			for ast := range bodyOptions(oneof.AST().Body()) {
				optionRef{
					Context: f.Context(),
					Report:  r,

					scope: ty.Scope(),
					def:   ast,

					field: oneofOptions,
					raw:   &oneof.raw.options,
				}.resolve()
			}
		}
	}
	for field := range seq.Values(f.AllExtensions()) {
		for ast := range seq.Values(field.AST().Options().Entries()) {
			optionRef{
				Context: f.Context(),
				Report:  r,

				scope: field.Scope(),
				def:   ast,

				field: fieldOptions,
				raw:   &field.raw.options,
			}.resolve()
		}
	}
}

// symbolRef is all of the information necessary to resolve an option reference.
type optionRef struct {
	*Context
	*report.Report

	scope FullName
	def   ast.Option

	field ref[rawField]
	raw   *arena.Pointer[rawValue]
}

// resolve performs symbol resolution.
func (r optionRef) resolve() {
	root := wrapField(r.Context, r.field).Element()

	// Check if this is a pseudo-option, and diagnose if it has multiple
	// components. The values of pseudo-options are calculated elsewhere; this
	// is only for diagnostics.
	dp := r.imports.DescriptorProto().Context()
	if r.field.ptr == dp.langSymbols.fieldOptions {
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
		v := newMessage(r.Context, r.field, ref[rawType]{})
		*r.raw = r.arenas.values.Compress(v.raw)
	}

	current := wrapValue(r.Context, *r.raw)
	for pc := range r.def.Path.Components {
		field := current.Field()
		message := field.Element()
		if !message.IsZero() && !message.IsMessage() {
			r.Error(errOptionMustBeMessage{
				selector: pc,
				got:      message.noun(),
				gotName:  message.FullName(),
				spec:     field.AST().Type(),
			})
			return
		}
		if field.Presence() == presence.Repeated {
			r.Error(errOptionMustBeMessage{
				selector: pc,
				got:      "repeated",
				gotName:  message.FullName(),
				spec:     field.AST().Type(),
			})
			return
		}

		if message.IsZero() {
			message = root
		}

		var next Field
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

			next = sym.AsField()
			if next.Container() != message {
				d := r.Errorf("expected `%s` extension, found %s in `%s`",
					message.FullName(), next.noun(), next.Container().FullName(),
				).Apply(
					report.Snippetf(pc, "because of this %s", taxa.FieldSelector),
					report.Snippetf(next.AST().Name(), "`%s` defined here", next.FullName()),
				)
				if next.IsExtension() {
					extendee := r.arenas.extendees.Deref(next.raw.extendee)
					d.Apply(report.Snippetf(extendee.def, "... within this %s", taxa.Extend))
				} else {
					d.Apply(report.Snippetf(next.Container().AST(), "... within this %s", taxa.Message))
				}

				return
			}

			if !next.IsExtension() {
				// Protoc accepts this! The horror!
				r.Warnf("redundant %s syntax", taxa.CustomOption).Apply(
					report.Snippetf(pc, "this field is not a %s", taxa.Extension),
					report.Snippetf(next.AST().Name(), "field declared inside of `%s` here", next.Parent().FullName()),
					report.Helpf("%s syntax should only be used with %ss", taxa.CustomOption, taxa.Extension),
					report.SuggestEdits(pc.Name(), fmt.Sprintf("replace %s with a field name", taxa.Parens), report.Edit{
						Start: 0, End: pc.Name().Span().Len(),
						Replace: next.Name(),
					}),
				)
			}
		} else if ident := pc.AsIdent(); !ident.IsZero() {
			next = message.FieldByName(ident.Text())
			if next.IsZero() {
				d := r.Errorf("cannot find %s `%s` in `%s`", taxa.Field, ident.Text(), message.FullName()).Apply(
					report.Snippetf(pc, "because of this %s", taxa.FieldSelector),
				)
				if !pc.IsFirst() {
					d.Apply(report.Snippetf(field.AST().Type(), "`%s` specified here", message.FullName()))
				}
				return
			}
		}

		path, _ := pc.SplitAfter()

		// Check to see if this value has already been set in the parent message.
		// We have already validated current as a singular message by this point.
		parent := current.AsMessage()

		// Check if this field is already set. The only cases where this is
		// allowed is if:
		//
		// 1. The current field is repeated and this is the last component.
		// 2. The current field is of message type and this is not the last
		//    component.
		raw := parent.insert(next)
		if !raw.Nil() {
			value := wrapValue(r.Context, *raw)
			switch {
			case next.Presence() == presence.Repeated:
				// TODO: Implement expression evaluation.
			case value.Field() != next:
				// A different member of a oneof was set.
				r.Errorf("oneof `%s` set multiple times", next.Oneof().FullName()).Apply(
					report.Snippetf(path, "... also set here"),
					report.Snippetf(value.OptionPath(), "first set here..."),
					report.Notef("at most one member of a oneof may be set by an option"),
				)
				return

			case field.Element().IsMessage():
				if !pc.IsLast() {
					current = value
					continue
				}
				fallthrough

			default:
				name := next.FullName()
				what := any(next.noun())
				if !next.IsExtension() && pc.IsFirst() {
					// For non-custom options, use the short name and call it
					// an "option".
					name = FullName(next.Name())
					what = "option"
				}

				r.Errorf("%v `%v` set multiple times", what, name).Apply(
					report.Snippetf(path, "... also set here"),
					report.Snippetf(value.OptionPath(), "first set here..."),
					report.Notef("an option may be set at most once"),
				)
				return
			}
		}

		// Construct a new value for this option.
		var fieldRef ref[rawField]
		if next.Context() != r.Context {
			fieldRef.file = int32(r.imports.byPath[next.Context().File().InternedPath()] + 1)
		}
		fieldRef.ptr = next.Context().arenas.fields.Compress(next.raw)

		// TODO: Implement expression evaluation.
		var value Value
		if next.Element().IsMessage() {
			value = newMessage(r.Context, fieldRef, ref[rawType]{})
		} else {
			// Just set the zero value; all scalars with a value of zero
			// are well-defined.
			value = newScalar[int32](r.Context, fieldRef, 0)
		}
		if value.raw.optionPath.IsZero() {
			value.raw.optionPath = path
		}

		*raw = r.arenas.values.Compress(value.raw)
		current = value
	}
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
