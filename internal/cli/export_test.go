package cli

import (
	"io"
	"testing"
	"time"

	"github.com/muesli/termenv"
)

// SetNow makes the CLI see fn as the current time, and fn's location as the
// local time zone, until t ends.
func SetNow(t testing.TB, fn func() time.Time) {
	prev := now
	now = fn
	t.Cleanup(func() { now = prev })
}

// SetTerminal makes the CLI treat its output as a colour terminal width
// columns wide until t ends.
func SetTerminal(t testing.TB, width int) {
	prev := detectTerminal
	detectTerminal = func(io.Writer) (terminal, bool) {
		return terminal{width: width, profile: termenv.TrueColor}, true
	}
	t.Cleanup(func() { detectTerminal = prev })
}

// SetChooser makes the CLI act as if it runs in a terminal, asking choose
// whenever it would show a picker, until t ends.
func SetChooser(t testing.TB, choose func(title string, options []string) (int, error)) {
	prev := newChooser
	newChooser = func(io.Reader, io.Writer) chooser { return choose }
	t.Cleanup(func() { newChooser = prev })
}
