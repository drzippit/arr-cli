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

// newAddMovieCmd builds `arr <radarr-instance> add`, which adds a Movie to
// the Instance's Library, monitored, from a query or a TMDB ID.
func newAddMovieCmd(inst config.Instance) *cobra.Command {
	var (
		tmdbID        int
		quality, root string
		noSearch      bool
	)
	cmd := &cobra.Command{
		Use:   "add [query]",
		Short: "Add a Movie to this Instance's Library, monitored, and search for it",
		RunE: func(cmd *cobra.Command, args []string) error {
			query := strings.Join(args, " ")
			switch {
			case tmdbID == 0 && query == "":
				return fmt.Errorf("give a query or --tmdb <id>")
			case tmdbID != 0 && query != "":
				return fmt.Errorf("give a query or --tmdb <id>, not both")
			}
			key, err := inst.ResolveAPIKey()
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			a := &adder{
				inst:   inst,
				client: arrapi.New(inst.URL, key),
				choose: newChooser(cmd.InOrStdin(), cmd.OutOrStdout()),
				stderr: cmd.ErrOrStderr(),
			}

			movie, err := a.pickMovie(ctx, query, tmdbID)
			if err != nil {
				return err
			}
			if movie.ID != 0 {
				return fmt.Errorf("%s is already in library", titleWithYear(movie.Title, movie.Year))
			}
			profileID, err := a.qualityProfile(ctx, quality)
			if err != nil {
				return err
			}
			rootPath, err := a.rootFolder(ctx, root)
			if err != nil {
				return err
			}
			err = a.client.AddMovie(ctx, arrapi.NewMovie{
				Candidate:        movie,
				QualityProfileID: profileID,
				RootFolderPath:   rootPath,
				Search:           !noSearch,
			})
			if err != nil {
				return err
			}
			msg := fmt.Sprintf("Added %s to %s", titleWithYear(movie.Title, movie.Year), inst.Name)
			if noSearch {
				msg += ", without searching for it"
			} else {
				msg += " and started a search for it"
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), msg)
			return err
		},
	}
	f := cmd.Flags()
	f.IntVar(&tmdbID, "tmdb", 0, "add the Movie with this TMDB ID instead of searching")
	f.StringVar(&quality, "quality", "", "name of the quality profile (default: quality_profile in the config)")
	f.StringVar(&root, "root", "", "root folder to keep the Movie in (default: root_folder in the config)")
	f.BoolVar(&noSearch, "no-search", false, "add the Movie without searching for it")
	return cmd
}

// adder holds what adding a Title to one Instance needs. choose is nil when
// arr cannot ask anyone.
type adder struct {
	inst   config.Instance
	client *arrapi.Client
	choose chooser
	stderr io.Writer
}

// pickMovie finds the Candidate to add: the one with TMDB ID tmdbID, or the
// one picked from those matching query. Without a terminal to ask in, a
// query must match exactly one Candidate. A Movie already in the Library
// comes back with its Library ID.
func (a *adder) pickMovie(ctx context.Context, query string, tmdbID int) (arrapi.Movie, error) {
	if tmdbID != 0 {
		m, err := a.client.LookupMovieByTMDB(ctx, tmdbID)
		if err != nil {
			return m, err
		}
		if inLib, ok, err := a.client.LibraryMovieByTMDB(ctx, tmdbID); err != nil || ok {
			return inLib, err
		}
		return m, nil
	}
	movies, err := a.client.LookupMovies(ctx, query)
	if err != nil {
		return arrapi.Movie{}, err
	}
	recs := make([]candidateRecord, len(movies))
	for i, m := range movies {
		recs[i] = movieCandidate(a.inst, m)
	}
	i, err := a.pickCandidate(recs, query, "Movie", "--tmdb")
	if err != nil {
		return arrapi.Movie{}, err
	}
	return movies[i], nil
}

