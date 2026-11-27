package cli

import (
	"context"
	"fmt"
	"io"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/drzippit/arr-cli/internal/arrapi"
	"github.com/drzippit/arr-cli/internal/config"
	"github.com/drzippit/arr-cli/internal/sabapi"
)

// queueRecord is one Download in progress on one Instance. Title is the Series
// or Movie it is for, which only a Media App knows; a Download Client's own row
// goes by its Release. Season, Episode and EpisodeTitle are set for Sonarr,
// Year for Radarr. Priority, Category and
// Position (from 1, in queue order) are set for Download Clients; a Download
// being post-processed has no Position.
type queueRecord struct {
	Instance            string     `json:"instance"`
	App                 config.App `json:"app"`
	Title               string     `json:"title,omitempty"`
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
	DownloadID          string     `json:"downloadId,omitempty"`
	Priority            string     `json:"priority,omitempty"`
	Category            string     `json:"category,omitempty"`
	Position            *int       `json:"position,omitempty"`
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
	if inst.App.Kind() == config.DownloadClient {
		return fetchSabnzbdQueue(ctx, inst, sabapi.New(inst.URL, key))
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
		DownloadID:          it.DownloadID,
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

// postProcessingLookback is how many of the newest History entries are
// searched for Downloads still being post-processed. SABnzbd lists those
// first, so they are always among the newest.
const postProcessingLookback = 100

// fetchSabnzbdQueue gives SABnzbd's Queue: the slots in its queue, in order,
// then the Downloads in its history it is still post-processing.
func fetchSabnzbdQueue(ctx context.Context, inst config.Instance, client *sabapi.Client) instanceResult[queueRecord] {
	q, err := client.Queue(ctx)
	if err != nil {
		return instanceResult[queueRecord]{err: err}
	}
	history, err := client.History(ctx, postProcessingLookback, false)
	if err != nil {
		return instanceResult[queueRecord]{err: err}
	}
	base := queueRecord{Instance: inst.Name, App: inst.App, DownloadClient: inst.App.DisplayName(), Protocol: "usenet"}
	// download is a row for the Download with this name and ID.
	download := func(name, id string) queueRecord {
		r := base
		r.Release, r.DownloadID = name, id
		return r
	}
	var recs []queueRecord
	for _, d := range q.Slots {
		r := download(d.Filename, d.NzoID)
		r.Progress = float64(d.Percentage)
		r.Size, r.SizeLeft = d.Bytes(), d.BytesLeft()
		r.TimeLeft = sabnzbdTimeLeft(d.TimeLeft)
		r.Status = strings.ToLower(d.Status)
		r.Priority, r.Category = strings.ToLower(d.Priority), d.Category
		position := d.Index + 1
		r.Position = &position
		recs = append(recs, r)
	}
	for _, d := range history {
		if d.Finished() {
			continue
		}
		r := download(d.Name, d.NzoID)
		r.Progress, r.Size = 100, d.Bytes
		r.Status, r.Category = strings.ToLower(d.Status), d.Category
		recs = append(recs, r)
	}
	return instanceResult[queueRecord]{recs: recs}
}

// sabnzbdTimeLeft turns SABnzbd's h:mm:ss, or d:hh:mm:ss from a day on, into
// the .NET TimeSpan the Media Apps give ("00:25:00", "1.02:03:04"), so time
// left reads the same from every App. It returns "" for no time left and the
// input when it does not parse.
func sabnzbdTimeLeft(s string) string {
	parts := strings.Split(s, ":")
	n := make([]int, len(parts))
	for i, p := range parts {
		v, err := strconv.Atoi(p)
		if err != nil {
			return s
		}
		n[i] = v
	}
	switch len(n) {
	case 3:
		return fmt.Sprintf("%02d:%02d:%02d", n[0], n[1], n[2])
	case 4:
		return fmt.Sprintf("%d.%02d:%02d:%02d", n[0], n[1], n[2], n[3])
	}
	return s
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
	// A Download Client's size, priority and category get columns only when
	// some row has them, so a Media App's queue looks as it always has.
	withClient := slices.ContainsFunc(recs, queueRecord.hasDownloadClientValues)
	header := []string{"INSTANCE", "TITLE", "EPISODE", "PROGRESS"}
	if withClient {
		header = append(header, "SIZE", "LEFT")
	}
	header = append(header, "ETA", "STATUS")
	if withClient {
		header = append(header, "PRIORITY", "CATEGORY")
	}
	header = append(header, "CLIENT")
	rows := make([][]string, 0, len(recs))
	for _, r := range recs {
		row := []string{r.Instance, r.displayTitle(), r.displayEpisode(), fmt.Sprintf("%.0f%%", math.Floor(r.Progress))}
		if withClient {
			row = append(row, formatSize(r.Size), formatSize(r.SizeLeft))
		}
		row = append(row, formatTimeLeft(r.TimeLeft), orDash(r.Status))
		if withClient {
			row = append(row, orDash(r.Priority), orDash(r.Category))
		}
		rows = append(rows, append(row, orDash(r.DownloadClient)))
	}
	return renderTable(w, header, rows)
}

// hasDownloadClientValues reports whether r has values only a Download Client
// gives.
func (r queueRecord) hasDownloadClientValues() bool {
	return r.Priority != "" || r.Category != "" || r.Position != nil
}

// displayTitle is the Title the Download is for, or its Release when no Media
// App said, as for one added to a Download Client by hand.
func (r queueRecord) displayTitle() string {
	if r.Title == "" {
		return r.Release
	}
	return titleWithYear(r.Title, r.Year)
}

// titleWithYear writes a Title as "Good Will Hunting (1997)", or just its name
// when the year is unknown or the name already ends with it, as in Sonarr's
// "Twin Peaks (2017)".
func titleWithYear(title string, year int) string {
	if year <= 0 {
		return title
	}
	suffix := fmt.Sprintf(" (%d)", year)
	if strings.HasSuffix(title, suffix) {
		return title
	}
	return title + suffix
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
