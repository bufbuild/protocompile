// Package editionstesting is a temporary package that allows users to test
// functionality related to Protobuf editions while that support is not yet
// complete. Once that support is complete, this package will be removed.
package editionstesting

import "github.com/bufbuild/protocompile/internal"

// AllowEditions can be called to opt into this repo's support for Protobuf
// editions. This is primarily intended for testing. This repo's support
// for editions is not yet complete.
//
// Once the implementation of editions is complete, this function will be
// REMOVED and editions will be allowed by all usages of this repo.
//
// The internal flag that this touches is not synchronized and calling this
// function is not thread-safe. So this must be called prior to using the
// compiler, ideally from an init() function or as one of the first things
// done from a main() function.
func AllowEditions() {
	internal.AllowEditions = true
}
