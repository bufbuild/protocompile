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

package disassembler

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Options contains disassembly options.
type Options struct {
	ExplicitWireTypes      bool
	ExplicitLengthPrefixes bool
	NoGroups               bool
	MaxDepth               int
}

// Disassemble translates Protobuf wire format into protoscope text.
func Disassemble(data []byte, out io.Writer) error {
	return DisassembleWithOptions(data, out, Options{})
}

// DisassembleWithOptions translates Protobuf wire format into protoscope text with options.
func DisassembleWithOptions(data []byte, out io.Writer, opts Options) error {
	d := &disassembler{data: data, opts: opts}
	return d.disassemble(out, 0, 0, 0)
}

type disassembler struct {
	data []byte
	off  int
	opts Options
}

const (
	wireVarint = 0
	wireI64    = 1
	wireLen    = 2
	wireSGroup = 3
	wireEGroup = 4
	wireI32    = 5

	defaultMaxDepth = 10
)

var wireTypeNames = [...]string{
	wireVarint: "VARINT ",
	wireI64:    "I64 ",
	wireLen:    "LEN ",
	wireSGroup: "SGROUP ",
	wireEGroup: "EGROUP ",
	wireI32:    "I32 ",
}

func (d *disassembler) disassemble(out io.Writer, indent int, groupTag uint64, depth int) error {
	limit := d.opts.MaxDepth
	if limit <= 0 {
		limit = defaultMaxDepth
	}
	if depth > limit {
		return errors.New("max depth exceeded")
	}

	for d.off < len(d.data) {
		u, n := binary.Uvarint(d.data[d.off:])
		if n <= 0 {
			// Not a valid varint, dump remaining as hex
			fmt.Fprint(out, strings.Repeat("  ", indent))
			fmt.Fprintln(out, "# Error: invalid varint tag")
			return d.dumpHex(out, indent)
		}

		tag := u >> 3
		wireType := u & 0x7

		// If we're in a group and see an EGroup with the same tag, we're done.
		if groupTag != 0 && wireType == wireEGroup && tag == groupTag {
			if !d.opts.NoGroups {
				d.off += n
				return nil
			}
		}

		if wireType > 5 || tag == 0 {
			// Invalid wire type, this isn't a protobuf stream or it's corrupted.
			fmt.Fprint(out, strings.Repeat("  ", indent))
			if tag == 0 {
				fmt.Fprintln(out, "# Error: invalid tag 0; this might be using a different framing (e.g. gRPC)")
			} else {
				fmt.Fprintf(out, "# Error: invalid wire type %d; this might be corrupted or using a different framing (e.g. gRPC)\n", wireType)
			}
			return d.dumpHex(out, indent)
		}

		d.off += n

		tagPart := fmt.Sprintf("%d:", tag)
		if d.opts.ExplicitWireTypes {
			tagPart += wireTypeNames[wireType]
		} else {
			tagPart += " "
		}

		var valStr string
		var comment string
		var err error

		switch wireType {
		case wireVarint:
			v, vn := binary.Uvarint(d.data[d.off:])
			if vn <= 0 {
				err = fmt.Errorf("invalid varint at offset %d", d.off)
			} else {
				d.off += vn
				valStr = strconv.FormatUint(v, 10)
			}
		case wireI64:
			valStr, comment, err = d.disassembleI64()
		case wireLen:
			fmt.Fprint(out, strings.Repeat("  ", indent))
			fmt.Fprint(out, tagPart)
			err = d.disassembleLen(out, indent, depth)
			if err == nil {
				continue // Already handled line/block
			}
		case wireSGroup:
			if d.opts.NoGroups {
				fmt.Fprintln(out)
				continue
			}
			fmt.Fprint(out, strings.Repeat("  ", indent))
			fmt.Fprint(out, tagPart)
			err = d.disassembleSGroup(out, indent, tag, depth)
			if err == nil {
				continue
			}
		case wireEGroup:
			if d.opts.NoGroups {
				fmt.Fprintln(out)
				continue
			}
			// Should have been handled above if matching.
			valStr = "(unmatched EGroup)"
		case wireI32:
			valStr, comment, err = d.disassembleI32()
		default:
			valStr = fmt.Sprintf("(unsupported wire type %d)", wireType)
		}

		if err != nil {
			return err
		}

		if valStr != "" {
			fmt.Fprint(out, strings.Repeat("  ", indent))
			line := tagPart + valStr
			fmt.Fprint(out, line)
			if comment != "" {
				// Align to column 30 (including indentation)
				currentPos := indent*2 + len(line)
				padding := 30 - currentPos
				if padding < 1 {
					padding = 1
				}
				fmt.Fprint(out, strings.Repeat(" ", padding))
				fmt.Fprint(out, "# ")
				fmt.Fprint(out, comment)
			}
			fmt.Fprintln(out)
		}
	}
	return nil
}

func (d *disassembler) disassembleI64() (string, string, error) {
	if d.off+8 > len(d.data) {
		return "", "", errors.New("unexpected EOF reading I64")
	}
	payload := d.data[d.off : d.off+8]
	v := binary.LittleEndian.Uint64(payload)
	d.off += 8

	reps := possibilitiesI64(payload)
	var floatRep *Representation
	for _, r := range reps {
		if r.Type == "float64" {
			floatRep = &r
			break
		}
	}

	hexVal := fmt.Sprintf("0x%016xi64", v)
	if floatRep != nil && floatRep.Likelihood >= 0.5 {
		return floatRep.Text, hexVal, nil
	} else if floatRep != nil {
		return hexVal, floatRep.Text, nil
	}
	return hexVal, "", nil
}

