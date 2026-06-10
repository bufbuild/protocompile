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
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Representation represents a possible translation/formatting of a protobuf value.
type Representation struct {
	Type        string  // E.g., "message", "string", "bytes", "varint", "zigzag", "bool", "fixed32", "float32", "fixed64", "float64", "packed_varint", "packed_fixed32", "packed_fixed64"
	Text        string  // The protoscope textual value representation
	Description string  // Human-readable description
	Likelihood  float64 // Likelihood score, between 0.0 and 1.0 (higher is more likely)
}

// Possibilities analyzes the raw payload bytes for a given wire type and returns
// all valid alternative representations sorted by likelihood.
func Possibilities(wireType int, payload []byte) []Representation {
	var reps []Representation

	switch wireType {
	case wireVarint:
		reps = possibilitiesVarint(payload)
	case wireI32:
		reps = possibilitiesI32(payload)
	case wireI64:
		reps = possibilitiesI64(payload)
	case wireLen:
		reps = possibilitiesLen(payload)
	}

	sort.Slice(reps, func(i, j int) bool {
		if reps[i].Likelihood == reps[j].Likelihood {
			return reps[i].Description < reps[j].Description
		}
		return reps[i].Likelihood > reps[j].Likelihood
	})

	return reps
}

func possibilitiesVarint(payload []byte) []Representation {
	val, n := binary.Uvarint(payload)
	if n <= 0 || n < len(payload) {
		return nil
	}

	var reps []Representation

	// 1. Unsigned Varint (Decimal)
	reps = append(reps, Representation{
		Type:        "varint",
		Text:        strconv.FormatUint(val, 10),
		Description: "Varint",
		Likelihood:  0.9,
	})

	// 2. Zigzag Varint
	zz := int64(val>>1) ^ -int64(val&1)
	reps = append(reps, Representation{
		Type:        "zigzag",
		Text:        strconv.FormatInt(zz, 10),
		Description: "Zigzag Varint",
		Likelihood:  0.7,
	})

	// 3. Boolean
	switch val {
	case 0:
		reps = append(reps, Representation{
			Type:        "bool",
			Text:        "false",
			Description: "Boolean",
			Likelihood:  0.8,
		})
	case 1:
		reps = append(reps, Representation{
			Type:        "bool",
			Text:        "true",
			Description: "Boolean",
			Likelihood:  0.8,
		})
	}

	return reps
}

func possibilitiesI32(payload []byte) []Representation {
	if len(payload) != 4 {
		return nil
	}
	val := binary.LittleEndian.Uint32(payload)

	reps := make([]Representation, 0, 3)

	// 1. Fixed32 (Hex)
	reps = append(reps, Representation{
		Type:        "fixed32",
		Text:        fmt.Sprintf("0x%08xi32", val),
		Description: "Fixed32 (Hex)",
		Likelihood:  0.9,
	})

	// 2. Fixed32 (Decimal)
	reps = append(reps, Representation{
		Type:        "fixed32",
		Text:        fmt.Sprintf("%di32", int32(val)),
		Description: "Fixed32 (Decimal)",
		Likelihood:  0.8,
	})

	// 3. Float32
	fval := math.Float32frombits(val)
	f64 := float64(fval)
	text := fmt.Sprintf("%g", fval)
	text = strings.Replace(text, "e+", "e", 1)
	text += "i32"
	if !strings.Contains(text, ".") && !strings.Contains(text, "e") && !math.IsNaN(f64) && !math.IsInf(f64, 0) {
		text = fmt.Sprintf("%.1fi32", fval)
	}
	var likelihood float64
	switch {
	case math.IsNaN(f64) || math.IsInf(f64, 0):
		likelihood = 0.2
	case fval == 0.0 || (math.Abs(f64) > 1e-6 && math.Abs(f64) < 1e6):
		likelihood = 0.7
	default:
		likelihood = 0.5
	}
	reps = append(reps, Representation{
		Type:        "float32",
		Text:        text,
		Description: "Float32",
		Likelihood:  likelihood,
	})

	return reps
}

