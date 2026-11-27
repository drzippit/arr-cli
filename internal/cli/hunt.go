package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/drzippit/arr-cli/internal/config"
)

// huntRecord is one Title of a Hunt's Selection, with how its Hunt went:
// queued when the App took the search, failed when it did not.
type huntRecord struct {
	libraryRecord
	Hunt      string `json:"hunt,omitempty"`
	HuntError string `json:"huntError,omitempty"`
}

// defaultHuntCap is how many Titles a Hunt may cover without --max or --all,
// since each is a search against the indexers' API limits (ADR 0006).
const defaultHuntCap = 25

// newHuntCmd builds `arr <instance> hunt`, which asks a Media App to look
// through its indexers for Releases of a Selection, as its own search does.
func newHuntCmd(inst config.Instance, flags *globalFlags) *cobra.Command {
	var (
		filter  titleFilter
		guard   selectionGuard
		huntCap int
	)
	cmd := &cobra.Command{
		Use:   "hunt [query]",
		Short: "Look through the indexers for Releases of many Titles at once",
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := makeSelection(cmd, inst, args, filter, guard)
			if err != nil {
				return err
			}
			ctx, client, sel := cmd.Context(), s.client, s.titles
			w, stderr := cmd.OutOrStdout(), cmd.ErrOrStderr()
			recs := make([]huntRecord, len(sel))
			for i, r := range sel {
				recs[i] = huntRecord{libraryRecord: r}
			}
			if len(sel) == 0 {
				fmt.Fprintln(stderr, "arr: no Titles matched; nothing hunted")
				return renderHunts(w, recs, flags.json)
			}
			if len(sel) > huntCap && !guard.all {
				return fmt.Errorf("this Hunt covers %s, more than %d; pass --max %d to hunt them all, or narrow the Selection",
					countTitles(inst.App, len(sel)), huntCap, len(sel))
			}
			if guard.dryRun {
				if err := renderHunts(w, recs, flags.json); err != nil {
					return err
				}
				fmt.Fprintln(stderr, "arr: dry run; nothing hunted")
				return nil
			}
			question := fmt.Sprintf("Hunt for Releases of %s in %s?", countTitles(inst.App, len(sel)), inst.Name)
			if err := guard.confirm(cmd, question, len(sel)); err != nil {
				return err
			}
			if inst.App == config.Sonarr {
				for i := range recs {
					recs[i].hunted(client.SearchSeries(ctx, recs[i].id))
				}
			} else {
				ids := make([]int, len(recs))
				for i, r := range recs {
					ids[i] = r.id
				}
				err := client.SearchMovies(ctx, ids)
				for i := range recs {
					recs[i].hunted(err)
				}
			}
			if err := renderHunts(w, recs, flags.json); err != nil {
				return err
			}
			var failures []failure
			for _, r := range recs {
				if r.HuntError != "" {
					failures = append(failures, failure{inst.Name + ": " + r.Title, r.HuntError})
				}
			}
			return reportFailures(stderr, failures)
		},
	}
	filter.addFlags(cmd)
	guard.addFlags(cmd)
	cmd.Flags().IntVar(&huntCap, "max", defaultHuntCap, "hunt at most this many Titles; a bigger Selection is refused")
	return cmd
}

// hunted records how the Hunt for r went: err is why the App did not take
// the search, or nil.
func (r *huntRecord) hunted(err error) {
	if err != nil {
		r.Hunt, r.HuntError = "failed", err.Error()
		return
	}
	r.Hunt = "queued"
}

func renderHunts(w io.Writer, recs []huntRecord, asJSON bool) error {
	if asJSON {
		if recs == nil {
			recs = []huntRecord{}
		}
		return renderJSON(w, recs)
	}
	rows := make([][]string, len(recs))
	for i, r := range recs {
		hunt := orDash(r.Hunt)
		if r.Hunt == "failed" {
			hunt = "FAILED"
		}
		rows[i] = []string{r.Instance, r.displayTitle(), yesNo(r.Monitored), hunt}
	}
	return renderTable(w, []string{"INSTANCE", "TITLE", "MONITORED", "HUNT"}, rows)
}
