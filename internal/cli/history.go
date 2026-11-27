package cli

import (
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/drzippit/arr-cli/internal/config"
	"github.com/drzippit/arr-cli/internal/sabapi"
)

// historyRecord is one finished Download in a Download Client's History,
// known by its Release's name, since only a Media App knows the Title it is for.
// Status is completed or failed, and FailMessage says why a failed one
// failed.
type historyRecord struct {
	Instance    string     `json:"instance"`
	App         config.App `json:"app"`
	Release     string     `json:"release"`
	DownloadID  string     `json:"downloadId"`
	Status      string     `json:"status"`
	Size        int64      `json:"size"`
	Category    string     `json:"category,omitempty"`
	Finished    time.Time  `json:"finished"`
	FailMessage string     `json:"failMessage,omitempty"`
}

// defaultHistoryLimit is how many Downloads arr history shows without
// --limit.
const defaultHistoryLimit = 20

// newHistoryCmd builds a history command for the Download Client Instances
// that instances returns: one for `arr <instance> history`, or all of them
// for `arr history`.
func newHistoryCmd(instances func() ([]config.Instance, error), flags *globalFlags) *cobra.Command {
	var (
		failedOnly bool
		limit      int
	)
	cmd := &cobra.Command{
		Use:   "history",
		Short: "Show the Downloads each Download Client has finished, newest first",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if limit < 1 {
				return errors.New("--limit must be 1 or more")
			}
			insts, err := instances()
			if err != nil {
				return err
			}
			fetch := func(ctx context.Context, inst config.Instance) instanceResult[historyRecord] {
				return fetchHistory(ctx, inst, limit, failedOnly)
			}
			recs, failures := gather(insts, forEachInstance(cmd.Context(), insts, fetch))
			slices.SortStableFunc(recs, func(a, b historyRecord) int { return b.Finished.Compare(a.Finished) })
			recs = recs[:min(len(recs), limit)]
			if err := renderHistory(cmd.OutOrStdout(), recs, flags.json); err != nil {
				return err
			}
			return reportFailures(cmd.ErrOrStderr(), failures)
		},
	}
	cmd.Flags().BoolVar(&failedOnly, "failed", false, "show only the Downloads that failed")
	cmd.Flags().IntVar(&limit, "limit", defaultHistoryLimit, "show at most this many Downloads")
	return cmd
}

// fetchHistory gives the newest limit finished Downloads of one SABnzbd
// Instance. SABnzbd's history also holds the Downloads it is still
// post-processing, which belong to the Queue, so when some of those were
// among the newest it asks again for as many more.
func fetchHistory(ctx context.Context, inst config.Instance, limit int, failedOnly bool) instanceResult[historyRecord] {
	key, err := inst.ResolveAPIKey()
	if err != nil {
		return instanceResult[historyRecord]{err: err}
	}
	client := sabapi.New(inst.URL, key)
	slots, err := client.History(ctx, limit, failedOnly)
	if err != nil {
		return instanceResult[historyRecord]{err: err}
	}
	if unfinished := len(slots) - countFinished(slots); unfinished > 0 {
		if slots, err = client.History(ctx, limit+unfinished, failedOnly); err != nil {
			return instanceResult[historyRecord]{err: err}
		}
	}
	var recs []historyRecord
	for _, s := range slots {
		if !s.Finished() || (failedOnly && s.Status != "Failed") {
			continue
		}
		recs = append(recs, historyRecord{
			Instance: inst.Name, App: inst.App, Release: s.Name, DownloadID: s.NzoID,
			Status: strings.ToLower(s.Status), Size: s.Bytes, Category: s.Category,
			Finished: time.Unix(s.Completed, 0).UTC(), FailMessage: s.FailMessage,
		})
	}
	return instanceResult[historyRecord]{recs: recs}
}

func countFinished(slots []sabapi.HistorySlot) int {
	n := 0
	for _, s := range slots {
		if s.Finished() {
			n++
		}
	}
	return n
}

func renderHistory(w io.Writer, recs []historyRecord, asJSON bool) error {
	if asJSON {
		return renderJSON(w, recs)
	}
	loc := now().Location()
	rows := make([][]string, 0, len(recs))
	for _, r := range recs {
		rows = append(rows, []string{
			r.Instance, r.Finished.In(loc).Format("2006-01-02 15:04"), r.Release, r.Status,
			formatSize(r.Size), orDash(r.Category), orDash(r.FailMessage),
		})
	}
	return renderTable(w, []string{"INSTANCE", "FINISHED", "RELEASE", "STATUS", "SIZE", "CATEGORY", "REASON"}, rows)
}
