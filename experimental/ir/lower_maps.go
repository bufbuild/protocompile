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

	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/internal"
	"github.com/bufbuild/protocompile/experimental/ir/presence"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/seq"
)

// generateMapEntries generates map entry types for all map-typed fields.
func generateMapEntries(f File, r *report.Report) {
	c := f.Context()
	for parent := range seq.Values(f.AllTypes()) {
		if !parent.IsMessage() {
			continue
		}

		for field := range seq.Values(parent.Members()) {
			// optional, repeated etc. on map types is already legalized in
			// the parser.
			decl := field.AST().Type().RemovePrefixes().AsGeneric()
			if decl.IsZero() {
				continue
			}

			key, value := decl.AsMap()
			if key.IsZero() {
				continue // Legalized in the parser.
			}

			name := toPascal(field.Name(), true) + "Entry"
			fqn := parent.FullName().Append(name)

			// Set option map_entry = true;
			dpIdx := int32(len(f.Context().imports.files))
			dp := f.Context().imports.DescriptorProto().Context()
			messageOptions := ref[rawMember]{dpIdx, dp.langSymbols.messageOptions}
			mapEntry := ref[rawMember]{dpIdx, dp.langSymbols.mapEntry}
			options := newMessage(c, messageOptions)
			*options.insert(wrapMember(c, mapEntry)) = c.arenas.values.NewCompressed(rawValue{
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
			parent.raw.nested = append(parent.raw.nested, raw)
			c.types = append(c.types, raw)

			// Construct the fields and att them to ty.
			makeField := func(name string, number int32, elem ast.TypeAny) {
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

			makeField("key", 1, key)
			makeField("value", 2, value)

			// Update the field to be a repeated field of the given type.
			field.raw.elem.ptr = raw
			field.raw.oneof = -int32(presence.Repeated)
		}
	}

	for extn := range seq.Values(f.AllExtensions()) {
		k, _ := extn.AST().Type().RemovePrefixes().AsGeneric().AsMap()
		if !k.IsZero() {
			r.Errorf("unsupported map-typed extension").Apply(
				report.Snippetf(extn.AST().Type(), "declared here"),
				report.Helpf("extensions cannot be map-typed; instead, "+
					"define a message type with a map-typed field"),
			)
			continue
		}
	}
}
