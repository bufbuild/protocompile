// Package parser contains the logic for parsing protobuf source code into an
// AST (abstract syntax tree) and also for converting an AST into a descriptor
// proto.
//
// A FileDescriptorProto is very similar to an AST, but the AST this package
// uses is more useful because it contains more information about the source
// code, including details about whitespace and comments, that cannot be
// represented by a descriptor proto. This makes it ideal for things like
// code formatters, which may want to preserve things like whitespace and
// comment format.
package parser
