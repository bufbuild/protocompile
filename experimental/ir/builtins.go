package ir

import (
	"fmt"
	"reflect"

	"github.com/bufbuild/protocompile/internal/arena"
	"github.com/bufbuild/protocompile/internal/intern"
)

// builtins contains those symbols that are built into the language, and which the compiler cannot
// handle not being present. This field is only present in the Context
// for descriptor.proto.
//
// This is resolved using reflection in [resolveLangSymbols]. The names of the
// fields of this type must match those in builtinIDs that names its symbol.
type builtins struct {
	FileOptions      Member
	MessageOptions   Member
	FieldOptions     Member
	OneofOptions     Member
	EnumOptions      Member
	EnumValueOptions Member

	MapEntry Member
}

// builtinIDs is all of the interning IDs of names in [builtins], plus some
// others. This lives inside of [Session] and is constructed once.
type builtinIDs struct {
	DescriptorFile intern.ID `intern:"google/protobuf/descriptor.proto"`
	AnyPath        intern.ID `intern:"google.protobuf.Any"`

	FileOptions      intern.ID `intern:"google.protobuf.FileDescriptorProto.options"`
	MessageOptions   intern.ID `intern:"google.protobuf.DescriptorProto.options"`
	FieldOptions     intern.ID `intern:"google.protobuf.FieldDescriptorProto.options"`
	OneofOptions     intern.ID `intern:"google.protobuf.OneofDescriptorProto.options"`
	EnumOptions      intern.ID `intern:"google.protobuf.EnumDescriptorProto.options"`
	EnumValueOptions intern.ID `intern:"google.protobuf.EnumValueDescriptorProto.options"`

	MapEntry intern.ID `intern:"google.protobuf.MessageOptions.map_entry"`
}

func resolveLangSymbols(c *Context) {
	if !c.File().IsDescriptorProto() {
		return
	}

	kinds := map[reflect.Type]struct {
		kind SymbolKind
		wrap func(arena.Untyped, reflect.Value)
	}{
		reflect.TypeFor[Member](): {
			kind: SymbolKindField,
			wrap: makeBuiltinWrapper(c, wrapMember),
		},
	}

	c.dpBuiltins = new(builtins)
	v := reflect.ValueOf(c.dpBuiltins).Elem()
	ids := reflect.ValueOf(c.session.builtins)
	for i := range v.NumField() {
		field := v.Field(i)
		id := ids.FieldByName(v.Type().Field(i).Name).Interface().(intern.ID)
		kind := kinds[field.Type()]

		ref := c.exported.lookup(c, id)
		sym := wrapSymbol(c, ref)
		if sym.Kind() != kind.kind {
			panic(fmt.Errorf(
				"missing descriptor.proto symbol: %s `%s`; got kind %s",
				kind.kind.noun(), c.session.intern.Value(id), sym.Kind(),
			))
		}
		kind.wrap(sym.raw.data, field)
	}
}

// makeBuiltinWrapper helps construct reflection shims for resolveBuiltins.
func makeBuiltinWrapper[T any, Raw any](c *Context, wrap func(*Context, ref[Raw]) T) func(arena.Untyped, reflect.Value) {
	return func(p arena.Untyped, out reflect.Value) {
		x := wrap(c, ref[Raw]{ptr: arena.Pointer[Raw](p)})
		out.Set(reflect.ValueOf(x))
	}
}
