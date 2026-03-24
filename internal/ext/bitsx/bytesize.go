package bitsx

import (
	"fmt"
	"math/bits"
)

// ByteSize formats a number as a human-readable number of bytes.
func ByteSize[T Int](v T) string {
	abs := v
	if v < 0 {
		abs = -v
	}

	n := bits.Len64(uint64(abs))
	if n >= 30 {
		return fmt.Sprintf("%.03f GB", float64(v)/float64(1024*1024*1024))
	}
	if n >= 20 {
		return fmt.Sprintf("%.03f MB", float64(v)/float64(1024*1024))
	}
	if n >= 10 {
		return fmt.Sprintf("%.03f KB", float64(v)/float64(1024))
	}
	return fmt.Sprintf("%v.000 B", v)
}
