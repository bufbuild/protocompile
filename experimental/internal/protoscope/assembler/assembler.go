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

package assembler

import (
	"encoding/binary"
	"encoding/hex"
	"math"
	"strings"
	"unicode"

	"github.com/bufbuild/protocompile/experimental/id"
	"github.com/bufbuild/protocompile/experimental/internal/protoscope/ast"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/experimental/token/keyword"
)

// Assemble translates a protoscope AST into Protobuf wire format.
func Assemble(file *ast.File) []byte {
	a := &assembler{}
	for decl := range seq.Values(file.Decls()) {
		a.assembleDecl(decl, false)
	}
	return a.buf
}

type assembler struct {
	buf []byte
}

func (a *assembler) assembleDecl(decl ast.DeclAny, inBlock bool) {
	switch decl.Kind() {
	case ast.DeclKindField:
		a.assembleField(id.Wrap(decl.Context(), id.ID[ast.Field](decl.ID().Value())))
	case ast.DeclKindLiteral:
		a.assembleLiteral(id.Wrap(decl.Context(), id.ID[ast.Literal](decl.ID().Value())), inBlock)
	case ast.DeclKindBlock:
		a.assembleBlock(id.Wrap(decl.Context(), id.ID[ast.Block](decl.ID().Value())), inBlock)
	}
}

func (a *assembler) assembleField(f ast.Field) {
	tag, _ := f.Tag().AsNumber().Int()
	wireType := uint64(0)
	val := f.Value()
	if !val.IsZero() {
		switch val.Kind() {
		case ast.DeclKindBlock:
			block := id.Wrap(val.Context(), id.ID[ast.Block](val.ID().Value()))
			if block.Token().Keyword() == keyword.Bang {
				wireType = 3 // SGROUP
			} else {
				wireType = 2 // LEN
			}
		case ast.DeclKindLiteral:
			lit := id.Wrap(val.Context(), id.ID[ast.Literal](val.ID().Value()))
			if lit.Token().Kind() == token.String {
				wireType = 2 // LEN
			} else if lit.Token().Kind() == token.Number {
				num := lit.Token().AsNumber()
				suffix := num.Suffix().Text()
				switch suffix {
				case "i64", "f64":
					wireType = 1 // I64
				case "i32", "f32":
					wireType = 5 // I32
				default:
					if num.IsFloat() {
						wireType = 1 // I64 (double)
					}
				}
			}
		}
	}

	// Override with explicit wire type hint.
	switch f.WireType().Text() {
	case "VARINT":
		wireType = 0
	case "I64":
		wireType = 1
	case "LEN":
		wireType = 2
	case "SGROUP":
		wireType = 3
	case "EGROUP":
		wireType = 4
	case "I32":
		wireType = 5
	}

	a.writeVarint(tag<<3 | wireType)

	if wireType == 4 { // EGROUP
		return
	}

	if wireType == 3 { // SGROUP
		a.assembleDecl(val, false)
		a.writeVarint(tag<<3 | 4) // Emit matching EGROUP
		return
	}

	if wireType == 2 { // LEN
		// Length-delimited fields must be prefixed by their length.
		// Some values (blocks and strings) are self-length-delimited.
		isSelfDelimited := false
		if !val.IsZero() && val.Kind() == ast.DeclKindBlock {
			isSelfDelimited = true
		}
		if !val.IsZero() && val.Kind() == ast.DeclKindLiteral {
			lit := id.Wrap(val.Context(), id.ID[ast.Literal](val.ID().Value()))
			if lit.Token().Kind() == token.String {
				isSelfDelimited = true
			}
		}

		if isSelfDelimited {
			a.assembleDecl(val, false)
		} else {
			sub := &assembler{}
			sub.assembleDecl(val, false)
			a.writeVarint(uint64(len(sub.buf)))
			a.buf = append(a.buf, sub.buf...)
		}
		return
	}

	a.assembleDecl(val, false)
}

