// Copyright 2020-2024 Buf Technologies, Inc.
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

package linker

import "google.golang.org/protobuf/types/descriptorpb"

// allocPool helps allocate descriptor instances. Instead of allocating
// them one at a time, we allocate a pool -- a large, flat slice to hold
// all descriptors of a particular kind for a file. We then use capacity
// in the pool when we need space for individual descriptors.
type allocPool struct {
	numMessages   int
	numFields     int
	numOneofs     int
	numEnums      int
	numEnumValues int
	numExtensions int
	numServices   int
	numMethods    int

	messages   []msgDescriptor
	fields     []fldDescriptor
	oneofs     []oneofDescriptor
	enums      []enumDescriptor
	enumVals   []enValDescriptor
	extensions []extTypeDescriptor
	services   []svcDescriptor
	methods    []mtdDescriptor
}

func (p *allocPool) getMessages(count int) []msgDescriptor {
	if p.messages == nil {
		p.messages = make([]msgDescriptor, p.numMessages)
	}
	allocated := p.messages[:count]
	p.messages = p.messages[count:]
	return allocated
}

func (p *allocPool) getFields(count int) []fldDescriptor {
	if p.fields == nil {
		p.fields = make([]fldDescriptor, p.numFields)
	}
	allocated := p.fields[:count]
	p.fields = p.fields[count:]
	return allocated
}

func (p *allocPool) getOneofs(count int) []oneofDescriptor {
	if p.oneofs == nil {
		p.oneofs = make([]oneofDescriptor, p.numOneofs)
	}
	allocated := p.oneofs[:count]
	p.oneofs = p.oneofs[count:]
	return allocated
}

func (p *allocPool) getEnums(count int) []enumDescriptor {
	if p.enums == nil {
		p.enums = make([]enumDescriptor, p.numEnums)
	}
	allocated := p.enums[:count]
	p.enums = p.enums[count:]
	return allocated
}

func (p *allocPool) getEnumValues(count int) []enValDescriptor {
	if p.enumVals == nil {
		p.enumVals = make([]enValDescriptor, p.numEnumValues)
	}
	allocated := p.enumVals[:count]
	p.enumVals = p.enumVals[count:]
	return allocated
}

func (p *allocPool) getExtensions(count int) []extTypeDescriptor {
	if p.extensions == nil {
		p.extensions = make([]extTypeDescriptor, p.numExtensions)
	}
	allocated := p.extensions[:count]
	p.extensions = p.extensions[count:]
	return allocated
}

func (p *allocPool) getServices(count int) []svcDescriptor {
	if p.services == nil {
		p.services = make([]svcDescriptor, p.numServices)
	}
	allocated := p.services[:count]
	p.services = p.services[count:]
	return allocated
}

func (p *allocPool) getMethods(count int) []mtdDescriptor {
	if p.methods == nil {
		p.methods = make([]mtdDescriptor, p.numMethods)
	}
	allocated := p.methods[:count]
	p.methods = p.methods[count:]
	return allocated
}

func (p *allocPool) countElements(file *descriptorpb.FileDescriptorProto) {
	p.countElementsInMessages(file.MessageType)
	p.countElementsInEnums(file.EnumType)
	p.numExtensions += len(file.Extension)
	p.numServices += len(file.Service)
	for _, svc := range file.Service {
		p.numMethods += len(svc.Method)
	}
}

func (p *allocPool) countElementsInMessages(msgs []*descriptorpb.DescriptorProto) {
	p.numMessages += len(msgs)
	for _, msg := range msgs {
		p.numFields += len(msg.Field)
		p.numOneofs += len(msg.OneofDecl)
		p.countElementsInMessages(msg.NestedType)
		p.countElementsInEnums(msg.EnumType)
		p.numExtensions += len(msg.Extension)
	}
}

func (p *allocPool) countElementsInEnums(enums []*descriptorpb.EnumDescriptorProto) {
	p.numEnums += len(enums)
	for _, enum := range enums {
		p.numEnumValues += len(enum.Value)
	}
}
