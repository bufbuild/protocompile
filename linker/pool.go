package linker

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
