package timex

import "time"

// Stopwatch allows for recording the elapsed time of some event, while allowing
// for pauses in timing.
//
// A zero Stopwatch is ready to use. Methods on Stopwatch must not be called
// concurrently.
type Stopwatch struct {
	elapsed time.Duration
	start   time.Time
}

// Time returns the time elapsed since the first call to [Stopwatch.Start].
func (s *Stopwatch) Elapsed() time.Duration {
	elapsed := s.elapsed
	if !s.start.IsZero() {
		elapsed += time.Since(s.start)
	}
	return elapsed
}

// Start sets the stopwatch running.
//
// Repeated calls to [Stopwatch.Start] are no-ops.
func (s *Stopwatch) Start() {
	if s.start.IsZero() {
		s.start = time.Now()
	}
}

// Stop stops the stopwatch, and reports the total runtime thus far.
//
// Repeated calls to [Stopwatch.Stop] are no-ops.
func (s *Stopwatch) Stop() time.Duration {
	if !s.start.IsZero() {
		s.elapsed += time.Since(s.start)
	}
	s.start = time.Time{}
	return s.elapsed
}

// Reset resets the elapsed time to zero.
func (s *Stopwatch) Reset() {
	s.elapsed = 0
	if !s.start.IsZero() {
		s.start = time.Now()
	}
}
