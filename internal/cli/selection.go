package cli

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"

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
