// Package reporter contains the types used for reporting errors from
// protocompile operations. It contains error types as well as interfaces
// for reporting and handling errors.
package reporter

import (
	"sync"

	"github.com/jhump/protocompile/ast"
)

// ErrorReporter is responsible for reporting the given error. If the reporter
// returns a non-nil error, parsing/linking will abort with that error. If the
// reporter returns nil, parsing will continue, allowing the parser to try to
// report as many syntax and/or link errors as it can find.
type ErrorReporter func(err ErrorWithPos) error

// WarningReporter is responsible for reporting the given warning. This is used
// for indicating non-error messages to the calling program for things that do
// not cause the parse to fail but are considered bad practice. Though they are
// just warnings, the details are supplied to the reporter via an error type.
type WarningReporter func(ErrorWithPos)

// Reporter is a type that handles reporting both errors and warnings.
type Reporter interface {
	// Error is called when the given error is encountered and needs to be
	// reported to the calling program. This signature matches ErrorReporter
	// because it has the same semantics. If this function returns non-nil
	// then the operation will abort immediately with the given error. But
	// if it returns nil, the operation will continue, reporting more errors
	// as they are encountered. If the reporter never returns non-nil then
	// the operation will eventually fail with ErrInvalidSource.
	Error(ErrorWithPos) error
	// Warning is called when the given warnings is encountered and needs to be
	// reported to the calling program. Despite the argument being an error
	// type, a warning will never cause the operation to abort or fail (unless
	// the reporter's implementation of this method panics).
	Warning(ErrorWithPos)
}

// NewReporter creates a new reporter that invokes the given functions on error
// or warning.
func NewReporter(errs ErrorReporter, warnings WarningReporter) Reporter {
	return reporterFuncs{errs: errs, warnings: warnings}
}

type reporterFuncs struct {
	errs     ErrorReporter
	warnings WarningReporter
}

func (r reporterFuncs) Error(err ErrorWithPos) error {
	if r.errs == nil {
		return err
	}
	return r.errs(err)
}

func (r reporterFuncs) Warning(err ErrorWithPos) {
	if r.warnings != nil {
		r.warnings(err)
	}
}

// Handler is used by protocompile operations for handling errors and warnings.
type Handler struct {
	reporter Reporter

	mu           sync.Mutex
	errsReported bool
	err          error
}

// NewHandler creates a new Handler that reports errors and warnings using the
// given reporter.
func NewHandler(rep Reporter) *Handler {
	if rep == nil {
		rep = NewReporter(nil, nil)
	}
	return &Handler{reporter: rep}
}

// HandleErrorf handles an error with the given source position, creating the
// error using the given message format and arguments.
//
// If the handler has already aborted (by returning a non-nil error from a call
// to HandleError or HandleErrorf), that same error is returned and the given
// error is not reported.
func (h *Handler) HandleErrorf(pos ast.SourcePos, format string, args ...interface{}) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.err != nil {
		return h.err
	}
	h.errsReported = true
	err := h.reporter.Error(Errorf(pos, format, args...))
	h.err = err
	return err
}

// HandleError handles the given error. If the given err is an ErrorWithPos, it
// is reported, and this function returns the error returned by the reporter. If
// the given err is NOT an ErrorWithPos, the current operation will abort
// immediately.
//
// If the handler has already aborted (by returning a non-nil error from a call
// to HandleError or HandleErrorf), that same error is returned and the given
// error is not reported.
func (h *Handler) HandleError(err error) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.err != nil {
		return h.err
	}
	if ewp, ok := err.(ErrorWithPos); ok {
		h.errsReported = true
		err = h.reporter.Error(ewp)
	}
	h.err = err
	return err
}

// HandleWarning handles a warning with the given source position. This will
// delegate to the handler's configured reporter.
func (h *Handler) HandleWarning(pos ast.SourcePos, err error) {
	// no need for lock; warnings don't interact with mutable fields
	h.reporter.Warning(errorWithSourcePos{pos: pos, underlying: err})
}

// Error returns the handler result. If any errors have been reported then this
// returns a non-nil error. If the reporter never returned a non-nil error then
// ErrInvalidSource is returned. Otherwise, this returns the error returned by
// the  handler's reporter (the same value returned by ReporterError).
func (h *Handler) Error() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.errsReported && h.err == nil {
		return ErrInvalidSource
	}
	return h.err
}

// ReporterError returns the error returned by the handler's reporter. If
// the reporter has either not been invoked (no errors handled) or has not
// returned any non-nil value, then this returns nil.
func (h *Handler) ReporterError() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	return h.err
}