func (d *disassembler) disassembleI32() (string, string, error) {
	if d.off+4 > len(d.data) {
		return "", "", errors.New("unexpected EOF reading I32")
	}
	payload := d.data[d.off : d.off+4]
	v := binary.LittleEndian.Uint32(payload)
	d.off += 4

	reps := possibilitiesI32(payload)
	var floatRep *Representation
	for _, r := range reps {
		if r.Type == "float32" {
			floatRep = &r
			break
		}
	}

	hexVal := fmt.Sprintf("0x%08xi32", v)
	if floatRep != nil && floatRep.Likelihood >= 0.5 {
		return floatRep.Text, hexVal, nil
	} else if floatRep != nil {
		return hexVal, floatRep.Text, nil
	}
	return hexVal, "", nil
}

func (d *disassembler) disassembleLen(out io.Writer, indent, depth int) error {
	l, n := binary.Uvarint(d.data[d.off:])
	if n <= 0 {
		return fmt.Errorf("invalid length at offset %d", d.off)
	}
	d.off += n
	if l > uint64(len(d.data)-d.off) {
		return fmt.Errorf("length %d out of bounds", l)
	}
	payload := d.data[d.off : d.off+int(l)]
	d.off += int(l)

	if d.opts.ExplicitLengthPrefixes {
		fmt.Fprintf(out, "%d ", l)
	}

	// Heuristic: Prefer string if it's cleanly printable and not obviously a message.
	switch {
	case isPrintable(payload) && !isMessage(payload):
		fmt.Fprintf(out, "{%q}\n", string(payload))
	case isMessage(payload):
		fmt.Fprint(out, "{\n")
		sub := &disassembler{data: payload, opts: d.opts}
		if err := sub.disassemble(out, indent+1, 0, depth+1); err != nil {
			if err.Error() == "max depth exceeded" {
				return err
			}
			// If recursion fails, fall back to hex for this payload
			fmt.Fprintf(out, " (fallback) `%s`", toHexSpace(payload))
		}
		fmt.Fprint(out, strings.Repeat("  ", indent))
		fmt.Fprint(out, "}\n")
	default:
		fmt.Fprintf(out, "{`%s`}\n", toHexSpace(payload))
	}
	return nil
}

func (d *disassembler) disassembleSGroup(out io.Writer, indent int, tag uint64, depth int) error {
	fmt.Fprint(out, "!{\n")
	if err := d.disassemble(out, indent+1, tag, depth+1); err != nil {
		return err
	}
	fmt.Fprint(out, strings.Repeat("  ", indent))
	fmt.Fprint(out, "}\n")
	return nil
}

func (d *disassembler) dumpHex(out io.Writer, indent int) error {
	if d.off >= len(d.data) {
		return nil
	}
	fmt.Fprint(out, strings.Repeat("  ", indent))
	fmt.Fprintf(out, "`%s`\n", toHexSpace(d.data[d.off:]))
	d.off = len(d.data)
	return nil
}

func checkMessageStructure(data []byte) (ok bool, fields int) {
	if len(data) == 0 {
		return false, 0
	}
	off := 0
	for off < len(data) {
		u, n := binary.Uvarint(data[off:])
		if n <= 0 {
			return false, 0
		}
		off += n
		wireType := u & 0x7
		tag := u >> 3
		if wireType > 5 || tag == 0 {
			return false, 0
		}
		fields++
		switch wireType {
		case wireVarint:
			_, n = binary.Uvarint(data[off:])
			if n <= 0 {
				return false, 0
			}
			off += n
		case wireI64:
			if off+8 > len(data) {
				return false, 0
			}
			off += 8
		case wireLen:
			l, n := binary.Uvarint(data[off:])
			if n <= 0 {
				return false, 0
			}
			off += n
			if l > uint64(len(data)-off) {
				return false, 0
			}
			off += int(l)
		case wireSGroup, wireEGroup:
			// Groups are not supported for simple structural check here
			return false, 0
		case wireI32:
			if off+4 > len(data) {
				return false, 0
			}
			off += 4
		default:
			return false, 0
		}
	}
	return off == len(data), fields
}

func isMessage(data []byte) bool {
	ok, fields := checkMessageStructure(data)
	if !ok || fields == 0 {
		return false
	}
	// Heuristic: if it has many fields, it's likely a message even if it looks like a string.
	if fields > 3 {
		return true
	}
	// If it's short and mostly printable, it's likely a string.
	if isMostlyPrintable(data) {
		return false
	}
	return true
}

func isPrintable(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	if !utf8.Valid(data) {
		return false
	}
	for _, r := range string(data) {
		if !unicode.IsPrint(r) && !unicode.IsSpace(r) {
			return false
		}
	}
	return true
}

func isMostlyPrintable(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	if !utf8.Valid(data) {
		return false
	}
	printable := 0
	total := 0
	for _, r := range string(data) {
		total++
		if unicode.IsPrint(r) || unicode.IsSpace(r) {
			printable++
		}
	}
	if total == 0 {
		return false
	}
	return printable*10 > total*8
}

func toHexSpace(data []byte) string {
	var sb strings.Builder
	for i, b := range data {
		if i > 0 {
			sb.WriteByte(' ')
		}
		fmt.Fprintf(&sb, "%02x", b)
	}
	return sb.String()
}