func possibilitiesI64(payload []byte) []Representation {
	if len(payload) != 8 {
		return nil
	}
	val := binary.LittleEndian.Uint64(payload)

	reps := make([]Representation, 0, 3)

	// 1. Fixed64 (Hex)
	reps = append(reps, Representation{
		Type:        "fixed64",
		Text:        fmt.Sprintf("0x%016xi64", val),
		Description: "Fixed64 (Hex)",
		Likelihood:  0.9,
	})

	// 2. Fixed64 (Decimal)
	reps = append(reps, Representation{
		Type:        "fixed64",
		Text:        fmt.Sprintf("%di64", int64(val)),
		Description: "Fixed64 (Decimal)",
		Likelihood:  0.8,
	})

	// 3. Float64
	fValActual := math.Float64frombits(val)
	text := fmt.Sprintf("%g", fValActual)
	text = strings.Replace(text, "e+", "e", 1)
	if !strings.Contains(text, ".") && !strings.Contains(text, "e") && !math.IsNaN(fValActual) && !math.IsInf(fValActual, 0) {
		text = fmt.Sprintf("%.1f", fValActual)
	}
	var likelihood float64
	switch {
	case math.IsNaN(fValActual) || math.IsInf(fValActual, 0):
		likelihood = 0.2
	case fValActual == 0.0 || (math.Abs(fValActual) > 1e-6 && math.Abs(fValActual) < 1e6):
		likelihood = 0.7
	default:
		likelihood = 0.5
	}
	reps = append(reps, Representation{
		Type:        "float64",
		Text:        text,
		Description: "Float64",
		Likelihood:  likelihood,
	})

	return reps
}

func possibilitiesLen(payload []byte) []Representation {
	var reps []Representation

	// 1. Fallback Hex Bytes (always valid)
	reps = append(reps, Representation{
		Type:        "bytes",
		Text:        fmt.Sprintf("{`%s`}", toHexSpace(payload)),
		Description: "Bytes",
		Likelihood:  0.1,
	})

	// 2. String
	if utf8.Valid(payload) {
		isMsg := isMessage(payload)
		likelihood := 0.4
		if isPrintable(payload) {
			if !isMsg {
				likelihood = 0.9
			} else {
				likelihood = 0.6
			}
		}
		reps = append(reps, Representation{
			Type:        "string",
			Text:        fmt.Sprintf("{%q}", string(payload)),
			Description: "String",
			Likelihood:  likelihood,
		})
	}

	// 3. Message
	if isStructMessage(payload) {
		var buf bytes.Buffer
		if err := Disassemble(payload, &buf); err == nil {
			text := formatSingleLine(buf.String())
			likelihood := 0.6
			if isMessage(payload) {
				likelihood = 0.9
			}
			reps = append(reps, Representation{
				Type:        "message",
				Text:        text,
				Description: "Embedded Message",
				Likelihood:  likelihood,
			})
		}
	}

	// 4. Packed Varints
	if len(payload) > 0 {
		var ints []uint64
		off := 0
		ok := true
		for off < len(payload) {
			v, n := binary.Uvarint(payload[off:])
			if n <= 0 {
				ok = false
				break
			}
			off += n
			ints = append(ints, v)
		}
		if ok && len(ints) > 0 {
			var sb strings.Builder
			sb.WriteString("[")
			for _, val := range ints {
				fmt.Fprintf(&sb, " %d", val)
			}
			sb.WriteString(" ]")
			allSmall := true
			for _, val := range ints {
				if val > 1000 {
					allSmall = false
					break
				}
			}
			likelihood := 0.3
			if allSmall {
				likelihood = 0.5
			}
			reps = append(reps, Representation{
				Type:        "packed_varint",
				Text:        sb.String(),
				Description: "Packed Varints",
				Likelihood:  likelihood,
			})
		}
	}

	// 5. Packed Fixed32
	if len(payload) > 0 && len(payload)%4 == 0 {
		var sb strings.Builder
		sb.WriteString("[")
		for i := 0; i < len(payload); i += 4 {
			v := binary.LittleEndian.Uint32(payload[i:])
			fmt.Fprintf(&sb, " 0x%08xi32", v)
		}
		sb.WriteString(" ]")
		reps = append(reps, Representation{
			Type:        "packed_fixed32",
			Text:        sb.String(),
			Description: "Packed Fixed32",
			Likelihood:  0.4,
		})
	}

	// 6. Packed Fixed64
	if len(payload) > 0 && len(payload)%8 == 0 {
		var sb strings.Builder
		sb.WriteString("[")
		for i := 0; i < len(payload); i += 8 {
			v := binary.LittleEndian.Uint64(payload[i:])
			fmt.Fprintf(&sb, " 0x%016xi64", v)
		}
		sb.WriteString(" ]")
		reps = append(reps, Representation{
			Type:        "packed_fixed64",
			Text:        sb.String(),
			Description: "Packed Fixed64",
			Likelihood:  0.4,
		})
	}

	return reps
}

func isStructMessage(data []byte) bool {
	ok, fields := checkMessageStructure(data)
	return ok && fields > 0
}

func formatSingleLine(text string) string {
	text = strings.TrimSpace(text)
	lines := strings.Split(text, "\n")
	var cleaned []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			cleaned = append(cleaned, line)
		}
	}
	return "{ " + strings.Join(cleaned, " ") + " }"
}
