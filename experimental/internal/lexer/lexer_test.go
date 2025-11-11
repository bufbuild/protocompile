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

package lexer_test

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/bufbuild/protocompile/experimental/internal/lexer"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/experimental/token/keyword"
	"github.com/bufbuild/protocompile/internal/golden"
)

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
		text = unescapeTestCase(text)

		r := &report.Report{Options: report.Options{Tracing: 10}}
		file := report.NewFile(path, text)
		lex := lexer.Lexer{
			OnKeyword: func(k keyword.Keyword) lexer.OnKeyword {
				switch k {
				case keyword.Comment:
					return lexer.LineComment
				case keyword.LComment, keyword.RComment:
					return lexer.BlockComment
				case keyword.LParen, keyword.LBracket, keyword.LBrace,
					keyword.RParen, keyword.RBracket, keyword.RBrace:
					return lexer.BracketKeyword
				default:
					if k.IsProtobuf() || k.IsCEL() {
						return lexer.KeepKeyword
					}
					return lexer.DiscardKeyword
				}
			},

			NumberCanStartWithDot: true,
			OldStyleOctal:         true,
			RequireASCIIIdent:     true,
			EscapeExtended:        true,
			EscapeAsk:             true,
			EscapeOctal:           true,
			EscapePartialX:        true,
			EscapeUppercaseX:      true,
			EscapeOldStyleUnicode: true,
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
			start := stream.Location(sp.Start, report.TermWidth)
			fmt.Fprintf(
				&tsv, "%v\t\t%v\t\t%#v\t\t%03d:%03d\t\t%03d:%03d\t\t%q",
				int32(tok.ID())-1,
				tok.Kind(), tok.Keyword(),
				sp.Start, sp.End,
				start.Line, start.Column,
				tok.Text(),
			)

			switch tok.Kind() {
			case token.Number:
				n := tok.AsNumber()
				v := n.Value()
				if v.IsInt() {
					fmt.Fprintf(&tsv, "\t\tnum:%.0f", n.Value())
				} else {
					fmt.Fprintf(&tsv, "\t\tnum:%g", n.Value())
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
