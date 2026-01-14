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

package expr

import (
	"fmt"

	"github.com/bufbuild/protocompile/experimental/id"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/internal/arena"
	"github.com/bufbuild/protocompile/internal/ext/unsafex"
)

// Context is book-keeping for all of the expressions parsed from a particular
// [token.Stream].
//
// This type does not represent an AST node; instead, it is intended to be
// embedded into the top-level struct for one of the other AST packages in
// the compiler.
type Context struct {
	_      unsafex.NoCopy
	stream *token.Stream

	arenas
}

// Nodes provides storage for the various AST node types, and can be used
// to construct new ones.
type Nodes Context

// New creates a fresh expression context for a stream.
func New(stream *token.Stream) *Context {
	return &Context{stream: stream}
}

// Stream returns the underlying token stream.
func (c *Context) Stream() *token.Stream {
	if c == nil {
		return nil
	}
	return c.stream
}

// Nodes returns the node arena for this file, which can be used to allocate
// new AST nodes.
func (c *Context) Nodes() *Nodes {
	return (*Nodes)(c)
}

// File returns the [File] that this Nodes adds nodes to.
func (n *Nodes) Context() *Context {
	return (*Context)(n)
}

// NewError constructs a new [Error] at the given span in this context.
func (n *Nodes) NewError(span source.Span) Error {
	return id.Wrap(n.Context(), id.ID[Error](n.Context().errors.NewCompressed(rawError{span})))
}

// NewBlock constructs a new [Block] in this context.
func (n *Nodes) NewBlock(args BlockArgs) Block {
	n.panicIfNotOurs(args.Braces)

	return id.Wrap(n.Context(), id.ID[Block](n.Context().blocks.NewCompressed(rawBlock{
		braces: args.Braces.ID(),
	})))
}

// NewCall constructs a new [Call] in this context.
func (n *Nodes) NewCall(args CallArgs) Call {
	n.panicIfNotOurs(args.Callee, args.Args)

	return id.Wrap(n.Context(), id.ID[Call](n.Context().calls.NewCompressed(rawCall{
		callee: args.Callee.ID(),
		args:   args.Args.ID(),
	})))
}

// NewCase constructs a new [Case] in this context.
func (n *Nodes) NewCase(args CaseArgs) Case {
	n.panicIfNotOurs(args.Keyword, args.Alts, args.Colon, args.Block)

	return id.Wrap(n.Context(), id.ID[Case](n.Context().cases.NewCompressed(rawCase{
		kw:    args.Keyword.ID(),
		alts:  args.Alts.ID(),
		colon: args.Colon.ID(),
		block: args.Block.ID(),
	})))
}

// NewCase constructs a new [Case] in this context.
func (n *Nodes) NewControl(args ControlArgs) Control {
	n.panicIfNotOurs(args.Keyword, args.Args, args.If, args.Condition)

	return id.Wrap(n.Context(), id.ID[Control](n.Context().controls.NewCompressed(rawControl{
		kw:   args.Keyword.ID(),
		args: args.Args.ID(),
		ifT:  args.If.ID(),
		cond: args.Condition.ID(),
	})))
}

// NewFor constructs a new [Case] in this context.
func (n *Nodes) NewFor(args ForArgs) For {
	n.panicIfNotOurs(args.For, args.Vars, args.In, args.Iterator, args.Block)

	return id.Wrap(n.Context(), id.ID[For](n.Context().fors.NewCompressed(rawFor{
		forT:  args.For.ID(),
		vars:  args.Vars.ID(),
		inT:   args.For.ID(),
		iter:  args.Iterator.ID(),
		block: args.Block.ID(),
	})))
}

// NewIf constructs a new [If] in this context.
func (n *Nodes) NewIf(args IfArgs) If {
	n.panicIfNotOurs(args.Else, args.If, args.Cond, args.Block)

	return id.Wrap(n.Context(), id.ID[If](n.Context().ifs.NewCompressed(rawIf{
		elseT: args.Else.ID(),
		ifT:   args.If.ID(),
		cond:  args.Cond.ID(),
		block: args.Block.ID(),
	})))
}

