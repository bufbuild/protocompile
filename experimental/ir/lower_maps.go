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

	"github.com/bufbuild/protocompile/experimental/id"
	"github.com/bufbuild/protocompile/experimental/ir/presence"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/seq"
	pcinternal "github.com/bufbuild/protocompile/internal"
)

// generateMapEntries generates map entry types for all map-typed fields.
func generateMapEntries(file *File, r *report.Report) {
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
			base = file.Package()
		}
		name := pcinternal.MapEntry(field.Name())
		fqn := base.Append(name)

		// Set option map_entry = true;
		builtins := file.builtins()
		messageOptions := builtins.MessageOptions.toRef(file)
		mapEntry := builtins.MapEntry.toRef(file)

		options := newMessage(file, builtins.MessageOptions.toRef(file))
		options.slot(GetRef(file, mapEntry)).Insert(id.Wrap(
			file,
			id.ID[Value](file.arenas.values.NewCompressed(rawValue{
				field: mapEntry,
				bits:  1,
			})),
		))

		// Construct the type itself.
		ty := id.Wrap(file, id.ID[Type](file.arenas.types.NewCompressed(rawType{
			def:    field.AST().ID(),
			name:   file.session.intern.Intern(name),
			fqn:    file.session.intern.Intern(string(fqn)),
			parent: parent.ID(),
			options: id.ID[Value](file.arenas.values.NewCompressed(rawValue{
				field: messageOptions,
				bits:  rawValueBits(file.arenas.messages.Compress(options.Raw())),
			})),

			mapEntryOf: field.ID(),
		})))
		ty.Raw().memberByName = sync.OnceValue(ty.makeMembersByName)
		if parent.IsZero() {
			file.types = slices.Insert(file.types, file.topLevelTypesEnd, ty.ID())
			file.topLevelTypesEnd++
		} else {
			file.types = append(file.types, ty.ID())
			parent.Raw().nested = append(parent.Raw().nested, ty.ID())
		}

		// Construct the fields and attach them to ty.
		makeField := func(name string, number int32) {
			fqn := fqn.Append(name)

			p := id.ID[Member](file.arenas.members.NewCompressed(rawMember{
				name:   file.session.intern.Intern(name),
				fqn:    file.session.intern.Intern(string(fqn)),
				parent: ty.ID(),
				number: number,
				oneof:  -int32(presence.Explicit),
			}))

			ty.Raw().members = slices.Insert(ty.Raw().members, int(ty.Raw().extnsStart), p)
			ty.Raw().extnsStart++
		}

		makeField("key", 1)
		makeField("value", 2)

		// Update the field to be a repeated field of the given type.
		field.Raw().elem = ty.toRef(file)
		field.Raw().oneof = -int32(presence.Repeated)
	}

	for parent := range seq.Values(file.AllTypes()) {
		if !parent.IsMessage() {
			continue
		}

		for field := range seq.Values(parent.Members()) {
			lowerField(field)
		}
	}

	for extn := range seq.Values(file.AllExtensions()) {
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
