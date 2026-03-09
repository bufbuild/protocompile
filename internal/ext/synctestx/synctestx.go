package synctestx

import (
	"runtime"
	"sync"
)

// Hammer runs f across count goroutines, ensuring that f is called
// simultaneously, simulating a thundering herd. Returns once all spawned
// goroutines have exited.
//
// If count is zero, uses GOMAXPROCS instead.
func Hammer(count int, f func()) {
	if count == 0 {
		count = runtime.GOMAXPROCS(0)
	}

	start := new(sync.WaitGroup)
	end := new(sync.WaitGroup)
	for range count {
		start.Add(1)
		end.Add(1)
		go func() {
			defer end.Done()

			// This ensures that we have a thundering herd situation: all of
			// these goroutines wake up and hammer f() at the same time.
			start.Done()
			start.Wait()

			f()
		}()
	}

	end.Wait()
}
