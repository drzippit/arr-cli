package cli

import (
	"errors"
	"io"

	"github.com/charmbracelet/huh"
)

// chooser asks the person at the terminal to pick one of options, with the
// one at index selected picked to start with, and returns its index.
type chooser func(title string, options []string, selected int) (int, error)

// newChooser returns a chooser that asks through in and out, or nil when
// either is not a terminal, as when arr runs from a script or its output
// goes elsewhere. Tests replace it.
var newChooser = func(in io.Reader, out io.Writer) chooser {
	if !isTerminal(in) || !isTerminal(out) {
		return nil
	}
	return func(title string, options []string, selected int) (int, error) {
		return huhChoose(in, out, title, options, selected)
	}
}

// errCancelled means the person at the terminal backed out of a prompt.
var errCancelled = errors.New("cancelled")

func huhChoose(in io.Reader, out io.Writer, title string, options []string, selected int) (int, error) {
	opts := make([]huh.Option[int], len(options))
	for i, o := range options {
		opts[i] = huh.NewOption(o, i)
	}
	picked := selected
	sel := huh.NewSelect[int]().Title(title).Options(opts...).Value(&picked)
	err := huh.NewForm(huh.NewGroup(sel)).WithInput(in).WithOutput(out).Run()
	if errors.Is(err, huh.ErrUserAborted) {
		return 0, errCancelled
	}
	return picked, err
}

// confirmer asks the person at the terminal a yes/no question.
type confirmer func(question string) (bool, error)

// newConfirmer returns a confirmer that asks through in and out, or nil when
// either is not a terminal. Tests replace it.
var newConfirmer = func(in io.Reader, out io.Writer) confirmer {
	if !isTerminal(in) || !isTerminal(out) {
		return nil
	}
	return func(question string) (bool, error) {
		var yes bool
		c := huh.NewConfirm().Title(question).Affirmative("Yes").Negative("No").Value(&yes)
		err := huh.NewForm(huh.NewGroup(c)).WithInput(in).WithOutput(out).Run()
		if errors.Is(err, huh.ErrUserAborted) {
			return false, errCancelled
		}
		return yes, err
	}
}

// picker lets the person at the terminal untick some of options, all ticked
// to start with, and returns the indexes left ticked.
type picker func(title string, options []string) ([]int, error)

// newPicker returns a picker that asks through in and out, or nil when
// either is not a terminal. Tests replace it.
var newPicker = func(in io.Reader, out io.Writer) picker {
	if !isTerminal(in) || !isTerminal(out) {
		return nil
	}
	return func(title string, options []string) ([]int, error) {
		opts := make([]huh.Option[int], len(options))
		picked := make([]int, len(options))
		for i, o := range options {
			opts[i] = huh.NewOption(o, i).Selected(true)
			picked[i] = i
		}
		ms := huh.NewMultiSelect[int]().Title(title).Options(opts...).Value(&picked)
		err := huh.NewForm(huh.NewGroup(ms)).WithInput(in).WithOutput(out).Run()
		if errors.Is(err, huh.ErrUserAborted) {
			return nil, errCancelled
		}
		return picked, err
	}
}
