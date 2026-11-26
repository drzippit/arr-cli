package cli

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/drzippit/arr-cli/internal/arrapi"
	"github.com/drzippit/arr-cli/internal/config"
)

// now is the current time; its location is the local time zone that calendar
// days and months are counted in. Tests replace it.
var now = time.Now

// calendarRecord is one episode airing, or one Movie release, on one
// Instance. Airs, Season, Episode and EpisodeTitle are set for Sonarr; Year
// and ReleaseKind for Radarr.
type calendarRecord struct {
	Instance     string     `json:"instance"`
	App          config.App `json:"app"`
	Date         string     `json:"date"`
	Airs         *time.Time `json:"airs,omitempty"`
	Title        string     `json:"title"`
	Year         int        `json:"year,omitempty"`
	Season       *int       `json:"season,omitempty"`
	Episode      *int       `json:"episode,omitempty"`
	EpisodeTitle string     `json:"episodeTitle,omitempty"`
	ReleaseKind  string     `json:"release,omitempty"`
	HasFile      bool       `json:"hasFile"`

	day time.Time // local midnight on Date
}

// releaseKinds are the kinds of Movie release, in the order Radarr gives
// their dates, with how the list describes each.
var releaseKinds = []struct {
	kind, label string
	date        func(arrapi.CalendarMovie) *time.Time
}{
	{"inCinemas", "in cinemas", func(m arrapi.CalendarMovie) *time.Time { return m.InCinemas }},
	{"digital", "digital release", func(m arrapi.CalendarMovie) *time.Time { return m.DigitalRelease }},
	{"physical", "physical release", func(m arrapi.CalendarMovie) *time.Time { return m.PhysicalRelease }},
}

// newCalendarCmd builds a calendar command for the Instances that instances
// returns: one for `arr <instance> calendar`, or all of them for
// `arr calendar`.
func newCalendarCmd(instances func() ([]config.Instance, error), flags *globalFlags) *cobra.Command {
	var month string
	var list bool
	cmd := &cobra.Command{
		Use:   "calendar",
		Short: "Show the episodes and Movie releases due in one calendar month",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			first, err := parseMonth(month, now())
			if err != nil {
				return err
			}
			insts, err := instances()
			if err != nil {
				return err
			}
			fetch := func(ctx context.Context, inst config.Instance) instanceResult[calendarRecord] {
				return fetchCalendar(ctx, inst, first)
			}
			recs, failures := gather(insts, forEachInstance(cmd.Context(), insts, fetch))
			sortCalendar(recs)
			if err := renderCalendar(cmd.OutOrStdout(), first, recs, flags.json); err != nil {
				return err
			}
			return reportFailures(cmd.ErrOrStderr(), failures)
		},
	}
	cmd.Flags().StringVar(&month, "month", "", "month to show: YYYY-MM, next or prev (default: this month)")
	// The list is the only view until the month grid lands; then the grid
	// becomes the default and --list chooses this view.
	cmd.Flags().BoolVar(&list, "list", false, "show a list grouped by date")
	return cmd
}

// parseMonth returns midnight on the first day of the month that arg names,
// in now's location.
func parseMonth(arg string, today time.Time) (time.Time, error) {
	this := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, today.Location())
	switch arg {
	case "":
		return this, nil
	case "next":
		return this.AddDate(0, 1, 0), nil
	case "prev":
		return this.AddDate(0, -1, 0), nil
	}
	m, err := time.ParseInLocation("2006-01", arg, today.Location())
	if err != nil {
		return time.Time{}, fmt.Errorf("--month %q: want YYYY-MM, next or prev", arg)
	}
	return m, nil
}

// calendarMonth is one local calendar month.
type calendarMonth struct{ first, next time.Time }

func (m calendarMonth) contains(t time.Time) bool { return !t.Before(m.first) && t.Before(m.next) }

func fetchCalendar(ctx context.Context, inst config.Instance, first time.Time) instanceResult[calendarRecord] {
	key, err := inst.ResolveAPIKey()
	if err != nil {
		return instanceResult[calendarRecord]{err: err}
	}
	client := arrapi.New(inst.URL, key)
	month := calendarMonth{first, first.AddDate(0, 1, 0)}
	// Ask for a day either side: the Apps compare in UTC, and Movie release
	// days are midnight UTC, so the local month's edges can fall outside an
	// exact range. The records are trimmed to the month afterwards.
	start, end := month.first.AddDate(0, 0, -1), month.next.AddDate(0, 0, 1)

	var recs []calendarRecord
	switch inst.App {
	case config.Sonarr:
		eps, err := client.EpisodeCalendar(ctx, start, end)
		if err != nil {
			return instanceResult[calendarRecord]{err: err}
		}
		recs = episodeRecords(inst, month, eps)
	case config.Radarr:
		movies, err := client.MovieCalendar(ctx, start, end)
		if err != nil {
			return instanceResult[calendarRecord]{err: err}
		}
		recs = movieRecords(inst, month, movies)
	}
	return instanceResult[calendarRecord]{recs: recs}
}