func (a *assembler) assembleLiteral(l ast.Literal, inBlock bool) {
	tok := l.Token()
	switch tok.Keyword() {
	case keyword.True:
		a.writeVarint(1)
		return
	case keyword.False:
		a.writeVarint(0)
		return
	}

	switch tok.Kind() {
	case token.Number:
		// Check for suffix hints
		num := tok.AsNumber()
		suffix := num.Suffix().Text()
		switch suffix {
		case "i32":
			var buf [4]byte
			if num.IsFloat() {
				f, _ := num.Float()
				binary.LittleEndian.PutUint32(buf[:], math.Float32bits(float32(f)))
			} else {
				v, _ := num.Int()
				binary.LittleEndian.PutUint32(buf[:], uint32(v))
			}
			a.buf = append(a.buf, buf[:]...)
		case "f32":
			f, _ := num.Float()
			var buf [4]byte
			binary.LittleEndian.PutUint32(buf[:], math.Float32bits(float32(f)))
			a.buf = append(a.buf, buf[:]...)
		case "i64":
			var buf [8]byte
			if num.IsFloat() {
				f, _ := num.Float()
				binary.LittleEndian.PutUint64(buf[:], math.Float64bits(f))
			} else {
				v, _ := num.Int()
				binary.LittleEndian.PutUint64(buf[:], v)
			}
			a.buf = append(a.buf, buf[:]...)
		case "f64":
			f, _ := num.Float()
			var buf [8]byte
			binary.LittleEndian.PutUint64(buf[:], math.Float64bits(f))
			a.buf = append(a.buf, buf[:]...)
		case "z":
			v := num.Value().Int(nil).Int64()
			// Zigzag encoding: (n << 1) ^ (n >> 63)
			zigzag := uint64((v << 1) ^ (v >> 63))
			a.writeVarint(zigzag)
		default:
			if num.IsFloat() {
				f, _ := num.Float()
				var buf [8]byte
				binary.LittleEndian.PutUint64(buf[:], math.Float64bits(f))
				a.buf = append(a.buf, buf[:]...)
			} else {
				v, _ := num.Int()
				a.writeVarint(v)
			}
		}
	case token.String:
		open, _ := tok.AsString().Quotes()
		isHex := open.Text() == "`"
		var contentBytes []byte
		if isHex {
			// Decode hex string
			var sb strings.Builder
			for _, r := range tok.AsString().Text() {
				if !unicode.IsSpace(r) {
					sb.WriteRune(r)
				}
			}
			var err error
			contentBytes, err = hex.DecodeString(sb.String())
			if err != nil {
				// Fallback to raw string text if decoding fails
				contentBytes = []byte(tok.AsString().Text())
			}
		} else {
			contentBytes = []byte(tok.AsString().Text())
		}

		if inBlock {
			a.buf = append(a.buf, contentBytes...)
		} else {
			a.writeVarint(uint64(len(contentBytes)))
			a.buf = append(a.buf, contentBytes...)
		}
	}
}

func (a *assembler) assembleBlock(b ast.Block, _ bool) {
	tok := b.Token()
	switch tok.Keyword() {
	case keyword.LBracket, keyword.Brackets, keyword.LBrace, keyword.Braces:
		// Length-prefixed block
		sub := &assembler{}
		for decl := range seq.Values(b.Decls()) {
			sub.assembleDecl(decl, true)
		}
		a.writeVarint(uint64(len(sub.buf)))
		a.buf = append(a.buf, sub.buf...)
	case keyword.Bang:
		// Group content (no length prefix)
		for decl := range seq.Values(b.Decls()) {
			a.assembleDecl(decl, false)
		}
	}
}

func (a *assembler) writeVarint(v uint64) {
	var buf [10]byte
	n := binary.PutUvarint(buf[:], v)
	a.buf = append(a.buf, buf[:n]...)
}
