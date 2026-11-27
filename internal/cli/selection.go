package cli

import (
	"bufio"
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/drzippit/arr-cli/internal/arrapi"
	"github.com/drzippit/arr-cli/internal/config"
)

// titleStatuses lists the statuses each Media App gives its Titles.
var titleStatuses = map[config.App][]string{
	config.Sonarr: {"continuing", "ended", "upcoming", "deleted"},
	config.Radarr: {"tba", "announced", "inCinemas", "released", "deleted"},
}

// titleFilter narrows the Titles of a Library to those a command is about:
// the ones arr list shows, or a Selection edit and hunt change. A Title must
// match every filter that is set.
type titleFilter struct {
	monitored, unmonitored bool
	missing                bool
	status                 string
	quality                string
}

// addFlags gives cmd the filter's flags.
func (f *titleFilter) addFlags(cmd *cobra.Command) {
	cmd.Flags().BoolVar(&f.monitored, "monitored", false, "only monitored Titles")
	cmd.Flags().BoolVar(&f.unmonitored, "unmonitored", false, "only unmonitored Titles")
	cmd.Flags().StringVar(&f.quality, "quality", "", "only Titles with this quality profile, by name")
	cmd.Flags().StringVar(&f.status, "status", "", "only Titles with this status, in the App's own words, such as ended or released")
	cmd.Flags().BoolVar(&f.missing, "missing", false, "only Titles that are Missing: monitored, out, and without all their files")
}

// check says why the filters cannot be used on insts, if they cannot. A
// status must be one that at least one of their Apps gives.
func (f titleFilter) check(insts []config.Instance) error {
	if f.monitored && f.unmonitored {
		return errors.New("--monitored and --unmonitored cannot be used together")
	}
	if f.status == "" {
		return nil
	}
	var valid []string
	for _, inst := range insts {
		for _, st := range titleStatuses[inst.App] {
			if !slices.Contains(valid, st) {
				valid = append(valid, st)
			}
		}
	}
	if !slices.Contains(valid, f.status) {
		return fmt.Errorf("--status %q is not one of %s", f.status, strings.Join(valid, ", "))
	}
	return nil
}

// matches reports whether r passes every filter that is set.
func (f titleFilter) matches(r libraryRecord) bool {
	switch {
	case f.monitored && !r.Monitored, f.unmonitored && r.Monitored:
		return false
	case f.missing && !r.missing:
		return false
	case f.status != "" && r.Status != f.status:
		return false
	case f.quality != "" && !strings.EqualFold(r.QualityProfile, f.quality):
		return false
	}
	return true
}

// checkQuality says why --quality names no quality profile of the Libraries
// in libs that could be fetched, listing the ones they have. A Library whose
// profiles could not be fetched counts as failed, since --quality needs them.
func (f titleFilter) checkQuality(libs []library) error {
	if f.quality == "" {
		return nil
	}
	for i := range libs {
		if libs[i].err == nil && libs[i].profilesErr != nil {
			libs[i].err = libs[i].profilesErr
		}
	}
	var known []string
	fetched := false
	for _, l := range libs {
		if l.err != nil {
			continue
		}
		fetched = true
		for _, p := range l.profiles {
			if strings.EqualFold(p.Name, f.quality) {
				return nil
			}
			if !slices.Contains(known, p.Name) {
				known = append(known, p.Name)
			}
		}
	}
	if !fetched {
		return nil
	}
	return fmt.Errorf("--quality %q is not a quality profile; there are %s", f.quality, strings.Join(known, ", "))
}

// any reports whether any filter is set.
func (f titleFilter) any() bool {
	return f.monitored || f.unmonitored || f.missing || f.status != "" || f.quality != ""
}

// selection is a Selection in one Instance, with what a bulk command needs to
// act on it: a client for the Instance and its Library.
type selection struct {
	client *arrapi.Client
	lib    library
	titles []libraryRecord
}

