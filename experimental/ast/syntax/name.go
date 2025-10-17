package syntax

import (
	"fmt"
	"strconv"
)

var names = func() map[Syntax]string {
	names := make(map[Syntax]string)
	for syntax := range All() {
		if syntax.IsEdition() {
			names[syntax] = fmt.Sprintf("Edition %s", syntax)
		} else {
			names[syntax] = strconv.Quote(syntax.String())
		}
	}
	return names
}()

// Name returns the name of this syntax as it should appear in diagnostics.
func (s Syntax) Name() string {
	name, ok := names[s]
	if !ok {
		return "Edition <?>"
	}
	return name
}
