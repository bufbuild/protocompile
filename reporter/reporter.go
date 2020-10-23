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

type Reporter interface {
	Error(ErrorWithPos) error
	Warning(ErrorWithPos)
}

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

type Handler struct {
	reporter Reporter

	mu           sync.Mutex
	errsReported bool
	err          error
}

func NewHandler(rep Reporter) *Handler {
	if rep == nil {
		rep = NewReporter(nil, nil)
	}
	return &Handler{reporter: rep}
}

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

func (h *Handler) HandleWarning(pos ast.SourcePos, err error) {
	// no need for lock; warnings don't interact with mutable fields
	h.reporter.Warning(errorWithSourcePos{pos: pos, underlying: err})
}

func (h *Handler) Error() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.errsReported && h.err == nil {
		return ErrInvalidSource
	}
	return h.err
}

func (h *Handler) ReporterError() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	return h.err
}