// makeSelection makes the Selection of inst that args and filter ask for,
// after the checks every bulk command makes first: the guard against the
// whole Library (ADR 0006), the filters, and the App version (ADR 0003).
//
// A lone "-" as args reads the Selection's IDs from stdin instead of a query.
func makeSelection(cmd *cobra.Command, inst config.Instance, args []string, filter titleFilter, guard selectionGuard) (selection, error) {
	fromStdin := len(args) == 1 && args[0] == "-"
	var ids []int
	query := strings.Join(args, " ")
	if fromStdin {
		// Stdin is the pipe, so there is no one to ask (ADR 0006).
		if !guard.yes && !guard.dryRun {
			return selection{}, errors.New("with -, pass --yes or --dry-run, since arr cannot ask while stdin is a pipe")
		}
		query = ""
		var err error
		if ids, err = readTitleIDs(cmd.InOrStdin(), inst); err != nil {
			return selection{}, err
		}
	} else if err := guard.checkSelection(query, filter); err != nil {
		return selection{}, err
	}
	if err := filter.check([]config.Instance{inst}); err != nil {
		return selection{}, err
	}
	ctx := cmd.Context()
	client, err := newMediaAppClient(inst)
	if err != nil {
		return selection{}, err
	}
	if err := requireSupportedVersion(ctx, inst, client); err != nil {
		return selection{}, err
	}
	lib := fetchLibrary(ctx, inst)
	if lib.err != nil {
		return selection{}, lib.err
	}
	if err := filter.checkQuality([]library{lib}); err != nil {
		return selection{}, err
	}
	titles := lib.titles
	if fromStdin {
		if titles, err = titlesWithIDs(inst, lib.titles, ids); err != nil {
			return selection{}, err
		}
	}
	return selection{client: client, lib: lib, titles: selectTitles(titles, query, filter)}, nil
}

// readTitleIDs reads TVDB or TMDB IDs from r: either the JSON array that arr
// list --json prints for inst, or one ID per line, skipping blank lines.
// Records from another Instance are refused, since their IDs mean nothing in
// inst.
func readTitleIDs(r io.Reader, inst config.Instance) ([]int, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if trimmed := bytes.TrimSpace(data); len(trimmed) > 0 && trimmed[0] == '[' {
		var recs []struct {
			Instance string `json:"instance"`
			TvdbID   int    `json:"tvdbId"`
			TmdbID   int    `json:"tmdbId"`
		}
		if err := json.Unmarshal(trimmed, &recs); err != nil {
			return nil, fmt.Errorf("stdin is not the JSON arr list --json prints: %w", err)
		}
		ids := make([]int, len(recs))
		for i, rec := range recs {
			// A record without its Instance, as from jq 'map({tvdbId})', is
			// taken to be from inst.
			if rec.Instance != "" && rec.Instance != inst.Name {
				return nil, fmt.Errorf("stdin has a record from Instance %q, not %s; its IDs mean nothing here", rec.Instance, inst.Name)
			}
			if ids[i] = cmp.Or(rec.TvdbID, rec.TmdbID); ids[i] == 0 {
				return nil, fmt.Errorf("stdin record %d has no tvdbId or tmdbId", i+1)
			}
		}
		return ids, nil
	}
	var ids []int
	sc := bufio.NewScanner(bytes.NewReader(data))
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		id, err := strconv.Atoi(line)
		if err != nil {
			return nil, fmt.Errorf("stdin line %d: %q is not a TVDB or TMDB ID", n, line)
		}
		ids = append(ids, id)
	}
	return ids, sc.Err()
}

// titlesWithIDs gives the Titles of inst's Library with these TVDB (Sonarr)
// or TMDB (Radarr) IDs, once each however often an ID is given, refusing an ID
// that is not in it.
func titlesWithIDs(inst config.Instance, titles []libraryRecord, ids []int) ([]libraryRecord, error) {
	var out []libraryRecord
	seen := map[int]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		i := slices.IndexFunc(titles, func(r libraryRecord) bool { return r.TvdbID == id || r.TmdbID == id })
		if i < 0 {
			return nil, fmt.Errorf("no Title with ID %d in %s's Library", id, inst.Name)
		}
		out = append(out, titles[i])
	}
	return out, nil
}
