// Package cycle contains internal helpers for dealing with dependency cycles.
package cycle

import (
	"fmt"
	"strings"
)

// ErrCycle is an error due to cyclic dependencies.
type Error[T any] struct {
	// The offending cycle. The first and last entries will be equal.
	Cycle []T
}

// Error implements [error].
func (e *Error[T]) Error() string {
	var buf strings.Builder
	buf.WriteString("cycle detected: ")
	for i, q := range e.Cycle {
		if i != 0 {
			buf.WriteString(" -> ")
		}
		fmt.Fprintf(&buf, "%#v", q)
	}
	return buf.String()
}
