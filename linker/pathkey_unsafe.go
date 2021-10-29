//go:build !appengine && !gopherjs && !purego
// +build !appengine,!gopherjs,!purego

// NB: other environments where unsafe is inappropriate should use "purego" build tag
// https://github.com/golang/go/issues/23172

package linker

import (
	"reflect"
	"unsafe"

	"google.golang.org/protobuf/reflect/protoreflect"
)

var pathElementType = reflect.TypeOf(protoreflect.SourcePath{}).Elem()

func pathKey(p protoreflect.SourcePath) interface{} {
	hdr := (*reflect.SliceHeader)(unsafe.Pointer(reflect.ValueOf(&p).Pointer()))
	array := reflect.NewAt(reflect.ArrayOf(hdr.Len, pathElementType), unsafe.Pointer(hdr.Data))
	return array.Elem().Interface()
}
