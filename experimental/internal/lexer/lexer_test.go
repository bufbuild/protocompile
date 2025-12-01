// Copyright 2020-2025 Buf Technologies, Inc.
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

//nolint:tagliatelle
package lexer_test

import (
	"bytes"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert/yaml"
	"github.com/stretchr/testify/require"

	"github.com/bufbuild/protocompile/experimental/internal/lexer"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/experimental/source/length"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/experimental/token/keyword"
	"github.com/bufbuild/protocompile/internal/golden"
)

// Config is configuration settable in a text via //% comments.
type Config struct {
	Keywords struct {
		Hard         []string `yaml:"hard"`
		Soft         []string `yaml:"soft"`
		Bracket      []string `yaml:"bracket"`
		LineComment  []string `yaml:"line_comment"`
		BlockComment []string `yaml:"block_comment"`

		Map map[keyword.Keyword]lexer.OnKeyword `yaml:"-"`
	} `yaml:"keywords"`

	Prefixes struct {
		Numbers []string `yaml:"numbers"`
		Strings []string `yaml:"strings"`
	} `yaml:"prefixes"`

	Suffixes struct {
		Numbers []string `yaml:"numbers"`
		Strings []string `yaml:"strings"`
	} `yaml:"suffixes"`

	EmitNewline *struct {
		NotAfter  []string `yaml:"not_after"`
		NotBefore []string `yaml:"not_before"`
	} `yaml:"emit_newline"`

	NumberCanStartWithDot bool `yaml:"number_can_start_with_dot"`
	OldStyleOctal         bool `yaml:"old_style_octal"`
	RequireASCIIIdent     bool `yaml:"require_ascii_ident"`

	Escapes struct {
		Extended        bool `yaml:"extended"`
		Ask             bool `yaml:"ask"`
		Octal           bool `yaml:"octal"`
		PartialX        bool `yaml:"partial_x"`
		UppercaseX      bool `yaml:"uppercase_x"`
		OldStyleUnicode bool `yaml:"old_style_unicode"`
	}
}

func (c *Config) Parse(t *testing.T, text string) {
	t.Helper()

	config := new(bytes.Buffer)
	for line := range strings.Lines(text) {
		if line, ok := strings.CutPrefix(line, "//% "); ok {
			config.WriteString(line)
		}
	}

	err := yaml.Unmarshal(config.Bytes(), c)
	require.NoError(t, err)

	c.Keywords.Map = make(map[keyword.Keyword]lexer.OnKeyword)
	addKeywords := func(what lexer.OnKeyword, names []string) {
		for _, name := range names {
			kw := keyword.Lookup(name)
			require.NotEqual(t, keyword.Unknown, kw, "name: %s", name)
			c.Keywords.Map[kw] = what
		}
	}
	addKeywords(lexer.HardKeyword, c.Keywords.Hard)
	addKeywords(lexer.SoftKeyword, c.Keywords.Soft)
	addKeywords(lexer.BracketKeyword, c.Keywords.Bracket)
	addKeywords(lexer.LineComment, c.Keywords.LineComment)
	addKeywords(lexer.BlockComment, c.Keywords.BlockComment)
}

