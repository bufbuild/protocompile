package disassembler

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDisassembleHeuristicComments(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected []string // substrings we expect to find
	}{
		{
			name: "Likely Float64",
			data: func() []byte {
				return []byte{
					// Tag 1, I64
					0x09,
					// 0x405edd2f1a9fbe77 -> 123.456
					0x77, 0xbe, 0x9f, 0x1a, 0x2f, 0xdd, 0x5e, 0x40,
				}
			}(),
			expected: []string{
				"1: 123.456                    # 0x405edd2f1a9fbe77i64",
			},
		},
		{
			name: "Likely Float32",
			data: func() []byte {
				return []byte{
					// Tag 2, I32
					0x15,
					// 78.9f32 -> 0x429dcccd
					0xcd, 0xcc, 0x9d, 0x42,
				}
			}(),
			expected: []string{
				"2: 78.9i32                    # 0x429dcccdi32",
			},
		},
		{
			name: "Ambiguous I64 (Text)",
			data: func() []byte {
				return []byte{
					// Tag 12, I64
					0x61,
					// "ram@nibl" -> 72 61 6d 40 6e 69 62 6c
					0x72, 0x61, 0x6d, 0x40, 0x6e, 0x69, 0x62, 0x6c,
				}
			}(),
			expected: []string{
				"12: 1.239664294489405e214     # 0x6c62696e406d6172i64",
			},
		},
		{
			name: "Ambiguous I32 (Text)",
			data: func() []byte {
				return []byte{
					// Tag 12, I32
					0x65,
					// "ting" -> 74 69 6e 67
					0x67, 0x6e, 0x69, 0x74,
				}
			}(),
			expected: []string{
				"12: 7.397732e31i32            # 0x74696e67i32",
			},
		},
		{
			name: "NaN Float64",
			data: func() []byte {
				return []byte{
					// Tag 1, I64
					0x09,
					// NaN -> 0xffffffffffffffa8
					0xa8, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
				}
			}(),
			expected: []string{
				"1: 0xffffffffffffffa8i64      # NaN",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := DisassembleWithOptions(tt.data, &buf, Options{})
			require.NoError(t, err)
			output := buf.String()
			for _, exp := range tt.expected {
				assert.Contains(t, output, exp)
			}
		})
	}
}
