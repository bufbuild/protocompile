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
	"slices"
	"sync"

	"github.com/bufbuild/protocompile/experimental/internal"
	"github.com/bufbuild/protocompile/experimental/ir/presence"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/seq"
	pcinternal "github.com/bufbuild/protocompile/internal"
)

// generateMapEntries generates map entry types for all map-typed fields.
func generateMapEntries(f File, r *report.Report) {
	c := f.Context()
	lowerField := func(field Member) {
		// optional, repeated etc. on map types is already legalized in
		// the parser.
		decl := field.AST().Type().RemovePrefixes().AsGeneric()
		if decl.IsZero() {
			return
		}

		key, _ := decl.AsMap()
		if key.IsZero() {
			return // Legalized in the parser.
		}

		parent := field.Parent()
		base := parent.FullName()
		if base == "" {
			base = f.Package()
		}
		name := pcinternal.MapEntry(field.Name())
		fqn := base.Append(name)

		// Set option map_entry = true;
		builtins := c.builtins()
		messageOptions := builtins.MessageOptions.toRef(c)
		mapEntry := builtins.MapEntry.toRef(c)

		options := newMessage(c, builtins.MessageOptions.toRef(c))
		*options.insert(wrapMember(c, mapEntry)) =
			c.arenas.values.NewCompressed(rawValue{
				field: mapEntry,
				bits:  1,
			})

		// Construct the type itself.
		raw := c.arenas.types.NewCompressed(rawType{
			def:    field.AST(),
			name:   c.session.intern.Intern(name),
			fqn:    c.session.intern.Intern(string(fqn)),
			parent: c.arenas.types.Compress(parent.raw),
			options: c.arenas.values.NewCompressed(rawValue{
				field: messageOptions,
				bits:  rawValueBits(c.arenas.messages.Compress(options.raw)),
			}),

			mapEntryOf: c.arenas.members.Compress(field.raw),
		})
		ty := Type{internal.NewWith(c), c.arenas.types.Deref(raw)}
		ty.raw.memberByName = sync.OnceValue(ty.makeMembersByName)
		if parent.IsZero() {
			c.types = slices.Insert(c.types, c.topLevelTypesEnd, raw)
			c.topLevelTypesEnd++
		} else {
			c.types = append(c.types, raw)
			parent.raw.nested = append(parent.raw.nested, raw)
		}

		// Construct the fields and attach them to ty.
		makeField := func(name string, number int32) {
			fqn := fqn.Append(name)

			id := c.arenas.members.NewCompressed(rawMember{
				name:   c.session.intern.Intern(name),
				fqn:    c.session.intern.Intern(string(fqn)),
				parent: c.arenas.types.Compress(ty.raw),
				number: number,
				oneof:  -int32(presence.Explicit),
			})

			ty.raw.members = slices.Insert(ty.raw.members, int(ty.raw.extnsStart), id)
			ty.raw.extnsStart++
		}

		makeField("key", 1)
		makeField("value", 2)

		// Update the field to be a repeated field of the given type.
		field.raw.elem.ptr = raw
		field.raw.oneof = -int32(presence.Repeated)
	}

	for parent := range seq.Values(f.AllTypes()) {
		if !parent.IsMessage() {
			continue
		}

		for field := range seq.Values(parent.Members()) {
			lowerField(field)
		}
	}

	for extn := range seq.Values(f.AllExtensions()) {
		k, _ := extn.AST().Type().RemovePrefixes().AsGeneric().AsMap()
		if k.IsZero() {
			continue
		}

		r.Errorf("unsupported map-typed extension").Apply(
			report.Snippetf(extn.AST().Type(), "declared here"),
			report.Helpf("extensions cannot be map-typed; instead, "+
				"define a message type with a map-typed field"),
		)
		lowerField(extn)
	}
}
