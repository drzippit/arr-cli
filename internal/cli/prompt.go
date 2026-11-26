package cli

import (
	"errors"
	"io"

	"github.com/charmbracelet/huh"
)

// chooser asks the person at the terminal to pick one of options and returns
// its index.
type chooser func(title string, options []string) (int, error)

// newChooser returns a chooser that asks through in and out, or nil when
// either is not a terminal, as when arr runs from a script or its output
// goes elsewhere. Tests replace it.
var newChooser = func(in io.Reader, out io.Writer) chooser {
	if !isTerminal(in) || !isTerminal(out) {
		return nil
	}
	return func(title string, options []string) (int, error) {
		return huhChoose(in, out, title, options)
	}
}

// errCancelled means the person at the terminal backed out of a prompt.
var errCancelled = errors.New("cancelled")

func huhChoose(in io.Reader, out io.Writer, title string, options []string) (int, error) {
	opts := make([]huh.Option[int], len(options))
	for i, o := range options {
		opts[i] = huh.NewOption(o, i)
	}
	var picked int
	sel := huh.NewSelect[int]().Title(title).Options(opts...).Value(&picked)
	err := huh.NewForm(huh.NewGroup(sel)).WithInput(in).WithOutput(out).Run()
	if errors.Is(err, huh.ErrUserAborted) {
		return 0, errCancelled
	}
	return picked, err
}
