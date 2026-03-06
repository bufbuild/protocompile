package timex

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestStopwatch(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		sw := new(Stopwatch)
		assert.Equal(t, time.Duration(0), sw.Elapsed())

		sw.Start()
		time.Sleep(time.Second)
		assert.Equal(t, time.Second, sw.Elapsed())
		time.Sleep(time.Second)
		assert.Equal(t, 2*time.Second, sw.Stop())
		time.Sleep(time.Second)
		assert.Equal(t, 2*time.Second, sw.Elapsed())
		sw.Start()
		time.Sleep(time.Second)
		assert.Equal(t, 3*time.Second, sw.Elapsed())
		sw.Reset()
		time.Sleep(time.Second)
		assert.Equal(t, time.Second, sw.Elapsed())
	})
}
