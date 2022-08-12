package testutil

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"math"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/bufbuild/protocompile/linker"
	"github.com/bufbuild/protocompile/protoutil"
)

func LoadDescriptorSet(t *testing.T, path string, res linker.Resolver) *descriptorpb.FileDescriptorSet {
	data, err := ioutil.ReadFile(path)
	if !assert.Nil(t, err) {
		t.Fail()
	}
	var fdset descriptorpb.FileDescriptorSet
	err = proto.UnmarshalOptions{Resolver: res}.Unmarshal(data, &fdset)
	if !assert.Nil(t, err) {
		t.FailNow()
	}
	return &fdset
}

func CheckFiles(t *testing.T, act protoreflect.FileDescriptor, expSet FileProtoSet, recursive bool) {
	checkFiles(t, act, expSet, recursive, map[string]struct{}{})
}

func checkFiles(t *testing.T, act protoreflect.FileDescriptor, expSet FileProtoSet, recursive bool, checked map[string]struct{}) {
	if _, ok := checked[act.Path()]; ok {
		// already checked
		return
	}
	checked[act.Path()] = struct{}{}

	expProto := expSet.FindFile(act.Path())
	actProto := protoutil.ProtoFromFileDescriptor(act)
	checkFileDescriptor(t, actProto, expProto)

	if recursive {
		for i := 0; i < act.Imports().Len(); i++ {
			checkFiles(t, act.Imports().Get(i), expSet, true, checked)
		}
	}
}

type FileProtoSet interface {
	FindFile(name string) *descriptorpb.FileDescriptorProto
}

func FileProtoSetFromDescriptorProtos(fds *descriptorpb.FileDescriptorSet) FileProtoSet {
	return (*fdsProtoSet)(fds)
}

func FileProtoSetFromRegistry(reg *protoregistry.Files) FileProtoSet {
	return (*regProtoSet)(reg)
}

type fdsProtoSet descriptorpb.FileDescriptorSet

var _ FileProtoSet = &fdsProtoSet{}

func (fps *fdsProtoSet) FindFile(name string) *descriptorpb.FileDescriptorProto {
	files := fps.File
	for _, fd := range files {
		if fd.GetName() == name {
			return fd
		}
	}
	return nil
}

type regProtoSet protoregistry.Files

var _ FileProtoSet = &regProtoSet{}

func (fps *regProtoSet) FindFile(name string) *descriptorpb.FileDescriptorProto {
	f, err := (*protoregistry.Files)(fps).FindFileByPath(name)
	if err != nil {
		return nil
	}
	return protoutil.ProtoFromFileDescriptor(f)
}

func checkFileDescriptor(t *testing.T, act, exp *descriptorpb.FileDescriptorProto) {
	compareFiles(t, fmt.Sprintf("%q", act.GetName()), exp, act)
}

// adapted from implementation of proto.Equal, but records an error for each discrepancy
// found (does NOT exit early when a discrepancy is found)
func compareFiles(t *testing.T, path string, exp, act *descriptorpb.FileDescriptorProto) {
	if (exp == nil) != (act == nil) {
		if exp == nil {
			t.Errorf("%s: expected is nil; actual is not", path)
		} else {
			t.Errorf("%s: expected is not nil, but actual is", path)
		}
		return
	}
	mexp := exp.ProtoReflect()
	mact := act.ProtoReflect()
	if mexp.IsValid() != mact.IsValid() {
		if mexp.IsValid() {
			t.Errorf("%s: expected is valid; actual is not", path)
		} else {
			t.Errorf("%s: expected is not valid, but actual is", path)
		}
		return
	}
	compareMessages(t, path, mexp, mact)
}

func compareMessages(t *testing.T, path string, exp, act protoreflect.Message) {
	if exp.Descriptor() != act.Descriptor() {
		t.Errorf("%s: descriptors do not match: exp %#v, actual %#v", path, exp.Descriptor(), act.Descriptor())
		return
	}
	exp.Range(func(fd protoreflect.FieldDescriptor, expVal protoreflect.Value) bool {
		name := fieldDisplayName(fd)
		actVal := act.Get(fd)
		if !act.Has(fd) {
			t.Errorf("%s: expected has field %s but actual does not", path, name)
		} else {
			compareFields(t, path+"."+name, fd, expVal, actVal)
		}
		return true
	})
	act.Range(func(fd protoreflect.FieldDescriptor, actVal protoreflect.Value) bool {
		name := fieldDisplayName(fd)
		if !exp.Has(fd) {
			t.Errorf("%s: actual has field %s but expected does not", path, name)
		}
		return true
	})

	compareUnknown(t, path, exp.GetUnknown(), act.GetUnknown())
}

