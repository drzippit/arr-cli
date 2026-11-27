package cli

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/muesli/termenv"

	"github.com/drzippit/arr-cli/internal/config"
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
// whenever it would show a picker, until t ends. selected is the option the
// picker starts on.
func SetChooser(t testing.TB, choose func(title string, options []string, selected int) (int, error)) {
	prev := newChooser
	newChooser = func(io.Reader, io.Writer) chooser { return choose }
	t.Cleanup(func() { newChooser = prev })
}

// SetInstanceForm makes the CLI act as if it runs in a terminal, calling
// form whenever it would ask for a new Instance, until t ends.
func SetInstanceForm(t testing.TB, form func(checkName func(string) error) (config.Instance, error)) {
	prev := newInstanceForm
	newInstanceForm = func(io.Reader, io.Writer) instanceForm { return form }
	t.Cleanup(func() { newInstanceForm = prev })
}

// SetWatchFrames makes queue --watch draw frames frames, one straight after
// another, then stop as if Ctrl-C had been pressed, until t ends.
func SetWatchFrames(t testing.TB, frames int) {
	prevFrames, prevWait := watchFrames, watchWait
	watchFrames = frames
	watchWait = func(context.Context, time.Duration) bool { return true }
	t.Cleanup(func() { watchFrames, watchWait = prevFrames, prevWait })
}

// SetConfirm makes the CLI act as if it runs in a terminal, calling confirm
// whenever it would ask a yes/no question, until t ends.
func SetConfirm(t testing.TB, confirm func(question string) (bool, error)) {
	prev := newConfirmer
	newConfirmer = func(io.Reader, io.Writer) confirmer { return confirm }
	t.Cleanup(func() { newConfirmer = prev })
}

// RunWithStdin is Run with stdin as standard input.
func RunWithStdin(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	return run(args, stdin, stdout, stderr)
}
