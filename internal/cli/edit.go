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
			sel, err := makeSelection(cmd, inst, args, filter, guard)
			if err != nil {
				return err
			}
			ctx, client, lib := cmd.Context(), sel.client, sel.lib
			// edit shows and sets quality profiles, so it needs them.
			if lib.profilesErr != nil {
				return lib.profilesErr
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
			w := cmd.OutOrStdout()
			titles, err := guard.settle(cmd, inst, sel.titles, bulkAction{
				verb: "change", done: "changed",
				pickTitle: "Which to change? All are ticked; untick any to leave out",
				question: func(n int) string {
					return fmt.Sprintf("Change %s in %s: %s?", countTitles(inst.App, n), inst.Name, describeEdit(edit, quality))
				},
				render: func(titles []libraryRecord) error {
					return renderEdits(w, plannedEdits(titles, edit, quality), flags.json)
				},
			})
			if err != nil || titles == nil {
				return err
			}
			if inst.App == config.Sonarr {
				err = client.EditSeries(ctx, titleIDs(titles), edit)
			} else {
				err = client.EditMovies(ctx, titleIDs(titles), edit)
			}
			if err != nil {
				return err
			}
			return renderEdits(w, plannedEdits(titles, edit, quality), flags.json)
		},
	}
	filter.addFlags(cmd)
	guard.addFlags(cmd)
	cmd.Flags().BoolVar(&setMonitored, "set-monitored", false, "monitor (true) or unmonitor (false) the Selection; episodes keep their own setting")
	cmd.Flags().StringVar(&setQuality, "set-quality", "", "give the Selection this quality profile, by name")
	return cmd
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
		rows[i] = append(r.libraryRow(), r.displayChanges())
	}
	return renderTable(w, append(libraryHeader(), "CHANGED"), rows)
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