// NewOp constructs a new [Op] in this context.
func (n *Nodes) NewOp(args OpArgs) Op {
	n.panicIfNotOurs(args.Left, args.Right, args.Op)

	return id.Wrap(n.Context(), id.ID[Op](n.Context().ops.NewCompressed(rawOp{
		left:  args.Left.ID(),
		right: args.Right.ID(),
		op:    args.Op.ID(),
	})))
}

// NewRecord constructs a new [Record] in this context.
func (n *Nodes) NewRecord(args RecordArgs) Record {
	n.panicIfNotOurs(args.Entries)

	return id.WrapRaw(n.Context(), id.ID[Record](args.Entries.ID()), args.Entries.Raw())
}

// NewSwitch constructs a new [Switch] in this context.
func (n *Nodes) NewSwitch(args SwitchArgs) Switch {
	n.panicIfNotOurs(args.Switch, args.Arg, args.Braces)

	return id.Wrap(n.Context(), id.ID[Switch](n.Context().switches.NewCompressed(rawSwitch{
		kw:     args.Switch.ID(),
		arg:    args.Arg.ID(),
		braces: args.Braces.ID(),
	})))
}

type arenas struct {
	blocks   arena.Arena[rawBlock]
	calls    arena.Arena[rawCall]
	cases    arena.Arena[rawCase]
	controls arena.Arena[rawControl]
	errors   arena.Arena[rawError]
	fors     arena.Arena[rawFor]
	funcs    arena.Arena[rawFunc]
	ifs      arena.Arena[rawIf]
	ops      arena.Arena[rawOp]
	params   arena.Arena[rawParams]
	switches arena.Arena[rawSwitch]
}

// FromID implements [id.Context].
func (c *Context) FromID(id uint64, want any) any {
	switch want.(type) {
	case **rawBlock:
		return c.blocks.Deref(arena.Pointer[rawBlock](id))
	case **rawCall:
		return c.calls.Deref(arena.Pointer[rawCall](id))
	case **rawCase:
		return c.cases.Deref(arena.Pointer[rawCase](id))
	case **rawControl:
		return c.controls.Deref(arena.Pointer[rawControl](id))
	case **rawError:
		return c.errors.Deref(arena.Pointer[rawError](id))
	case **rawFor:
		return c.fors.Deref(arena.Pointer[rawFor](id))
	case **rawFunc:
		return c.funcs.Deref(arena.Pointer[rawFunc](id))
	case **rawIf:
		return c.ifs.Deref(arena.Pointer[rawIf](id))
	case **rawOp:
		return c.ops.Deref(arena.Pointer[rawOp](id))
	case **rawParams:
		return c.params.Deref(arena.Pointer[rawParams](id))
	case **rawSwitch:
		return c.switches.Deref(arena.Pointer[rawSwitch](id))

	default:
		return c.stream.FromID(id, want)
	}
}

// panicIfNotOurs checks that a contextual value is owned by this context, and panics if not.
//
// Does not panic if that is zero or has a zero context. Panics if n is zero.
func (n *Nodes) panicIfNotOurs(that ...any) {
	for _, that := range that {
		if that == nil {
			continue
		}

		var path string
		switch that := that.(type) {
		case interface{ Context() *token.Stream }:
			ctx := that.Context()
			if ctx == nil || ctx == n.Context().Stream() {
				continue
			}
			path = ctx.Path()

		case interface{ Context() *Context }:
			ctx := that.Context()
			if ctx == nil || ctx == n.Context() {
				continue
			}
			path = ctx.Stream().Path()

		default:
			continue
		}

		panic(fmt.Sprintf(
			"protocompile/expr: attempt to mix different contexts: %q vs %q",
			n.stream.Path(),
			path,
		))
	}
}