func fieldDisplayName(fd protoreflect.FieldDescriptor) string {
	if fd.IsExtension() {
		return "(" + string(fd.FullName()) + ")"
	}
	return string(fd.Name())
}

func compareFields(t *testing.T, path string, fd protoreflect.FieldDescriptor, exp, act protoreflect.Value) {
	switch {
	case fd.IsList():
		compareLists(t, path, fd, exp.List(), act.List())
	case fd.IsMap():
		compareMaps(t, path, fd, exp.Map(), act.Map())
	default:
		compareValues(t, path, fd, exp, act)
	}
}

func compareMaps(t *testing.T, path string, fd protoreflect.FieldDescriptor, exp, act protoreflect.Map) {
	exp.Range(func(k protoreflect.MapKey, expVal protoreflect.Value) bool {
		actVal := act.Get(k)
		if !act.Has(k) {
			t.Errorf("%s: expected map has key %s but actual does not", path, k.String())
		} else {
			compareValues(t, path+"["+k.String()+"]", fd.MapValue(), expVal, actVal)
		}
		return true
	})
	act.Range(func(k protoreflect.MapKey, actVal protoreflect.Value) bool {
		if !exp.Has(k) {
			t.Errorf("%s: actual map has key %s but expected does not", path, k.String())
		}
		return true
	})
}

func compareLists(t *testing.T, path string, fd protoreflect.FieldDescriptor, exp, act protoreflect.List) {
	if exp.Len() != act.Len() {
		t.Errorf("%s: expected is list with %d items but actual has %d", path, exp.Len(), act.Len())
	}
	lim := exp.Len()
	if act.Len() < lim {
		lim = act.Len()
	}
	for i := 0; i < lim; i++ {
		compareValues(t, path+"["+strconv.Itoa(i)+"]", fd, exp.Get(i), act.Get(i))
	}
}

func compareValues(t *testing.T, path string, fd protoreflect.FieldDescriptor, exp, act protoreflect.Value) {
	if fd.Kind() == protoreflect.MessageKind || fd.Kind() == protoreflect.GroupKind {
		compareMessages(t, path, exp.Message(), act.Message())
		return
	}

	var eq bool
	switch fd.Kind() {
	case protoreflect.BoolKind:
		eq = exp.Bool() == act.Bool()
	case protoreflect.EnumKind:
		eq = exp.Enum() == act.Enum()
	case protoreflect.Int32Kind, protoreflect.Sint32Kind,
		protoreflect.Int64Kind, protoreflect.Sint64Kind,
		protoreflect.Sfixed32Kind, protoreflect.Sfixed64Kind:
		eq = exp.Int() == act.Int()
	case protoreflect.Uint32Kind, protoreflect.Uint64Kind,
		protoreflect.Fixed32Kind, protoreflect.Fixed64Kind:
		eq = exp.Uint() == act.Uint()
	case protoreflect.FloatKind, protoreflect.DoubleKind:
		fx := exp.Float()
		fy := act.Float()
		if math.IsNaN(fx) || math.IsNaN(fy) {
			eq = math.IsNaN(fx) && math.IsNaN(fy)
		} else {
			eq = fx == fy
		}
	case protoreflect.StringKind:
		eq = exp.String() == act.String()
	case protoreflect.BytesKind:
		eq = bytes.Equal(exp.Bytes(), act.Bytes())
	default:
		eq = exp.Interface() == act.Interface()
	}
	if !eq {
		t.Errorf("%s: expected is %v but actual is %v", path, exp, act)
	}
}

func compareUnknown(t *testing.T, path string, exp, act protoreflect.RawFields) {
	if bytes.Equal(exp, act) {
		return
	}

	mexp := make(map[protoreflect.FieldNumber]protoreflect.RawFields)
	mact := make(map[protoreflect.FieldNumber]protoreflect.RawFields)
	for len(exp) > 0 {
		fnum, _, n := protowire.ConsumeField(exp)
		mexp[fnum] = append(mexp[fnum], exp[:n]...)
		exp = exp[n:]
	}
	for len(act) > 0 {
		fnum, _, n := protowire.ConsumeField(act)
		bact := act[:n]
		mact[fnum] = append(mact[fnum], bact...)
		if bexp, ok := mexp[fnum]; !ok {
			t.Errorf("%s: expected has data for unknown field with tag %d but actual does not", path, fnum)
		} else if !bytes.Equal(bexp, bact) {
			t.Errorf("%s: expected has %v for unknown field with tag %d but actual has %v", path, bexp, fnum, bact)
		}
		act = act[n:]
	}
	for fnum := range mexp {
		_, ok := mact[fnum]
		if !ok {
			t.Errorf("%s: actual has data for unknown field with tag %d but expected does not", path, fnum)
		}
	}
}
