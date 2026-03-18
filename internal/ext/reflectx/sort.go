package reflectx

import (
	"cmp"
	"reflect"
	"sort"
)

// Returns whether t is a [cmp.Ordered] type.
func Ordered(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Int,
		reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uint, reflect.Uintptr,
		reflect.Float32, reflect.Float64,
		reflect.String:
		return true
	default:
		return false
	}
}

// Compare compares order-comparable [reflect.Value]s of the same type.
//
// Returns false when a and b are of different types, or do not conform to
// [cmp.Ordered].
func Compare(a, b reflect.Value) (diff int, ok bool) {
	t := a.Type()
	if t != b.Type() {
		return 0, false
	}

	switch t.Kind() {
	case reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Int:
		diff = cmp.Compare(a.Int(), b.Int())
	case reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uint, reflect.Uintptr:
		diff = cmp.Compare(a.Uint(), b.Uint())
	case reflect.Float32, reflect.Float64:
		diff = cmp.Compare(a.Float(), b.Float())
	case reflect.String:
		diff = cmp.Compare(a.String(), b.String())
	default:
		return 0, false
	}

	return diff, true
}

// Sort sorts the elements of a slice (or pointer to array) of dynamic type
// using reflection.
//
// Returns whether v was sortable.
func Sort(v reflect.Value) (ok bool) {
	if v.Kind() == reflect.Array {
		return false
	}

	for v.Kind() == reflect.Interface || v.Kind() == reflect.Pointer {
		if v.IsZero() {
			return
		}
		v = v.Elem()
	}

	if !(v.Kind() == reflect.Slice || (v.Kind() == reflect.Array && !v.CanAddr())) {
		return false
	}
	if !Ordered(v.Type().Elem()) {
		return false
	}

	sort.Sort(sorter{
		v: v,
		// Swapper expects a slice. Calling Slice() here will convert a *[N]T
		// into a []T for us.
		swap: reflect.Swapper(v.Slice(0, v.Len()).Interface()),
	})
	return true
}

type sorter struct {
	v    reflect.Value
	swap func(i, j int)
}

var _ sort.Interface = sorter{}

// Len implements [sort.Interface].
func (s sorter) Len() int {
	return s.v.Len()
}

// Less implements [sort.Interface].
func (s sorter) Less(i int, j int) bool {
	n, _ := Compare(s.v.Index(i), s.v.Index(j))
	return n < 0
}

// Swap implements [sort.Interface].
func (s sorter) Swap(i int, j int) {
	s.swap(i, j)
}
