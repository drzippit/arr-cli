package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
)

// renderTable writes a Lip Gloss table. Styling is dropped automatically when
// w is not a terminal.
func renderTable(w io.Writer, headers []string, rows [][]string) error {
	r := lipgloss.NewRenderer(w)
	header := r.NewStyle().Bold(true).Padding(0, 1)
	cell := r.NewStyle().Padding(0, 1)
	t := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(r.NewStyle().Foreground(lipgloss.Color("8"))).
		Headers(headers...).
		Rows(rows...).
		StyleFunc(func(row, _ int) lipgloss.Style {
			if row == table.HeaderRow {
				return header
			}
			return cell
		})
	_, err := fmt.Fprintln(w, t.Render())
	return err
}

// renderJSON writes records as an indented JSON array.
func renderJSON[T any](w io.Writer, records []T) error {
	if records == nil {
		records = []T{}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(records)
}