// episodeRecords keeps the episodes that air in month, local time.
func episodeRecords(inst config.Instance, month calendarMonth, eps []arrapi.CalendarEpisode) []calendarRecord {
	var recs []calendarRecord
	for _, ep := range eps {
		airs := ep.AirDateUTC.In(month.first.Location())
		if !month.contains(airs) {
			continue
		}
		r := calendarRecord{
			Instance:     inst.Name,
			App:          inst.App,
			Date:         airs.Format(time.DateOnly),
			Airs:         &airs,
			Season:       &ep.SeasonNumber,
			Episode:      &ep.EpisodeNumber,
			EpisodeTitle: ep.Title,
			HasFile:      ep.HasFile,
			day:          time.Date(airs.Year(), airs.Month(), airs.Day(), 0, 0, 0, 0, airs.Location()),
		}
		if ep.Series != nil {
			r.Title = ep.Series.Title
		}
		recs = append(recs, r)
	}
	return recs
}

// movieRecords makes one record per Movie release that falls in month.
func movieRecords(inst config.Instance, month calendarMonth, movies []arrapi.CalendarMovie) []calendarRecord {
	var recs []calendarRecord
	for _, m := range movies {
		for _, rk := range releaseKinds {
			at := rk.date(m)
			if at == nil {
				continue
			}
			// A release date is a calendar day, not an instant, so it keeps
			// its day whatever the local time zone.
			u := at.UTC()
			day := time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, month.first.Location())
			if !month.contains(day) {
				continue
			}
			recs = append(recs, calendarRecord{
				Instance:    inst.Name,
				App:         inst.App,
				Date:        day.Format(time.DateOnly),
				Title:       m.Title,
				Year:        m.Year,
				ReleaseKind: rk.kind,
				HasFile:     m.HasFile,
				day:         day,
			})
		}
	}
	return recs
}

// sortCalendar orders records by day, then Movie releases (which have no
// time) before episodes in airing order. Records that tie keep their order,
// which is config order.
func sortCalendar(recs []calendarRecord) {
	slices.SortStableFunc(recs, func(a, b calendarRecord) int {
		if c := cmp.Compare(a.Date, b.Date); c != 0 {
			return c
		}
		switch {
		case a.Airs == nil && b.Airs == nil:
			return 0
		case a.Airs == nil:
			return -1
		case b.Airs == nil:
			return 1
		}
		return a.Airs.Compare(*b.Airs)
	})
}

func renderCalendar(w io.Writer, first time.Time, recs []calendarRecord, asJSON bool) error {
	if asJSON {
		return renderJSON(w, recs)
	}
	r := lipgloss.NewRenderer(w)
	bold := r.NewStyle().Bold(true)
	dim := r.NewStyle().Faint(true)

	monthName := first.Format("January 2006")
	if len(recs) == 0 {
		_, err := fmt.Fprintf(w, "Nothing due in %s.\n", monthName)
		return err
	}

	rows := make([][3]string, len(recs))
	var widths [3]int
	for i, rec := range recs {
		rows[i] = [3]string{rec.displayTime(), rec.Instance, rec.displayTitle()}
		for j, cell := range rows[i] {
			widths[j] = max(widths[j], lipgloss.Width(cell))
		}
	}
	pad := func(s string, width int) string { return s + strings.Repeat(" ", width-lipgloss.Width(s)) }

	var b strings.Builder
	b.WriteString(bold.Render(monthName) + "\n")
	for i, rec := range recs {
		if i == 0 || rec.Date != recs[i-1].Date {
			b.WriteString("\n" + bold.Render(rec.day.Format("Mon 2 Jan")) + "\n")
		}
		row := rows[i]
		fmt.Fprintf(&b, "  %s  %s  %s  %s\n",
			dim.Render(pad(row[0], widths[0])), pad(row[1], widths[1]), pad(row[2], widths[2]),
			dim.Render(rec.displayDetail()))
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func (r calendarRecord) displayTime() string {
	if r.Airs == nil {
		return ""
	}
	return r.Airs.Format("15:04")
}

// displayTitle is "<Series> SxxEyy" for an episode and "<Movie>" for a Movie.
func (r calendarRecord) displayTitle() string {
	if r.Season == nil || r.Episode == nil {
		return r.Title
	}
	return fmt.Sprintf("%s S%02dE%02d", r.Title, *r.Season, *r.Episode)
}

// displayDetail is the episode's title, or the kind of Movie release.
func (r calendarRecord) displayDetail() string {
	for _, rk := range releaseKinds {
		if rk.kind == r.ReleaseKind {
			return rk.label
		}
	}
	return r.EpisodeTitle
}
