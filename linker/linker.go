package linker

import (
	"fmt"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/jhump/protocompile/parser"
	"github.com/jhump/protocompile/reporter"
)

// Link handles linking a parsed descriptor proto into a fully-linked descriptor.
// If the given parser.Result has imports, they must all be present in the given
// dependencies. The symbols value is optional and may be nil. The handler value
// is used to report any link errors. If any such errors are reported, this
// function returns a non-nil error. The Result value returned also implements
// protoreflect.FileDescriptor.
//
// Note that linking does NOT interpret options. So options messages in the
// returned value have all values stored in UninterpretedOptions fields.
func Link(parsed parser.Result, dependencies Files, symbols *Symbols, handler *reporter.Handler) (Result, error) {
	if symbols == nil {
		symbols = &Symbols{}
	}
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

// Result is the result of linking. This is a protoreflect.FileDescriptor, but
// with some additional methods for exposing additional information, such as the
// for accessing the input AST or file descriptor.
//
// It also provides Resolve* methods, for looking up enums, messages, and
// extensions that are available to the protobuf source file this result
// represents. An element is "available" if it meets any of the following
// criteria:
//  1. The element is defined in this file itself.
//  2. The element is defined in a file that is directly imported by this file.
//  3. The element is "available" to a file that is directly imported by this
//     file as a public import.
// Other elements, even if the transitive closure of this file, are not
// available and thus won't be returned by these methods.
type Result interface {
	File
	parser.Result
	// ResolveEnumType returns an enum descriptor for the given named enum that
	// is available in this file. If no such element is available or if the
	// named element is not an enum, nil is returned.
	ResolveEnumType(protoreflect.FullName) protoreflect.EnumDescriptor
	// ResolveMessageType returns a message descriptor for the given named
	// message that is available in this file. If no such element is available
	// or if the named element is not a message, nil is returned.
	ResolveMessageType(protoreflect.FullName) protoreflect.MessageDescriptor
	// ResolveExtension returns an extension descriptor for the given named
	// extension that is available in this file. If no such element is available
	// or if the named element is not an extension, nil is returned.
	ResolveExtension(protoreflect.FullName) protoreflect.ExtensionTypeDescriptor
	// ValidateExtensions runs some validation checks on extensions that can only
	// be done after files are linked and options are interpreted. Any errors or
	// warnings encountered will be reported via the given handler. If any error
	// is reported, this function returns a non-nil error.
	ValidateExtensions(handler *reporter.Handler) error
	// CheckForUnusedImports is used to report warnings for unused imports. This
	// should be called after options have been interpreted. Otherwise, the logic
	// could incorrectly report imports as unused if the only symbol used were a
	// custom option.
	CheckForUnusedImports(handler *reporter.Handler)
}

// ErrorUnusedImport may be passed to a warning reporter when an unused
// import is detected. The error the reporter receives will be wrapped
// with source position that indicates the file and line where the import
// statement appeared.
type ErrorUnusedImport interface {
	error
	UnusedImport() string
}

type errUnusedImport string

func (e errUnusedImport) Error() string {
	return fmt.Sprintf("import %q not used", string(e))
}

func (e errUnusedImport) UnusedImport() string {
	return string(e)
}
