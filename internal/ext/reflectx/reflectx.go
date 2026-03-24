package reflectx

import "reflect"

// UnwrapStruct removes as many layers of "one field wrappers" on v as possible.
//
// This means:
// 1. The one field in a struct with non-zero size.
// 2. The one element of a [1]T.
func UnwrapStruct(v reflect.Value) reflect.Value {
loop:
	switch v.Kind() {
	case reflect.Struct:
		ty := v.Type()

		var nonzero reflect.StructField
		for i := range ty.NumField() {
			f := ty.Field(i)
			if f.Offset > 0 {
				// This catches the following problematic struct:
				//
				// struct { A int; B [0]int }
				//
				// Zero-sized fields after the last non-zero-sized field
				// result in padding.
				break loop
			}
			if f.Type.Size() > 0 {
				if nonzero.Type != nil {
					break loop
				}
				nonzero = f
			}
		}

		v = v.FieldByIndex(nonzero.Index)
		goto loop

	case reflect.Array:
		if v.Len() == 1 {
			v = v.Index(0)
			goto loop
		}
	}

	return v
}
