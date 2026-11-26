package cli

import (
	"io"
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/term"
	"github.com/muesli/termenv"
)

// terminal is what arr knows about a terminal it writes to.
type terminal struct {
	width   int
	profile termenv.Profile // the colours it can show
}

// detectTerminal reports whether w is a terminal, and if so what kind. Tests
// replace it.
var detectTerminal = func(w io.Writer) (terminal, bool) {
	if !isTerminal(w) {
		return terminal{}, false
	}
	width, _, err := term.GetSize(w.(*os.File).Fd())
	if err != nil {
		return terminal{}, false
	}
	return terminal{width: width, profile: lipgloss.NewRenderer(w).ColorProfile()}, true
}

// isTerminal reports whether v is a file open on a terminal.
func isTerminal(v any) bool {
	f, ok := v.(*os.File)
	return ok && term.IsTerminal(f.Fd())
}

// newRenderer returns a Lip Gloss renderer for w that styles output only when
// w is a terminal, as detectTerminal found.
func newRenderer(w io.Writer, t terminal, isTerminal bool) *lipgloss.Renderer {
	r := lipgloss.NewRenderer(w)
	if isTerminal {
		r.SetColorProfile(t.profile)
	} else {
		r.SetColorProfile(termenv.Ascii)
	}
	return r
}
