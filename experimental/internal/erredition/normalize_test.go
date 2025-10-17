package erredition

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalize(t *testing.T) {
	t.Parallel()

	assert.Equal(t,
		"the legacy closed enum behavior in Java is deprecated and is scheduled to be removed in Edition 2025",
		normalizeReason("The legacy closed enum behavior in Java is deprecated and is scheduled to be removed in edition 2025.  See http://protobuf.dev/programming-guides/enum/#java for more information."),
	)
}