func TestLexer(t *testing.T) {
	t.Parallel()

	corpus := golden.Corpus{
		Root:       "testdata",
		Refresh:    "PROTOCOMPILE_REFRESH",
		Extensions: []string{"proto"},
		Outputs: []golden.Output{
			{Extension: "tokens.tsv"},
			{Extension: "stderr.txt"},
		},
	}

	corpus.Run(t, func(t *testing.T, path, text string, outputs []string) {
		config := new(Config)
		config.Parse(t, text)

		text = unescapeTestCase(text)

		r := &report.Report{Options: report.Options{Tracing: 10}}
		file := source.NewFile(path, text)
		lex := lexer.Lexer{
			OnKeyword: func(k keyword.Keyword) lexer.OnKeyword {
				return config.Keywords.Map[k]
			},

			IsAffix: func(affix string, kind token.Kind, suffix bool) bool {
				switch kind {
				case token.Number:
					if suffix {
						return slices.Contains(config.Suffixes.Numbers, affix)
					}
					return slices.Contains(config.Prefixes.Numbers, affix)

				case token.String:
					if suffix {
						return slices.Contains(config.Suffixes.Strings, affix)
					}
					return slices.Contains(config.Prefixes.Strings, affix)

				default:
					return false
				}
			},

			NumberCanStartWithDot: config.NumberCanStartWithDot,
			OldStyleOctal:         config.OldStyleOctal,
			RequireASCIIIdent:     config.RequireASCIIIdent,
			EscapeExtended:        config.Escapes.Extended,
			EscapeAsk:             config.Escapes.Ask,
			EscapeOctal:           config.Escapes.Octal,
			EscapePartialX:        config.Escapes.PartialX,
			EscapeUppercaseX:      config.Escapes.UppercaseX,
			EscapeOldStyleUnicode: config.Escapes.OldStyleUnicode,
		}

		if config.EmitNewline != nil {
			lex.EmitNewline = func(before, after token.Token) bool {
				return !slices.Contains(config.EmitNewline.NotAfter, before.LeafSpan().Text()) &&
					!slices.Contains(config.EmitNewline.NotBefore, after.LeafSpan().Text())
			}
		}

		stream := lex.Lex(file, r)
		for _, d := range r.Diagnostics {
			if d.Level() == report.ICE {
				t.Fail()
			}
		}

		stderr, _, _ := report.Renderer{
			Colorize:  true,
			ShowDebug: true,
		}.RenderString(r)
		t.Log(stderr)
		outputs[1], _, _ = report.Renderer{}.RenderString(r)

		var tsv strings.Builder
		var count int
		tsv.WriteString("#\t\tkind\t\tkeyword\t\toffsets\t\tlinecol\t\ttext\n")
		for tok := range stream.All() {
			count++

			sp := tok.Span()
			start := stream.Location(sp.Start, length.TermWidth)
			_, kw, _ := strings.Cut(tok.Keyword().GoString(), ".")
			fmt.Fprintf(
				&tsv, "%v\t\t%v\t\t%v\t\t%03d:%03d\t\t%03d:%03d\t\t%q",
				int32(tok.ID())-1,
				tok.Kind(), kw,
				sp.Start, sp.End,
				start.Line, start.Column,
				tok.Text(),
			)

			switch tok.Kind() {
			case token.Number:
				n := tok.AsNumber()
				if n.IsFloat() {
					fmt.Fprintf(&tsv, "\t\tfp:%g", n.Value())
				} else {
					fmt.Fprintf(&tsv, "\t\tint:%.0f", n.Value())
				}
				fmt.Fprintf(&tsv, "/%v/%v", n.Base(), n.ExpBase())

				if prefix := n.Prefix().Text(); prefix != "" {
					fmt.Fprintf(&tsv, "\t\tpre:%q", prefix)
				}

				if suffix := n.Suffix().Text(); suffix != "" {
					fmt.Fprintf(&tsv, "\t\tsuf:%q", suffix)
				}

			case token.String:
				s := tok.AsString()
				fmt.Fprintf(&tsv, "\t\tstring:%q", s.Text())

				if prefix := s.Prefix().Text(); prefix != "" {
					fmt.Fprintf(&tsv, "\t\tpre:%q", prefix)
				}
			}

			if a, b := tok.StartEnd(); a != b {
				if tok == a {
					fmt.Fprintf(&tsv, "\t\tclose:%v", b.ID())
				} else {
					fmt.Fprintf(&tsv, "\t\topen:%v", a.ID())
				}
			}

			tsv.WriteByte('\n')
		}
		if count > 0 {
			outputs[0] = tsv.String()
		}
	})
}

// Our lexer tests support Unicode escapes in the form $u{nnnn} and byte escapes
// in the form $x{nn}. This is so that the checked-in files are human-readable
// while potentially containing unprintable characters or invalid bytes.
var escapePat = regexp.MustCompile(`\$([ux])\{(\w+)\}`)

func unescapeTestCase(s string) string {
	return escapePat.ReplaceAllStringFunc(s, func(needle string) string {
		groups := escapePat.FindStringSubmatch(needle)
		value, err := strconv.ParseInt(groups[2], 16, 32)
		if err != nil {
			panic(err)
		}

		if groups[1] == "x" {
			return string([]byte{byte(value)})
		}
		return string(rune(value))
	})
}
