//go:build appengine || gopherjs || purego
// +build appengine gopherjs purego

// NB: other environments where unsafe is unappropriate should use "purego" build tag
// https://github.com/golang/go/issues/23172

package linker

import (
	"reflect"

	"google.golang.org/protobuf/reflect/protoreflect"
)

func pathKey(p protoreflect.SourcePath) interface{} {
	rv := reflect.ValueOf(p)
	arrayType := reflect.ArrayOf(rv.Len(), rv.Type().Elem())
	array := reflect.New(arrayType).Elem()
	reflect.Copy(array, rv)
	return array.Interface()
}
