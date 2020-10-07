package parser

import "errors"

// ErrNoSyntax is a sentinel error that may be passed to a warning reporter.
// The error the reporter receives will be wrapped with source position that
// indicates the file that had no syntax statement.
var ErrNoSyntax = errors.New("no syntax specified; defaulting to proto2 syntax")
