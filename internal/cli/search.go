package cli

import (
	"context"
	"io"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/drzippit/arr-cli/internal/arrapi"
	"github.com/drzippit/arr-cli/internal/config"
)

// candidateRecord is one Candidate an App's lookup returned. TvdbID is set for
// Sonarr, TmdbID for Radarr.
type candidateRecord struct {
	Instance  string     `json:"instance"`
	App       config.App `json:"app"`
	Title     string     `json:"title"`
	Year      int        `json:"year,omitempty"`
	TvdbID    int        `json:"tvdbId,omitempty"`
	TmdbID    int        `json:"tmdbId,omitempty"`
	InLibrary bool       `json:"inLibrary"`
}

// newSearchCmd builds `arr <instance> search query`, which shows the
// Candidates the Instance's App finds for query, in the order the App gives
// them. A query of several words may be quoted or not.
func newSearchCmd(inst config.Instance, flags *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "search query",
		Short: "Find Series or Movies that could be added to this Instance",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			recs, err := fetchCandidates(cmd.Context(), inst, strings.Join(args, " "))
			if err != nil {
				return err
			}
			if flags.json {
				return renderJSON(cmd.OutOrStdout(), recs)
			}
			return renderCandidates(cmd.OutOrStdout(), inst.App, recs)
		},
	}
}

func fetchCandidates(ctx context.Context, inst config.Instance, term string) ([]candidateRecord, error) {
	key, err := inst.ResolveAPIKey()
	if err != nil {
		return nil, err
	}
	client := arrapi.New(inst.URL, key)
	var recs []candidateRecord
	switch inst.App {
	case config.Sonarr:
		series, err := client.LookupSeries(ctx, term)
		if err != nil {
			return nil, err
		}
		for _, s := range series {
			recs = append(recs, seriesCandidate(inst, s))
		}
	case config.Radarr:
		movies, err := client.LookupMovies(ctx, term)
		if err != nil {
			return nil, err
		}
		for _, m := range movies {
			recs = append(recs, movieCandidate(inst, m))
		}
	}
	return recs, nil
}

func seriesCandidate(inst config.Instance, s arrapi.Series) candidateRecord {
	return candidateRecord{
		Instance:  inst.Name,
		App:       inst.App,
		Title:     s.Title,
		Year:      s.Year,
		TvdbID:    s.TvdbID,
		InLibrary: s.ID != 0,
	}
}

func movieCandidate(inst config.Instance, m arrapi.Movie) candidateRecord {
	return candidateRecord{
		Instance:  inst.Name,
		App:       inst.App,
		Title:     m.Title,
		Year:      m.Year,
		TmdbID:    m.TmdbID,
		InLibrary: m.ID != 0,
	}
}

// renderCandidates writes a table with the TVDB ID for Sonarr or the TMDB ID
// for Radarr, marking the Candidates already in the Library.
func renderCandidates(w io.Writer, app config.App, recs []candidateRecord) error {
	rows := make([][]string, 0, len(recs))
	for _, r := range recs {
		rows = append(rows, []string{r.displayTitle(), r.displayID(), r.libraryMark()})
	}
	return renderTable(w, []string{"TITLE", idLabel(app), "LIBRARY"}, rows)
}

// idLabel names the ID an App's Candidates carry: TVDB for Sonarr, TMDB for
// Radarr.
func idLabel(app config.App) string {
	if app == config.Radarr {
		return "TMDB"
	}
	return "TVDB"
}

func (r candidateRecord) displayTitle() string { return titleWithYear(r.Title, r.Year) }

// displayID gives the Candidate's TVDB or TMDB ID; it carries only one of the
// two, and the other is 0.
func (r candidateRecord) displayID() string { return idOrDash(r.TvdbID + r.TmdbID) }

// libraryMark marks a Candidate already in the Library.
func (r candidateRecord) libraryMark() string {
	if r.InLibrary {
		return "✓ in library"
	}
	return ""
}

func idOrDash(id int) string {
	if id == 0 {
		return "-"
	}
	return strconv.Itoa(id)
}
