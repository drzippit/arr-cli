package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"sync"

	"github.com/spf13/cobra"

	"github.com/drzippit/arr-cli/internal/arrapi"
	"github.com/drzippit/arr-cli/internal/config"
)

// statusRecord is the status of one Instance.
type statusRecord struct {
	Instance  string               `json:"instance"`
	App       config.App           `json:"app"`
	Reachable bool                 `json:"reachable"`
	Version   string               `json:"version"`
	Warnings  *int                 `json:"warnings"`
	Error     string               `json:"error,omitempty"`
	Health    []arrapi.HealthCheck `json:"health,omitempty"`
}

// newStatusCmd builds a status command for the given Instances: one for
// `arr <instance> status`, or all of them for `arr status`.
func newStatusCmd(insts []config.Instance, flags *globalFlags) *cobra.Command {
	var verbose bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show whether each Instance is reachable, its version and its Warnings",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			recs := fetchStatusAll(cmd.Context(), insts)
			if err := renderStatus(cmd.OutOrStdout(), recs, verbose, flags.json); err != nil {
				return err
			}
			return reportFailures(cmd.ErrOrStderr(), recs)
		},
	}
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "list each Warning")
	return cmd
}

// fetchStatusAll asks every Instance at the same time and returns their
// records in config order.
func fetchStatusAll(ctx context.Context, insts []config.Instance) []statusRecord {
	recs := make([]statusRecord, len(insts))
	var wg sync.WaitGroup
	for i, inst := range insts {
		wg.Go(func() { recs[i] = fetchStatus(ctx, inst) })
	}
	wg.Wait()
	return recs
}

func fetchStatus(ctx context.Context, inst config.Instance) statusRecord {
	rec := statusRecord{Instance: inst.Name, App: inst.App}
	key, err := inst.ResolveAPIKey()
	if err != nil {
		return rec.failed(err)
	}
	client := arrapi.New(inst.URL, key)
	sys, err := client.SystemStatus(ctx)
	if err != nil {
		// An Instance that answers with an HTTP error is still reachable.
		var statusErr *arrapi.StatusError
		rec.Reachable = errors.As(err, &statusErr)
		return rec.failed(err)
	}
	rec.Reachable = true
	rec.Version = sys.Version
	health, err := client.Health(ctx)
	if err != nil {
		return rec.failed(err)
	}
	rec.Health = warningsOnly(health)
	n := len(rec.Health)
	rec.Warnings = &n
	return rec
}

func (r statusRecord) failed(err error) statusRecord {
	r.Error = err.Error()
	return r
}

// warningsOnly keeps the health checks of type warning or error, dropping
// notices.
func warningsOnly(checks []arrapi.HealthCheck) []arrapi.HealthCheck {
	var out []arrapi.HealthCheck
	for _, c := range checks {
		if c.Type == "warning" || c.Type == "error" {
			out = append(out, c)
		}
	}
	return out
}

func renderStatus(w io.Writer, recs []statusRecord, verbose, asJSON bool) error {
	if asJSON {
		if verbose {
			return renderJSON(w, recs)
		}
		brief := make([]statusRecord, len(recs))
		for i, r := range recs {
			r.Health = nil
			brief[i] = r
		}
		return renderJSON(w, brief)
	}
	rows := make([][]string, 0, len(recs))
	var warnings [][]string
	for _, r := range recs {
		rows = append(rows, []string{r.Instance, string(r.App), yesNo(r.Reachable), r.Version, countOrDash(r.Warnings)})
		for _, h := range r.Health {
			warnings = append(warnings, []string{r.Instance, h.Type, h.Source, h.Message})
		}
	}
	if err := renderTable(w, []string{"INSTANCE", "APP", "REACHABLE", "VERSION", "WARNINGS"}, rows); err != nil {
		return err
	}
	if !verbose || len(warnings) == 0 {
		return nil
	}
	return renderTable(w, []string{"INSTANCE", "TYPE", "SOURCE", "MESSAGE"}, warnings)
}

// reportFailures writes one line per failed Instance to w and returns a
// silentError if there were any, so the exit code is non-zero.
func reportFailures(w io.Writer, recs []statusRecord) error {
	failed := 0
	for _, r := range recs {
		if r.Error != "" {
			fmt.Fprintf(w, "arr: %s: %s\n", r.Instance, r.Error)
			failed++
		}
	}
	if failed > 0 {
		return silentError{fmt.Errorf("%d Instance(s) failed", failed)}
	}
	return nil
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func countOrDash(n *int) string {
	if n == nil {
		return "-"
	}
	return strconv.Itoa(*n)
}
