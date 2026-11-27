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
	// QualityProfile is the name of the Title's quality profile.
	QualityProfile string `json:"qualityProfile,omitempty"`

	sortTitle string
	// missing says whether the Title is Missing.
	missing bool
	// id is the Title's ID in its Instance's Library.
	id int
	// qualityProfileID is the ID of the Title's quality profile.
	qualityProfileID int
}

// newListCmd builds a list command for the Instances that instances returns:
// one for `arr <instance> list`, or all of them for `arr list`. A query of
// several words may be quoted or not. --plain prints just the Titles, for
// sharing a Library as text.
func newListCmd(instances func() ([]config.Instance, error), flags *globalFlags) *cobra.Command {
	var (
		plain  bool
		filter titleFilter
	)
	cmd := &cobra.Command{
		Use:   "list [query]",
		Short: "Show the Titles in each Instance's Library, optionally only those matching query",
		RunE: func(cmd *cobra.Command, args []string) error {
			if plain && flags.json {
				return fmt.Errorf("--plain and --json cannot be used together")
			}
			insts, err := instances()
			if err != nil {
				return err
			}
			if err := filter.check(insts); err != nil {
				return err
			}
			libs := forEachInstance(cmd.Context(), insts, fetchLibrary)
			if err := filter.checkQuality(libs); err != nil {
				return err
			}
			recs, failures := gather(insts, libraryResults(libs))
			recs = titlesMatching(recs, strings.Join(args, " "))
			recs = slices.DeleteFunc(recs, func(r libraryRecord) bool { return !filter.matches(r) })
			w := cmd.OutOrStdout()
			switch {
			case flags.json:
				err = renderJSON(w, recs)
			case plain:
				err = renderPlainLibrary(w, recs, len(insts) > 1)
			default:
				err = renderLibrary(w, recs)
			}
			if err != nil {
				return err
			}
			return reportFailures(cmd.ErrOrStderr(), failures)
		},
	}
	cmd.Flags().BoolVar(&plain, "plain", false, "print one Title per line, without a table")
	filter.addFlags(cmd)
	return cmd
}

// library is one Instance's Library, with its quality profiles, or why it
// could not be fetched. profilesErr is why the profiles could not be fetched;
// the Titles are still there, without their profile.
type library struct {
	titles      []libraryRecord
	profiles    []arrapi.QualityProfile
	profilesErr error
	err         error
}

// libraryResults gives the Titles of each library, for gather.
func libraryResults(libs []library) []instanceResult[libraryRecord] {
	out := make([]instanceResult[libraryRecord], len(libs))
	for i, l := range libs {
		out[i] = instanceResult[libraryRecord]{recs: l.titles, err: l.err}
	}
	return out
}

// fetchLibrary fetches the Titles in inst's Library, sorted by title, and the
// names of its quality profiles.
func fetchLibrary(ctx context.Context, inst config.Instance) library {
	key, err := inst.ResolveAPIKey()
	if err != nil {
		return library{err: err}
	}
	client := arrapi.New(inst.URL, key)
	// Without its quality profiles a Library is still worth showing, so a
	// failure here is kept for the commands that need the profiles.
	profiles, profilesErr := client.QualityProfiles(ctx)
	lib := library{profiles: profiles, profilesErr: profilesErr}
	profileName := map[int]string{}
	for _, p := range profiles {
		profileName[p.ID] = p.Name
	}
	switch inst.App {
	case config.Sonarr:
		series, err := client.AllSeries(ctx)
		if err != nil {
			return library{err: err}
		}
		for _, s := range series {
			r := seriesRecord(inst, s)
			r.id, r.qualityProfileID = s.ID, s.QualityProfileID
			r.QualityProfile = profileName[s.QualityProfileID]
			lib.titles = append(lib.titles, r)
		}
	case config.Radarr:
		movies, err := client.Movies(ctx)
		if err != nil {
			return library{err: err}
		}
		for _, m := range movies {
			r := movieRecord(inst, m)
			r.id, r.qualityProfileID = m.ID, m.QualityProfileID
			r.QualityProfile = profileName[m.QualityProfileID]
			lib.titles = append(lib.titles, r)
		}
	}
	slices.SortStableFunc(lib.titles, func(a, b libraryRecord) int { return cmp.Compare(a.sortTitle, b.sortTitle) })
	return lib
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
		// Sonarr's EpisodeCount counts the aired, monitored episodes and
		// those with a file.
		missing: s.Monitored && st.EpisodeFileCount < st.EpisodeCount,
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
		missing:    m.Monitored && m.IsAvailable && !m.HasFile,
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

func renderLibrary(w io.Writer, recs []libraryRecord) error {
	rows := make([][]string, 0, len(recs))
	for _, r := range recs {
		rows = append(rows, []string{
			r.Instance, r.displayTitle(), orDash(r.Status), yesNo(r.Monitored), r.displayFiles(), formatSize(r.SizeOnDisk),
		})
	}
	return renderTable(w, []string{"INSTANCE", "TITLE", "STATUS", "MONITORED", "FILES", "SIZE"}, rows)
}

// renderPlainLibrary writes one Title per line. With headed set, each
// Instance's Titles follow a line with its name, and a blank line separates
// the Instances.
func renderPlainLibrary(w io.Writer, recs []libraryRecord, headed bool) error {
	var b strings.Builder
	for i, r := range recs {
		if headed && (i == 0 || recs[i-1].Instance != r.Instance) {
			if i > 0 {
				b.WriteString("\n")
			}
			b.WriteString(r.Instance + "\n")
		}
		b.WriteString(r.displayTitle() + "\n")
	}
	_, err := io.WriteString(w, b.String())
	return err
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
