package cli

import (
	"testing"
	"time"
)

// SetNow makes the CLI see fn as the current time, and fn's location as the
// local time zone, until t ends.
func SetNow(t testing.TB, fn func() time.Time) {
	prev := now
	now = fn
	t.Cleanup(func() { now = prev })
}