// pickCandidate returns the index of the Candidate to add from those that
// matched query: the only one, or the one picked at the terminal. Without a
// terminal, several Candidates are listed on stderr and are an error that
// points to idFlag.
func (a *adder) pickCandidate(recs []candidateRecord, query, noun, idFlag string) (int, error) {
	switch {
	case len(recs) == 0:
		return 0, fmt.Errorf("no %ss found for %q", noun, query)
	case a.choose == nil && len(recs) == 1:
		return 0, nil
	case a.choose == nil:
		if err := renderCandidates(a.stderr, a.inst.App, recs); err != nil {
			return 0, err
		}
		return 0, fmt.Errorf("%d %ss match %q; add one with %s <id>", len(recs), noun, query, idFlag)
	}
	labels := make([]string, len(recs))
	for i, r := range recs {
		labels[i] = candidateLabel(r)
	}
	return a.choose(fmt.Sprintf("Which %s?", noun), labels)
}

// candidateLabel names a Candidate in a picker, as
// "The Matrix (1999) · TMDB 603 · ✓ in library".
func candidateLabel(r candidateRecord) string {
	parts := []string{r.displayTitle(), idLabel(r.App) + " " + r.displayID()}
	if mark := r.libraryMark(); mark != "" {
		parts = append(parts, mark)
	}
	return strings.Join(parts, " · ")
}

// qualityProfile returns the ID of the quality profile named by flag, or else
// by the Instance's config, or else picked at the terminal.
func (a *adder) qualityProfile(ctx context.Context, flag string) (int, error) {
	profiles, err := a.client.QualityProfiles(ctx)
	if err != nil {
		return 0, err
	}
	names := make([]string, len(profiles))
	for i, p := range profiles {
		names[i] = p.Name
	}
	i, err := a.resolve(setting{
		noun: "quality profile", flag: "quality", configKey: "quality_profile",
		value: cmp.Or(flag, a.inst.QualityProfile), options: names, matches: strings.EqualFold,
	})
	if err != nil {
		return 0, err
	}
	return profiles[i].ID, nil
}

// rootFolder returns the root folder given by flag, or else by the
// Instance's config, or else picked at the terminal.
func (a *adder) rootFolder(ctx context.Context, flag string) (string, error) {
	folders, err := a.client.RootFolders(ctx)
	if err != nil {
		return "", err
	}
	paths := make([]string, len(folders))
	for i, f := range folders {
		paths[i] = f.Path
	}
	samePath := func(a, b string) bool { return strings.TrimRight(a, "/") == strings.TrimRight(b, "/") }
	i, err := a.resolve(setting{
		noun: "root folder", flag: "root", configKey: "root_folder",
		value: cmp.Or(flag, a.inst.RootFolder), options: paths, matches: samePath,
	})
	if err != nil {
		return "", err
	}
	return paths[i], nil
}

// setting is one choice adding a Title needs, such as its quality profile,
// named by noun. value is what --flag or configKey in the config gave, if
// anything; matches says whether it names an option.
type setting struct {
	noun, flag, configKey string
	value                 string
	options               []string
	matches               func(value, option string) bool
}

// resolve returns the index of the option s.value names, or asks for one at
// the terminal when there is no value.
func (a *adder) resolve(s setting) (int, error) {
	available := strings.Join(s.options, ", ")
	if s.value != "" {
		i := slices.IndexFunc(s.options, func(o string) bool { return s.matches(s.value, o) })
		if i < 0 {
			return 0, fmt.Errorf("%s has no %s %q; available: %s", a.inst.Name, s.noun, s.value, available)
		}
		return i, nil
	}
	if len(s.options) == 0 {
		return 0, fmt.Errorf("%s has no %ss; add one in the App first", a.inst.Name, s.noun)
	}
	if a.choose == nil {
		return 0, fmt.Errorf("no %s given: pass --%s or set %s for Instance %q in %s (available: %s)",
			s.noun, s.flag, s.configKey, a.inst.Name, config.Path(), available)
	}
	return a.choose(fmt.Sprintf("Which %s?", s.noun), s.options)
}
