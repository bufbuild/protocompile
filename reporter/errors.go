package reporter

import (
	"errors"
	"fmt"

	"github.com/jhump/protocompile/ast"
)

// ErrInvalidSource is a sentinel error that is returned by calls to
// Parser.ParseFiles and Parser.ParseFilesButDoNotLink in the event that syntax
// or link errors are encountered, but the parser's configured ErrorReporter
// always returns nil.
var ErrInvalidSource = errors.New("parse failed: invalid proto source")

// ErrorWithPos is an error about a proto source file that includes information
// about the location in the file that caused the error.
//
// The value of Error() will contain both the SourcePos and Underlying error.
// The value of Unwrap() will only be the Underlying error.
type ErrorWithPos interface {
	error
	GetPosition() ast.SourcePos
	Unwrap() error
}

func Error(pos ast.SourcePos, err error) ErrorWithPos {
	return errorWithSourcePos{pos: pos, underlying: err}
}

func Errorf(pos ast.SourcePos, format string, args ...interface{}) ErrorWithPos {
	return errorWithSourcePos{pos: pos, underlying: fmt.Errorf(format, args...)}
}

// errorWithSourcePos is an error about a proto source file that includes
// information about the location in the file that caused the error.
//
// Errors that include source location information *might* be of this type.
// However, calling code that is trying to examine errors with location info
// should instead look for instances of the ErrorWithPos interface, which
// will find other kinds of errors. This type is only exported for backwards
// compatibility.
//
// SourcePos should always be set and never nil.
type errorWithSourcePos struct {
	underlying error
	pos        ast.SourcePos
}

func (e errorWithSourcePos) Error() string {
	sourcePos := e.GetPosition()
	return fmt.Sprintf("%s: %v", sourcePos, e.underlying)
}

// GetPosition implements the ErrorWithPos interface, supplying a location in
// proto source that caused the error.
func (e errorWithSourcePos) GetPosition() ast.SourcePos {
	return e.pos
}

// Unwrap implements the ErrorWithPos interface, supplying the underlying
// error. This error will not include location information.
func (e errorWithSourcePos) Unwrap() error {
	return e.underlying
}

var _ ErrorWithPos = errorWithSourcePos{}
