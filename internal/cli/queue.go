package cli

import (
	"context"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/drzippit/arr-cli/internal/arrapi"
	"github.com/drzippit/arr-cli/internal/config"
)

// queueRecord is one download in progress on one Instance. Season, Episode and
// EpisodeTitle are set for Sonarr, Year for Radarr.
type queueRecord struct {
	Instance            string     `json:"instance"`
	App                 config.App `json:"app"`
	Title               string     `json:"title"`
	Year                int        `json:"year,omitempty"`
	Season              *int       `json:"season,omitempty"`
	Episode             *int       `json:"episode,omitempty"`
	EpisodeTitle        string     `json:"episodeTitle,omitempty"`
	Release             string     `json:"release"`
	Progress            float64    `json:"progress"`
	Size                int64      `json:"size"`
	SizeLeft            int64      `json:"sizeLeft"`
	TimeLeft            string     `json:"timeLeft,omitempty"`
	EstimatedCompletion string     `json:"estimatedCompletionTime,omitempty"`
	Status              string     `json:"status"`
	DownloadClient      string     `json:"downloadClient"`
	Protocol            string     `json:"protocol"`
}

// newQueueCmd builds a queue command for the Instances that instances returns:
// one for `arr <instance> queue`, or all of them for `arr queue`. It only
// displays the queue; it has no remove, blocklist or retry actions.
func newQueueCmd(instances func() ([]config.Instance, error), flags *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "queue",
		Short: "Show downloads in progress on each Instance",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			insts, err := instances()
			if err != nil {
				return err
			}
			recs, failures := gather(insts, forEachInstance(cmd.Context(), insts, fetchQueue))
			if err := renderQueue(cmd.OutOrStdout(), recs, flags.json); err != nil {
				return err
			}
			return reportFailures(cmd.ErrOrStderr(), failures)
		},
	}
}

func fetchQueue(ctx context.Context, inst config.Instance) instanceResult[queueRecord] {
	key, err := inst.ResolveAPIKey()
	if err != nil {
		return instanceResult[queueRecord]{err: err}
	}
	items, err := arrapi.New(inst.URL, key).Queue(ctx)
	if err != nil {
		return instanceResult[queueRecord]{err: err}
	}
	recs := make([]queueRecord, len(items))
	for i, it := range items {
		recs[i] = newQueueRecord(inst, it)
	}
	return instanceResult[queueRecord]{recs: recs}
}

func newQueueRecord(inst config.Instance, it arrapi.QueueItem) queueRecord {
	r := queueRecord{
		Instance:            inst.Name,
		App:                 inst.App,
		Title:               it.Release,
		Release:             it.Release,
		Size:                int64(it.Size),
		SizeLeft:            int64(it.SizeLeft),
		TimeLeft:            it.TimeLeft,
		EstimatedCompletion: it.EstimatedCompletionTime,
		Status:              queueStatus(it),
		DownloadClient:      it.DownloadClient,
		Protocol:            it.Protocol,
	}
	if it.Size > 0 {
		r.Progress = math.Round((it.Size-it.SizeLeft)/it.Size*1000) / 10
	}
	switch {
	case it.Series != nil:
		r.Title = it.Series.Title
	case it.Movie != nil:
		r.Title = it.Movie.Title
		r.Year = it.Movie.Year
	}
	if it.Episode != nil {
		r.Season = &it.Episode.SeasonNumber
		r.Episode = &it.Episode.EpisodeNumber
		r.EpisodeTitle = it.Episode.Title
	}
	return r
}

// queueStatus is the download's status, except that a finished download the
// App has not imported shows why (such as importBlocked), since that is the
// one a user needs to act on.
func queueStatus(it arrapi.QueueItem) string {
	if it.Status == "completed" && it.TrackedDownloadState != "" && it.TrackedDownloadState != "imported" {
		return it.TrackedDownloadState
	}
	return it.Status
}

func renderQueue(w io.Writer, recs []queueRecord, asJSON bool) error {
	if asJSON {
		return renderJSON(w, recs)
	}
	rows := make([][]string, 0, len(recs))
	for _, r := range recs {
		rows = append(rows, []string{
			r.Instance, r.displayTitle(), r.displayEpisode(), fmt.Sprintf("%.0f%%", math.Floor(r.Progress)),
			formatTimeLeft(r.TimeLeft), orDash(r.Status), orDash(r.DownloadClient),
		})
	}
	return renderTable(w, []string{"INSTANCE", "TITLE", "EPISODE", "PROGRESS", "ETA", "STATUS", "CLIENT"}, rows)
}

func (r queueRecord) displayTitle() string {
	if r.Year > 0 {
		return fmt.Sprintf("%s (%d)", r.Title, r.Year)
	}
	return r.Title
}

func (r queueRecord) displayEpisode() string {
	if r.Season == nil || r.Episode == nil {
		return "-"
	}
	s := fmt.Sprintf("S%02dE%02d", *r.Season, *r.Episode)
	if r.EpisodeTitle != "" {
		s += " " + r.EpisodeTitle
	}
	return s
}

// formatTimeLeft turns the App's .NET TimeSpan ("1.02:03:04", "00:12:34" or
// "00:00:05.1234567") into "1d 2h 3m", "12m 34s" or "5s". It returns the input
// unchanged if it does not parse, and "-" if it is empty.
func formatTimeLeft(span string) string {
	if span == "" {
		return "-"
	}
	var days int
	clock := span
	if d, rest, ok := strings.Cut(span, "."); ok && strings.Contains(rest, ":") {
		n, err := strconv.Atoi(d)
		if err != nil {
			return span
		}
		days, clock = n, rest
	}
	clock, _, _ = strings.Cut(clock, ".")
	var h, m, sec int
	if _, err := fmt.Sscanf(clock, "%d:%d:%d", &h, &m, &sec); err != nil {
		return span
	}
	parts := []struct {
		n    int
		unit string
	}{{days, "d"}, {h, "h"}, {m, "m"}, {sec, "s"}}
	var out []string
	for _, p := range parts {
		if p.n > 0 || len(out) > 0 {
			out = append(out, fmt.Sprintf("%d%s", p.n, p.unit))
		}
		if len(out) == 3 {
			break
		}
	}
	if len(out) == 0 {
		return "0s"
	}
	return strings.Join(out, " ")
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
