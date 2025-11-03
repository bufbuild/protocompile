package ir

import (
	"fmt"

	"github.com/bufbuild/protocompile/experimental/id"
)

// Ref is a reference in a Protobuf file: an [id.ID] along with information for
// retrieving which file that ID is for, relative to the referencing file's
// context.
//
// The context needed for resolving a ref is called its "base context", which
// the user is expected to keep track of.
type Ref[T any] struct {
	// The file this ref is defined in. If zero, it refers to the current file.
	// If -1, it refers to a predeclared type. Otherwise, it refers to an
	// import (with its index offset by 1).
	file int32
	id   id.ID[T]
}

// IsZero returns whether this is the zero ID.
func (r Ref[T]) IsZero() bool {
	return r.id == 0
}

// Get vets the value that a reference refers to.
func GetRef[T ~id.Value[T, *Context, Raw], Raw any](base *Context, r Ref[T]) T {
	return id.NewValue(r.Context(base), r.id)
}

// Context returns the context for this reference relative to a base context.
func (r Ref[T]) Context(base *Context) *Context {
	switch r.file {
	case 0:
		return base
	case -1:
		return primitiveCtx
	default:
		return base.imports.files[r.file-1].file.Context()
	}
}

// ChangeContext changes the implicit context for this ref to be with respect to
// the new one given.
func (r Ref[T]) ChangeContext(base, next *Context) Ref[T] {
	if base == next {
		return r
	}

	ctx := r.Context(base)
	if ctx == primitiveCtx {
		r.file = -1
		return r
	}

	// Figure out where file sits in next.
	idx, ok := next.imports.byPath[ctx.File().InternedPath()]
	if !ok {
		panic(fmt.Sprintf("protocompile/ir: could not change contexts %q -> %q", base.File().Path(), next.File().Path()))
	}

	r.file = int32(idx) + 1
	return r
}
