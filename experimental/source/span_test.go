// Copyright 2020-2026 Buf Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package source_test

import (
	"math/rand/v2"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/stretchr/testify/assert"

	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/experimental/source/length"
	"github.com/bufbuild/protocompile/internal"
	"github.com/bufbuild/protocompile/internal/ext/unicodex"
)

func TestLocation(t *testing.T) {
	t.Parallel()

	file := source.NewFile(
		"test",
		"foo\nbar\ncat: 🐈‍⬛\ntail",
	)

	tests := []struct {
		loc  source.Location
		unit length.Unit
	}{
		{loc: source.Location{0, 1, 1}, unit: length.Bytes},
		{loc: source.Location{0, 1, 1}, unit: length.UTF16},
		{loc: source.Location{0, 1, 1}, unit: length.Runes},
		{loc: source.Location{0, 1, 1}, unit: length.TermWidth},

		{loc: source.Location{2, 1, 3}, unit: length.Bytes},
		{loc: source.Location{2, 1, 3}, unit: length.UTF16},
		{loc: source.Location{2, 1, 3}, unit: length.Runes},
		{loc: source.Location{2, 1, 3}, unit: length.TermWidth},

		{loc: source.Location{13, 3, 6}, unit: length.Bytes},
		{loc: source.Location{13, 3, 6}, unit: length.UTF16},
		{loc: source.Location{13, 3, 6}, unit: length.Runes},
		{loc: source.Location{13, 3, 6}, unit: length.TermWidth},

		{loc: source.Location{23, 3, 16}, unit: length.Bytes},
		{loc: source.Location{23, 3, 10}, unit: length.UTF16},
		{loc: source.Location{23, 3, 9}, unit: length.Runes},
		{loc: source.Location{23, 3, 8}, unit: length.TermWidth},
		{loc: source.Location{24, 4, 1}, unit: length.UTF16},
		{loc: source.Location{27, 4, 4}, unit: length.UTF16},
		{loc: source.Location{28, 4, 5}, unit: length.UTF16},
		{loc: source.Location{28, 4, 5}, unit: length.Runes},
		{loc: source.Location{28, 4, 5}, unit: length.Bytes},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			t.Parallel()
			t.Logf("%q | %q", file.Text()[:test.loc.Offset], file.Text()[test.loc.Offset:])
			assert.Equal(t, test.loc, file.Location(test.loc.Offset, test.unit), "offset/%s -> line/col", test.unit)

			if test.unit != length.TermWidth {
				assert.Equal(t, test.loc, file.InverseLocation(test.loc.Line, test.loc.Column, test.unit), "line/col -> offset/%s", test.unit)
			}
		})
	}
}

func TestLocationLongLine(t *testing.T) {
	t.Parallel()

	// Measures the column from the start of the line, which is what Location
	// computed before it used checkpoints.
	reference := func(text string, offset int, unit length.Unit) source.Location {
		start := strings.LastIndexByte(text[:offset], '\n') + 1
		chunk := text[start:offset]
		column := 0
		switch unit {
		case length.Bytes:
			column = len(chunk)
		case length.Runes:
			for range chunk {
				column++
			}
		case length.UTF16:
			for _, r := range chunk {
				column += utf16.RuneLen(r)
			}
		case length.TermWidth:
			w := &unicodex.Width{}
			_, _ = w.WriteString(chunk)
			column = w.Column
		}
		return source.Location{
			Offset: offset,
			Line:   strings.Count(text[:offset], "\n") + 1,
			Column: column + 1,
		}
	}

	pieces := []string{
		"a", "b", " ", "  ", "\t", "\r", "\n", "\r\n", "\x01", "\xff", "é",
		"e\u0301", "\u0301", "\u4e2d", "🐈‍⬛", "\u200d", "🇺🇸", "🇺", "\u0600", "\U0001F44D\U0001F3FD",
	}
	rng := rand.New(rand.NewPCG(1, 2))
	for range 20 {
		var b strings.Builder
		for b.Len() < 4000 {
			// Mostly ASCII, so that long lines get checkpoints.
			if rng.IntN(4) != 0 {
				b.WriteString(pieces[rng.IntN(3)])
			} else {
				piece := pieces[rng.IntN(len(pieces))]
				if piece == "\n" || piece == "\r\n" {
					if rng.IntN(8) != 0 {
						continue
					}
				}
				b.WriteString(piece)
			}
		}
		text := b.String()
		file := source.NewFile("test", text)
		for offset := 1; offset <= len(text); offset++ {
			for _, unit := range []length.Unit{length.Bytes, length.Runes, length.UTF16, length.TermWidth} {
				want := reference(text, offset, unit)
				if got := file.Location(offset, unit); got != want {
					t.Fatalf("offset %d/%s: got %v, want %v", offset, unit, got, want)
				}
			}
		}
	}

	if internal.IsRace {
		t.Skip("skipping long line timing because race detector is enabled")
	}
	// Measuring each column from the start of the line made this quadratic.
	text := strings.Repeat("optional int32 a = 1; ", 1<<14)
	file := source.NewFile("test", text)
	start := time.Now()
	for offset := 0; offset < len(text); offset += 16 {
		for _, unit := range []length.Unit{length.Runes, length.UTF16, length.TermWidth} {
			file.Location(offset, unit)
		}
	}
	assert.Less(t, time.Since(start), 5*time.Second)
}
