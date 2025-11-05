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

package ir_test

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/ir"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/internal/ext/mapsx"
	"github.com/bufbuild/protocompile/internal/intern"
)

func TestImportResolution(t *testing.T) {
	t.Parallel()

	s := new(ir.Session)

	// Below, -> is a normal import, => is public, and ~> is weak.

	// a
	a := buildFile(s, "a.proto")
	assert.Empty(t, directs(a))
	assert.Empty(t, transitive(a))

	// a -> b
	b := buildFile(s, "b.proto", ir.Import{File: a})
	assert.Equal(t, []imp{{path: "a.proto"}}, directs(b))
	assert.Equal(t, []imp{{path: "a.proto"}}, transitive(b))

	// a -> b -
	// \        \
	//   -> c -> d
	c := buildFile(s, "c.proto", ir.Import{File: a})
	d := buildFile(s, "d.proto", ir.Import{File: b}, ir.Import{File: c})
	assert.Equal(t, []imp{{path: "b.proto"}, {path: "c.proto"}}, directs(d))
	assert.Equal(t, []imp{{path: "b.proto"}, {path: "c.proto"}, {path: "a.proto"}}, transitive(d))

	// a => e
	e := buildFile(s, "e.proto", ir.Import{File: a, Public: true})
	assert.Equal(t, []imp{{path: "a.proto", public: true}}, directs(e))
	assert.Equal(t, []imp{{path: "a.proto", public: true}}, transitive(e))

	// a => e -> f
	f := buildFile(s, "f.proto", ir.Import{File: e})
	assert.Equal(t, []imp{{path: "e.proto"}}, directs(f))
	assert.Equal(t, []imp{{path: "e.proto"}, {path: "a.proto"}}, transitive(f))

	// a => e => g
	g := buildFile(s, "g.proto", ir.Import{File: e, Public: true})
	assert.Equal(t, []imp{{path: "e.proto", public: true}}, directs(g))
	assert.Equal(t, []imp{{path: "e.proto", public: true}, {path: "a.proto", public: true}}, transitive(g))

	// a -> b -
	// \        \
	//   -> c -> d => h
	h := buildFile(s, "h.proto", ir.Import{File: d, Public: true})
	assert.Equal(t, []imp{{path: "d.proto", public: true}}, directs(h))
	assert.Equal(t, []imp{{path: "d.proto", public: true}, {path: "b.proto"}, {path: "c.proto"}, {path: "a.proto"}}, transitive(h))

	// This test case is particularly important, because it validates that
	// in a diamond configuration, we don't lose public-ness of a transitive
	// import if there are multiple public and non-public paths.
	//
	// a => i => j
	// \        /
	//   -> c -
	i := buildFile(s, "i.proto", ir.Import{File: a, Public: true})
	j := buildFile(s, "j.proto", ir.Import{File: c}, ir.Import{File: i, Public: true})
	assert.Equal(t, []imp{{path: "i.proto", public: true}, {path: "c.proto"}}, directs(j))
	assert.Equal(t, []imp{{path: "i.proto", public: true}, {path: "c.proto"}, {path: "a.proto", public: true}}, transitive(j))
	// Same as above but order is swapped.
	j2 := buildFile(s, "j.proto", ir.Import{File: i, Public: true}, ir.Import{File: c})
	assert.Equal(t, []imp{{path: "i.proto", public: true}, {path: "c.proto"}}, directs(j2))
	assert.Equal(t, []imp{{path: "i.proto", public: true}, {path: "c.proto"}, {path: "a.proto", public: true}}, transitive(j2))

	// a ~> k
	// NOTE: weak imports are not tracked transitively.
	k := buildFile(s, "k.proto", ir.Import{File: a, Weak: true})
	assert.Equal(t, []imp{{path: "a.proto", weak: true}}, directs(k))
	assert.Equal(t, []imp{{path: "a.proto"}}, transitive(k))

	// a ~> k ~> l
	l := buildFile(s, "l.proto", ir.Import{File: k, Weak: true})
	assert.Equal(t, []imp{{path: "k.proto", weak: true}}, directs(l))
	assert.Equal(t, []imp{{path: "k.proto"}, {path: "a.proto"}}, transitive(l))
}

// buildFile implements the most basic rudiments of building a File with a
// given import set, without involving an AST.
func buildFile(
	session *ir.Session,
	path string,
	imports ...ir.Import,
) *ir.File {
	file := ir.NewFile(session, path)
	table := ir.GetImports(file)

	dedup := make(intern.Map[ast.DeclImport])
	for _, imp := range imports {
		if !mapsx.AddZero(dedup, imp.File.InternedPath()) {
			continue
		}

		table.AddDirect(imp)
	}
	table.Recurse(dedup)
	table.Insert(ir.Import{}, -1, false) // Dummy descriptor.proto.

	return file
}

type imp struct {
	path         string
	public, weak bool
}

func directs(f *ir.File) []imp {
	return slices.Collect(seq.Map(ir.GetImports(f).Directs(), func(i ir.Import) imp {
		return imp{i.File.Path(), i.Public, i.Weak}
	}))
}

func transitive(f *ir.File) []imp {
	return slices.Collect(seq.Map(ir.GetImports(f).Transitive(), func(i ir.Import) imp {
		return imp{i.File.Path(), i.Public, i.Weak}
	}))
}
