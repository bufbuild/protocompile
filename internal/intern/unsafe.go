package intern

import (
	"reflect"
	"unsafe"

	"github.com/bufbuild/protocompile/internal/ext/unsafex"
)

// anyArena emulates the runtime.convT functions, except that it places the
// allocated value into the given arena.
//
// It only works for types T which are not indirect interface values (e.g.,
// pointer-shaped types).
type anyArena[T any] []T

func (c *anyArena[T]) alloc(value T) any {
	a := *c
	if len(a) == cap(a) {
		// append(nil, make) ensures that the capacity is as large as possible,
		// i.e., matching a size class.
		a = append([]T(nil), make([]T, cap(a)*2)...)[:0]
	}

	a = append(a, value)
	*c = a
	p := &a[len(a)-1]

	type iface struct {
		itab unsafe.Pointer
		data unsafe.Pointer
	}

	var z T
	itab := unsafex.Bitcast[iface](reflect.TypeOf(z)).data
	return unsafex.Bitcast[any](iface{
		itab: itab,
		data: unsafe.Pointer(p),
	})
}
