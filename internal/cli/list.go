package cli

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/drzippit/arr-cli/internal/arrapi"
	"github.com/drzippit/arr-cli/internal/config"
)

// libraryRecord is one Title in an Instance's Library. TvdbID, Seasons,
// Episodes and EpisodeFiles are set for Sonarr; TmdbID and HasFile for Radarr.
// Episodes counts the monitored or downloaded episodes, as Sonarr does.
type libraryRecord struct {
	Instance     string     `json:"instance"`
	App          config.App `json:"app"`
	Title        string     `json:"title"`
	Year         int        `json:"year,omitempty"`
	Status       string     `json:"status"`
	Monitored    bool       `json:"monitored"`
	SizeOnDisk   int64      `json:"sizeOnDisk"`
	TvdbID       int        `json:"tvdbId,omitempty"`
	Seasons      *int       `json:"seasons,omitempty"`
	Episodes     *int       `json:"episodes,omitempty"`
	EpisodeFiles *int       `json:"episodeFiles,omitempty"`
	TmdbID       int        `json:"tmdbId,omitempty"`
	HasFile      *bool      `json:"hasFile,omitempty"`

	sortTitle string
}

// newListCmd builds a list command for the Instances that instances returns:
// one for `arr <instance> list`, or all of them for `arr list`. A query of
// several words may be quoted or not.
func newListCmd(instances func() ([]config.Instance, error), flags *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "list [query]",
		Short: "Show the Titles in each Instance's Library, optionally only those matching query",
		RunE: func(cmd *cobra.Command, args []string) error {
			insts, err := instances()
			if err != nil {
				return err
			}
			recs, failures := gather(insts, forEachInstance(cmd.Context(), insts, fetchLibrary))
			recs = titlesMatching(recs, strings.Join(args, " "))
			if err := renderLibrary(cmd.OutOrStdout(), recs, flags.json); err != nil {
				return err
			}
			return reportFailures(cmd.ErrOrStderr(), failures)
		},
	}
}

func fetchLibrary(ctx context.Context, inst config.Instance) instanceResult[libraryRecord] {
	key, err := inst.ResolveAPIKey()
	if err != nil {
		return instanceResult[libraryRecord]{err: err}
	}
	client := arrapi.New(inst.URL, key)
	var recs []libraryRecord
	switch inst.App {
	case config.Sonarr:
		series, err := client.AllSeries(ctx)
		if err != nil {
			return instanceResult[libraryRecord]{err: err}
		}
		for _, s := range series {
			recs = append(recs, seriesRecord(inst, s))
		}
	case config.Radarr:
		movies, err := client.Movies(ctx)
		if err != nil {
			return instanceResult[libraryRecord]{err: err}
		}
		for _, m := range movies {
			recs = append(recs, movieRecord(inst, m))
		}
	}
	slices.SortStableFunc(recs, func(a, b libraryRecord) int { return cmp.Compare(a.sortTitle, b.sortTitle) })
	return instanceResult[libraryRecord]{recs: recs}
}

func seriesRecord(inst config.Instance, s arrapi.Series) libraryRecord {
	st := s.Statistics
	return libraryRecord{
		Instance:     inst.Name,
		App:          inst.App,
		Title:        s.Title,
		Year:         s.Year,
		Status:       s.Status,
		Monitored:    s.Monitored,
		SizeOnDisk:   st.SizeOnDisk,
		TvdbID:       s.TvdbID,
		Seasons:      &st.SeasonCount,
		Episodes:     &st.EpisodeCount,
		EpisodeFiles: &st.EpisodeFileCount,
		sortTitle:    cmp.Or(s.SortTitle, strings.ToLower(s.Title)),
	}
}

func movieRecord(inst config.Instance, m arrapi.Movie) libraryRecord {
	return libraryRecord{
		Instance:   inst.Name,
		App:        inst.App,
		Title:      m.Title,
		Year:       m.Year,
		Status:     m.Status,
		Monitored:  m.Monitored,
		SizeOnDisk: m.SizeOnDisk,
		TmdbID:     m.TmdbID,
		HasFile:    &m.HasFile,
		sortTitle:  cmp.Or(m.SortTitle, strings.ToLower(m.Title)),
	}
}

// titlesMatching keeps the records whose Title contains query, ignoring case. An
// empty query keeps them all.
func titlesMatching(recs []libraryRecord, query string) []libraryRecord {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return recs
	}
	var out []libraryRecord
	for _, r := range recs {
		if strings.Contains(strings.ToLower(r.Title), query) {
			out = append(out, r)
		}
	}
	return out
}

func renderLibrary(w io.Writer, recs []libraryRecord, asJSON bool) error {
	if asJSON {
		return renderJSON(w, recs)
	}
	rows := make([][]string, 0, len(recs))
	for _, r := range recs {
		rows = append(rows, []string{
			r.Instance, r.displayTitle(), orDash(r.Status), yesNo(r.Monitored), r.displayFiles(), formatSize(r.SizeOnDisk),
		})
	}
	return renderTable(w, []string{"INSTANCE", "TITLE", "STATUS", "MONITORED", "FILES", "SIZE"}, rows)
}

// displayFiles says how many of a Series' episodes are on disk, or whether a
// Movie is.
func (r libraryRecord) displayFiles() string {
	switch r.App {
	case config.Sonarr:
		return fmt.Sprintf("%d/%d episodes", *r.EpisodeFiles, *r.Episodes)
	case config.Radarr:
		if *r.HasFile {
			return "downloaded"
		}
		return "missing"
	}
	return "-"
}

// formatSize writes a byte count in binary units, such as "8.0 GiB", or "-"
// for nothing.
func formatSize(n int64) string {
	if n <= 0 {
		return "-"
	}
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit && exp < 4; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTP"[exp])
}

func (r libraryRecord) displayTitle() string { return titleWithYear(r.Title, r.Year) }
