// Copyright 2020-2022 Buf Technologies, Inc.
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

package prototest

import (
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/bufbuild/protocompile/linker"
	"github.com/bufbuild/protocompile/protoutil"
)

func LoadDescriptorSet(t *testing.T, path string, res linker.Resolver) *descriptorpb.FileDescriptorSet {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var fdset descriptorpb.FileDescriptorSet
	err = proto.UnmarshalOptions{Resolver: res}.Unmarshal(data, &fdset)
	require.NoError(t, err)
	return &fdset
}

func CheckFiles(t *testing.T, act protoreflect.FileDescriptor, expSet *descriptorpb.FileDescriptorSet, recursive bool) {
	t.Helper()
	checkFiles(t, act, expSet, recursive, map[string]struct{}{})
}

func checkFiles(t *testing.T, act protoreflect.FileDescriptor, expSet *descriptorpb.FileDescriptorSet, recursive bool, checked map[string]struct{}) {
	if _, ok := checked[act.Path()]; ok {
		// already checked
		return
	}
	checked[act.Path()] = struct{}{}

	expProto := findFileInSet(expSet, act.Path())
	actProto := protoutil.ProtoFromFileDescriptor(act)
	if diff := cmp.Diff(expProto, actProto, protocmp.Transform()); diff != "" {
		t.Errorf("file descriptor mismatch (-want +got):\n%v", diff)
	}

	if recursive {
		for i := 0; i < act.Imports().Len(); i++ {
			checkFiles(t, act.Imports().Get(i), expSet, true, checked)
		}
	}
}

func findFileInSet(fps *descriptorpb.FileDescriptorSet, name string) *descriptorpb.FileDescriptorProto {
	files := fps.File
	for _, fd := range files {
		if fd.GetName() == name {
			return fd
		}
	}
	return nil
}

func CompareMessages(t *testing.T, path string, exp, act protoreflect.Message, opts ...cmp.Option) {
	t.Helper()
	if exp.Descriptor() != act.Descriptor() {
		t.Errorf("%s: descriptors do not match: exp %#v, actual %#v", path, exp.Descriptor(), act.Descriptor())
		return
	}
	cmpOpts := []cmp.Option{protocmp.Transform()}
	cmpOpts = append(cmpOpts, opts...)
	if diff := cmp.Diff(exp.Interface(), act.Interface(), cmpOpts...); diff != "" {
		t.Errorf("message mismatch (-want +got):\n%v", diff)
	}
}
