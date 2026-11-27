package cli

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/drzippit/arr-cli/internal/arrapi"
	"github.com/drzippit/arr-cli/internal/config"
)

// editRecord is one Title of a Selection after edit, with what changed.
type editRecord struct {
	libraryRecord
	Changes map[string]change `json:"changes"`
}

// change is one setting of a Title going from one value to another.
type change struct {
	From any `json:"from"`
	To   any `json:"to"`
}

// selectionGuard holds the flags that guard a Selection (ADR 0006).
type selectionGuard struct {
	yes, dryRun, all bool
}

func (g *selectionGuard) addFlags(cmd *cobra.Command) {
	cmd.Flags().BoolVar(&g.yes, "yes", false, "change the Selection without asking")
	cmd.Flags().BoolVar(&g.dryRun, "dry-run", false, "show what would change, and change nothing")
	cmd.Flags().BoolVar(&g.all, "all", false, "make the whole Library the Selection when there is no query or filter")
}

// newEditCmd builds `arr <instance> edit`, which changes the monitoring and
// quality profile of a Selection in one request to the App's bulk editor.
func newEditCmd(inst config.Instance, flags *globalFlags) *cobra.Command {
	var (
		filter       titleFilter
		guard        selectionGuard
		setMonitored bool
		setQuality   string
	)
	cmd := &cobra.Command{
		Use:   "edit [query]",
		Short: "Change the monitoring or quality profile of many Titles at once",
		RunE: func(cmd *cobra.Command, args []string) error {
			monitorChanged := cmd.Flags().Changed("set-monitored")
			if !monitorChanged && setQuality == "" {
				return errors.New("pass --set-monitored or --set-quality to say what to change")
			}
			query := strings.Join(args, " ")
			if err := guard.checkSelection(query, filter); err != nil {
				return err
			}
			if err := filter.check([]config.Instance{inst}); err != nil {
				return err
			}
			ctx := cmd.Context()
			client, err := newMediaAppClient(inst)
			if err != nil {
				return err
			}
			if err := requireSupportedVersion(ctx, inst, client); err != nil {
				return err
			}
			lib := fetchLibrary(ctx, inst)
			if lib.err != nil {
				return lib.err
			}
			// edit shows and sets quality profiles, so it needs them.
			if lib.profilesErr != nil {
				return lib.profilesErr
			}
			if err := filter.checkQuality([]library{lib}); err != nil {
				return err
			}
			var edit arrapi.Edit
			if monitorChanged {
				edit.Monitored = &setMonitored
			}
			var quality arrapi.QualityProfile
			if setQuality != "" {
				i := slices.IndexFunc(lib.profiles, func(p arrapi.QualityProfile) bool { return strings.EqualFold(p.Name, setQuality) })
				if i < 0 {
					return fmt.Errorf("--set-quality %q is not a quality profile of %s; there are %s", setQuality, inst.Name, profileNames(lib.profiles))
				}
				quality = lib.profiles[i]
				edit.QualityProfileID = &quality.ID
			}
			sel := selectTitles(lib.titles, query, filter)
			w, stderr := cmd.OutOrStdout(), cmd.ErrOrStderr()
			if len(sel) == 0 {
				fmt.Fprintln(stderr, "arr: no Titles matched; nothing changed")
				return renderEdits(w, nil, flags.json)
			}
			recs := plannedEdits(sel, edit, quality)
			if guard.dryRun {
				if err := renderEdits(w, recs, flags.json); err != nil {
					return err
				}
				fmt.Fprintln(stderr, "arr: dry run; nothing changed")
				return nil
			}
			question := fmt.Sprintf("Change %s in %s: %s?", countTitles(inst.App, len(sel)), inst.Name, describeEdit(edit, quality))
			if err := guard.confirm(cmd, question, len(sel)); err != nil {
				return err
			}
			ids := make([]int, len(sel))
			for i, r := range sel {
				ids[i] = r.id
			}
			if inst.App == config.Sonarr {
				err = client.EditSeries(ctx, ids, edit)
			} else {
				err = client.EditMovies(ctx, ids, edit)
			}
			if err != nil {
				return err
			}
			return renderEdits(w, recs, flags.json)
		},
	}
	filter.addFlags(cmd)
	guard.addFlags(cmd)
	cmd.Flags().BoolVar(&setMonitored, "set-monitored", false, "monitor (true) or unmonitor (false) the Selection; episodes keep their own setting")
	cmd.Flags().StringVar(&setQuality, "set-quality", "", "give the Selection this quality profile, by name")
	return cmd
}

