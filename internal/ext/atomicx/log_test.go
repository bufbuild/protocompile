package atomicx_test

import (
	"runtime"
	"sync"
	"testing"

	"github.com/bufbuild/protocompile/internal/ext/atomicx"
	"github.com/stretchr/testify/assert"
)

func TestLog(t *testing.T) {
	t.Parallel()

	const trials = 1000

	mu := new(sync.Mutex)
	log := new(atomicx.Log[int])

	start := new(sync.WaitGroup)
	end := new(sync.WaitGroup)

	for i := range runtime.GOMAXPROCS(0) {
		start.Add(1)
		end.Add(1)
		go func() {
			defer end.Done()

			// This ensures that we have a thundering herd situation: all of
			// these goroutines wake up and hammer the intern table at the
			// same time.
			start.Done()
			start.Wait()

			for j := range trials {
				n := i*trials + j
				mu.Lock()
				i := log.Append(n) - 1
				mu.Unlock()
				assert.Equal(t, n, log.Load(i))
			}
		}()
	}

	end.Wait()
}
