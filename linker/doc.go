// Package linker contains logic and APIs related to linking a protobuf file.
// The process of linking involves resolving all symbol references to the
// referenced descriptor. The result of linking is a "rich" descriptor that
// is more useful than just a descriptor proto since the links allow easy
// traversal of a protobuf type schema and the relationships between elements.
//
// Files
//
// This package uses an augmentation to protoreflect.FileDescriptor instances
// in the form of the File interface. There are also factory functions for
// promoting a FileDescriptor into a linker.File. This new interface provides
// additional methods for resolving symbols in the file.
//
// This interface is both the result of linking but also an input to the linking
// process, as all dependencies of a file to be linked must be provided in this
// form. The actual result of the Link function is a super-interface of File.
//
// Symbols
//
// This package has a type named Symbols which represents a symbol table. This
// is usually an internal detail when linking, but callers can provide an
// instance so that symbols across multiple compile/link operations all have
// access to the same table. This allows for detection of cases where multiple
// files that try to declare elements with conflicting fully-qualified names or
// declaring extensions for a particular extendable message that have
// conflicting tag numbers.
//
// The calling code simply uses the same Symbols instance across all compile
// operations and if any files processed have such conflicts, they can be
// reported.
package linker