// newMediaAppClient returns a client for inst, a Media App Instance.
func newMediaAppClient(inst config.Instance) (*arrapi.Client, error) {
	key, err := inst.ResolveAPIKey()
	if err != nil {
		return nil, err
	}
	return arrapi.New(inst.URL, key), nil
}

// checkSelection refuses to make the whole Library the Selection by
// accident: with no query and no filter, --all must say so (ADR 0006).
func (g selectionGuard) checkSelection(query string, f titleFilter) error {
	if query == "" && !f.any() && !g.all {
		return errors.New("no query or filter, so this would act on the whole Library; pass --all if that is what you mean")
	}
	return nil
}

// confirm asks question in a terminal, unless --yes was given. Without a
// terminal it refuses, since a script must say --yes (ADR 0006).
func (g selectionGuard) confirm(cmd *cobra.Command, question string, n int) error {
	if g.yes {
		return nil
	}
	ask := newConfirmer(cmd.InOrStdin(), cmd.OutOrStdout())
	if ask == nil {
		return fmt.Errorf("this would change %d Titles; from a script pass --yes, or --dry-run to see what would change", n)
	}
	ok, err := ask(question)
	if err != nil {
		return err
	}
	if !ok {
		return errCancelled
	}
	return nil
}

// selectTitles is the Selection: the Titles matching query and filter.
func selectTitles(titles []libraryRecord, query string, filter titleFilter) []libraryRecord {
	sel := titlesMatching(slices.Clone(titles), query)
	return slices.DeleteFunc(sel, func(r libraryRecord) bool { return !filter.matches(r) })
}

// plannedEdits gives each Title of sel as it will be after edit, with what
// changes. quality is the profile edit sets, if it sets one.
func plannedEdits(sel []libraryRecord, edit arrapi.Edit, quality arrapi.QualityProfile) []editRecord {
	recs := make([]editRecord, len(sel))
	for i, r := range sel {
		changes := map[string]change{}
		if edit.Monitored != nil && r.Monitored != *edit.Monitored {
			changes["monitored"] = change{r.Monitored, *edit.Monitored}
			r.Monitored = *edit.Monitored
		}
		if edit.QualityProfileID != nil && r.qualityProfileID != quality.ID {
			changes["qualityProfile"] = change{r.QualityProfile, quality.Name}
			r.qualityProfileID, r.QualityProfile = quality.ID, quality.Name
		}
		recs[i] = editRecord{libraryRecord: r, Changes: changes}
	}
	return recs
}

// describeEdit says what edit does, as in "unmonitor, quality HD-1080p".
func describeEdit(edit arrapi.Edit, quality arrapi.QualityProfile) string {
	var parts []string
	if edit.Monitored != nil {
		parts = append(parts, map[bool]string{true: "monitor", false: "unmonitor"}[*edit.Monitored])
	}
	if edit.QualityProfileID != nil {
		parts = append(parts, "quality "+quality.Name)
	}
	return strings.Join(parts, ", ")
}

// countTitles says how many Titles of app's kind n is, as in "2 Movies".
func countTitles(app config.App, n int) string {
	noun := "Series"
	if app == config.Radarr {
		noun = "Movies"
		if n == 1 {
			noun = "Movie"
		}
	}
	return fmt.Sprintf("%d %s", n, noun)
}

func profileNames(profiles []arrapi.QualityProfile) string {
	names := make([]string, len(profiles))
	for i, p := range profiles {
		names[i] = p.Name
	}
	return strings.Join(names, ", ")
}

func renderEdits(w io.Writer, recs []editRecord, asJSON bool) error {
	if asJSON {
		if recs == nil {
			recs = []editRecord{}
		}
		return renderJSON(w, recs)
	}
	rows := make([][]string, len(recs))
	for i, r := range recs {
		rows[i] = []string{r.Instance, r.displayTitle(), yesNo(r.Monitored), orDash(r.QualityProfile), r.displayChanges()}
	}
	return renderTable(w, []string{"INSTANCE", "TITLE", "MONITORED", "QUALITY", "CHANGED"}, rows)
}

// displayChanges lists what changed, as in "quality: HD-720p → HD-1080p".
func (r editRecord) displayChanges() string {
	var parts []string
	if c, ok := r.Changes["monitored"]; ok {
		parts = append(parts, fmt.Sprintf("monitored: %s → %s", yesNo(c.From.(bool)), yesNo(c.To.(bool))))
	}
	if c, ok := r.Changes["qualityProfile"]; ok {
		parts = append(parts, fmt.Sprintf("quality: %s → %s", c.From, c.To))
	}
	return orDash(strings.Join(parts, ", "))
}
