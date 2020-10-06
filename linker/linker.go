package linker

import (
	"fmt"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/jhump/protocompile/parser"
	"github.com/jhump/protocompile/reporter"
)

func Link(parsed parser.Result, dependencies Files, symbols *Symbols, handler *reporter.Handler) (Result, error) {
	prefix := parsed.Proto().GetPackage()
	if prefix != "" {
		prefix += "."
	}

	for _, imp := range parsed.Proto().Dependency {
		dep := dependencies.FindFileByPath(imp)
		if dep == nil {
			return nil, fmt.Errorf("dependencies is missing import %q", imp)
		}
	}

	r := &result{
		Result:         parsed,
		deps:           dependencies,
		descriptors:    map[proto.Message]protoreflect.Descriptor{},
		descriptorPool: map[string]proto.Message{},
		usedImports:    map[string]struct{}{},
		prefix:         prefix,
	}

	// First, we put all symbols into a single pool, which lets us ensure there
	// are no duplicate symbols and will also let us resolve and revise all type
	// references in next step.
	if err := symbols.importResult(r, true, false, handler); err != nil {
		return nil, err
	}

	// After we've populated the pool, we can now try to resolve all type
	// references. All references must be checked for correct type, any fields
	// with enum types must be corrected (since we parse them as if they are
	// message references since we don't actually know message or enum until
	// link time), and references will be re-written to be fully-qualified
	// references (e.g. start with a dot ".").
	if err := r.resolveReferences(handler, symbols); err != nil {
		return nil, err
	}

	return r, handler.Error()
}

type Result interface {
	File
	parser.Result
	ResolveEnumType(protoreflect.FullName) protoreflect.EnumDescriptor
	ResolveMessageType(protoreflect.FullName) protoreflect.MessageDescriptor
	ResolveExtension(protoreflect.FullName) protoreflect.ExtensionTypeDescriptor
}
