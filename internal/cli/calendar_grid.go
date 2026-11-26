package cli

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/drzippit/arr-cli/internal/config"
)

const (
	// minCellWidth is the narrowest a day cell can be and still be useful;
	// a terminal narrower than seven of them gets the list instead.
	minCellWidth = 12
	// maxCellEntries is how many lines of entries a day cell holds before
	// the last becomes "+N more".
	maxCellEntries = 4
)

// gridFits reports whether a month grid fits a terminal width columns wide.
func gridFits(width int) bool { return gridCellWidth(width) >= minCellWidth }

// gridCellWidth is the width of each day cell between borders: seven cells
// and eight borders fill the terminal.
func gridCellWidth(width int) int { return (width - 8) / 7 }

// appColours tell Sonarr and Radarr entries apart in the grid.
var appColours = map[config.App]lipgloss.Color{
	config.Sonarr: lipgloss.Color("#35c5f4"),
	config.Radarr: lipgloss.Color("#ffc230"),
}

// renderGrid draws month as a grid of weeks, Monday first, width columns
// wide, with today highlighted and labelled.
func renderGrid(w io.Writer, r *lipgloss.Renderer, width int, month calendarMonth, today time.Time, recs []calendarRecord) error {
	cellW := gridCellWidth(width)
	border := r.NewStyle().Foreground(lipgloss.Color("8"))
	bold := r.NewStyle().Bold(true)
	todayStyle := r.NewStyle().Bold(true).Reverse(true)

	byDay := map[int][]calendarRecord{}
	for _, rec := range recs {
		byDay[rec.day.Day()] = append(byDay[rec.day.Day()], rec)
	}

	// cell pads s, already styled, to the cell width after a space.
	cell := func(s string) string {
		return " " + s + strings.Repeat(" ", max(0, cellW-1-lipgloss.Width(s)))
	}
	rule := func(left, mid, right string) string {
		parts := make([]string, 7)
		for i := range parts {
			parts[i] = strings.Repeat("─", cellW)
		}
		return border.Render(left+strings.Join(parts, mid)+right) + "\n"
	}
	row := func(cells []string) string {
		bar := border.Render("│")
		return bar + strings.Join(cells, bar) + bar + "\n"
	}

	var b strings.Builder
	b.WriteString(bold.Render(month.String()) + "\n")
	b.WriteString(rule("╭", "┬", "╮"))
	heads := make([]string, 7)
	for i, name := range []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"} {
		heads[i] = cell(bold.Render(name))
	}
	b.WriteString(row(heads))

	// Go counts weekdays from Sunday; the grid counts from Monday.
	offset := (int(month.first.Weekday()) + 6) % 7
	days := month.next.AddDate(0, 0, -1).Day()
	for start := 1 - offset; start <= days; start += 7 {
		b.WriteString(rule("├", "┼", "┤"))
		var lines [7][]string
		height := 2
		for col := range 7 {
			day := start + col
			if day < 1 || day > days {
				continue
			}
			num := bold.Render(fmt.Sprint(day))
			// Today is labelled as well as styled, so it stands out
			// without colour too (NO_COLOR).
			if month.contains(today) && day == today.Day() {
				num = todayStyle.Render(fmt.Sprint(day)) + " today"
			}
			lines[col] = append([]string{num}, cellEntries(r, byDay[day], cellW-1)...)
			height = max(height, len(lines[col]))
		}
		for i := range height {
			cells := make([]string, 7)
			for col := range 7 {
				s := ""
				if i < len(lines[col]) {
					s = lines[col][i]
				}
				cells[col] = cell(s)
			}
			b.WriteString(row(cells))
		}
	}
	b.WriteString(rule("╰", "┴", "╯"))
	_, err := io.WriteString(w, b.String())
	return err
}

// cellEntries is one styled line per record, each at most width columns,
// with the last line "+N more" when they don't all fit.
func cellEntries(r *lipgloss.Renderer, recs []calendarRecord, width int) []string {
	shown := recs
	if len(recs) > maxCellEntries {
		shown = recs[:maxCellEntries-1]
	}
	var lines []string
	for _, rec := range shown {
		style := r.NewStyle().Foreground(appColours[rec.App])
		if !rec.Monitored {
			style = style.Faint(true)
		}
		lines = append(lines, style.Render(truncate(rec.displayTitle(), width)))
	}
	if more := len(recs) - len(shown); more > 0 {
		lines = append(lines, r.NewStyle().Faint(true).Render(truncate(fmt.Sprintf("+%d more", more), width)))
	}
	return lines
}

// truncate shortens s to at most width columns, ending it with "…" if it
// was cut.
func truncate(s string, width int) string {
	if ansi.StringWidth(s) <= width {
		return s
	}
	return strings.TrimRight(ansi.Truncate(s, width-1, ""), " ") + "…"
}
