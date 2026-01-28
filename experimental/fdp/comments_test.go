package fdp

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/experimental/token"
)

func TestLineComments(t *testing.T) {
	t.Parallel()

	s := &token.Stream{
		File: source.NewFile("test",
			`// Line 1
// Line 2`),
	}
	line1 := s.Push(9, token.Comment)
	newline := s.Push(1, token.Space)
	line2 := s.Push(9, token.Comment)

	tokens := paragraph([]token.Token{line1, newline, line2})
	assert.Equal(t, " Line 1\n Line 2", tokens.stringify())
}

func TestBlockComments(t *testing.T) {
	t.Parallel()

	s := &token.Stream{
		File: source.NewFile("test",
			`/*
* Line 1
* Line 2
*/`),
	}
	block := s.Push(21, token.Comment)

	tokens := paragraph([]token.Token{block})
	assert.Equal(t, "\n Line 1\n Line 2\n", tokens.stringify())
}
