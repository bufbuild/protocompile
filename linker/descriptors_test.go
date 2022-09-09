package linker_test

import (
	"context"
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/bufbuild/protocompile"
	"github.com/bufbuild/protocompile/internal/prototest"
)

func TestFieldDefaults(t *testing.T) {
	fds := prototest.LoadDescriptorSet(t, "../internal/testdata/desc_test_defaults.protoset", nil)
	files, err := protodesc.NewFiles(fds)
	require.NoError(t, err)
	protocFd, err := files.FindFileByPath("desc_test_defaults.proto")
	require.NoError(t, err)

	compiler := protocompile.Compiler{
		Resolver: &protocompile.SourceResolver{
			ImportPaths: []string{"../internal/testdata"},
		},
	}
	results, err := compiler.Compile(context.Background(), "desc_test_defaults.proto")
	require.NoError(t, err)
	fd := results[0]

	checkDefaults(t, protocFd, fd, `"desc_test_defaults.proto"`)
}

type container interface {
	Extensions() protoreflect.ExtensionDescriptors
	Messages() protoreflect.MessageDescriptors
}

func checkDefaults(t *testing.T, exp, actual container, path string) {
	checkDefaultsInFields(t, exp.Extensions(), actual.Extensions(), fmt.Sprintf("extensions in %s", path))
	if assert.Equal(t, exp.Messages().Len(), actual.Messages().Len()) {
		for i := 0; i < exp.Messages().Len(); i++ {
			expMsg := exp.Messages().Get(i)
			actMsg := actual.Messages().Get(i)
			if !assert.Equal(t, expMsg.Name(), actMsg.Name(), "%s: message name at index %d", path, i) {
				continue
			}
			checkDefaults(t, expMsg, actMsg, fmt.Sprintf("%s.%s", path, expMsg.Name()))
		}
	}

	if expMsg, ok := exp.(protoreflect.MessageDescriptor); ok {
		actMsg := actual.(protoreflect.MessageDescriptor)
		checkDefaultsInFields(t, expMsg.Fields(), actMsg.Fields(), fmt.Sprintf("fields in %s", path))
	}
}

func checkDefaultsInFields(t *testing.T, exp, actual protoreflect.ExtensionDescriptors, where string) {
	if !assert.Equal(t, exp.Len(), actual.Len(), "%s: number of fields", where) {
		return
	}
	for i := 0; i < exp.Len(); i++ {
		expFld := exp.Get(i)
		actFld := actual.Get(i)
		if !assert.Equal(t, expFld.Name(), actFld.Name(), "%s: field name at index %d", where, i) {
			continue
		}
		assert.Equal(t, expFld.HasDefault(), actFld.HasDefault(), "%s: field has default at index %d (%s)", where, i, expFld.Name())

		expVal := expFld.Default().Interface()
		actVal := actFld.Default().Interface()
		if fl, ok := expVal.(float32); ok && math.IsNaN(float64(fl)) {
			actFl, actOk := actVal.(float32)
			assert.True(t, actOk && math.IsNaN(float64(actFl)), "%s: field default value should be float32 NaN at index %d (%s): %v", where, i, expFld.Name(), actVal)
		} else if fl, ok := expVal.(float64); ok && math.IsNaN(fl) {
			actFl, actOk := actVal.(float64)
			assert.True(t, actOk && math.IsNaN(actFl), "%s: field default value should be float64 NaN at index %d (%s): %v", where, i, expFld.Name(), actVal)
		} else {
			assert.Equal(t, expFld.Default().Interface(), actFld.Default().Interface(), "%s: field default value at index %d (%s)", where, i, expFld.Name())
		}

		expEnumVal := expFld.DefaultEnumValue()
		actEnumVal := actFld.DefaultEnumValue()
		if expEnumVal == nil {
			assert.Nil(t, actEnumVal, "%s: field default enum value should be nil at index %d (%s)", where, i, expFld.Name())
		} else if assert.NotNil(t, actEnumVal, "%s: field default enum value should not be nil at index %d (%s)", where, i, expFld.Name()) {
			assert.Equal(t, expEnumVal.Name(), actEnumVal.Name(), "%s: field default enum value at index %d (%s)", where, i, expFld.Name())
			assert.Equal(t, expEnumVal.Number(), actEnumVal.Number(), "%s: field default enum value at index %d (%s)", where, i, expFld.Name())
		}
	}
}
