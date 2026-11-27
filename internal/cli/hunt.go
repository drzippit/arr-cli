package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/drzippit/arr-cli/internal/config"
)

// huntRecord is one Title of a Hunt's Selection, with how its Hunt went:
// queued when the App took it, failed when it did not.
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
			sel, err := makeSelection(cmd, inst, args, filter, guard)
			if err != nil {
				return err
			}
			ctx, w := cmd.Context(), cmd.OutOrStdout()
			titles, err := guard.settle(cmd, inst, sel.titles, bulkAction{
				verb: "hunt", done: "hunted",
				pickTitle: "Which to hunt? All are ticked; untick any to leave out",
				question: func(n int) string {
					return fmt.Sprintf("Hunt for Releases of %s in %s?", countTitles(inst.App, n), inst.Name)
				},
				check: func(titles []libraryRecord) error {
					if len(titles) > huntCap && !guard.all {
						return fmt.Errorf("this Hunt covers %s, more than %d; pass --max %d or --all to hunt them all, or narrow the Selection",
							countTitles(inst.App, len(titles)), huntCap, len(titles))
					}
					return nil
				},
				render: func(titles []libraryRecord) error { return renderHunts(w, huntRecords(titles), flags.json) },
			})
			if err != nil || titles == nil {
				return err
			}
			recs := huntRecords(titles)
			if inst.App == config.Sonarr {
				for i := range recs {
					recs[i].hunted(sel.client.SearchSeries(ctx, recs[i].id))
				}
			} else {
				err := sel.client.SearchMovies(ctx, titleIDs(titles))
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
			return reportFailures(cmd.ErrOrStderr(), failures)
		},
	}
	filter.addFlags(cmd)
	guard.addFlags(cmd)
	cmd.Flags().IntVar(&huntCap, "max", defaultHuntCap, "hunt at most this many Titles; a bigger Selection is refused")
	return cmd
}

// huntRecords gives a row for each of titles, not yet hunted.
func huntRecords(titles []libraryRecord) []huntRecord {
	recs := make([]huntRecord, len(titles))
	for i, r := range titles {
		recs[i] = huntRecord{libraryRecord: r}
	}
	return recs
}

// hunted records how the Hunt for r went: err is why the App did not take
// it, or nil.
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
		rows[i] = append(r.libraryRow(), hunt)
	}
	return renderTable(w, append(libraryHeader(), "HUNT"), rows)
}
