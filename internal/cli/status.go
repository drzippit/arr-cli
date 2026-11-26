package cli

import (
	"context"
	"errors"
	"io"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/drzippit/arr-cli/internal/arrapi"
	"github.com/drzippit/arr-cli/internal/config"
)

// statusRecord is the status of one Instance. Supported says whether
// arr-cli supports its version, once that is known.
type statusRecord struct {
	Instance  string               `json:"instance"`
	App       config.App           `json:"app"`
	Reachable bool                 `json:"reachable"`
	Version   string               `json:"version"`
	Supported *bool                `json:"supported,omitempty"`
	Warnings  *int                 `json:"warnings"`
	Error     string               `json:"error,omitempty"`
	Health    []arrapi.HealthCheck `json:"health,omitempty"`
}

// newStatusCmd builds a status command for the Instances that instances
// returns: one for `arr <instance> status`, or all of them for `arr status`.
func newStatusCmd(instances func() ([]config.Instance, error), flags *globalFlags) *cobra.Command {
	var verbose bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show whether each Instance is reachable, its version and its Warnings",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			insts, err := instances()
			if err != nil {
				return err
			}
			recs := forEachInstance(cmd.Context(), insts, fetchStatus)
			if err := renderStatus(cmd.OutOrStdout(), recs, verbose, flags.json); err != nil {
				return err
			}
			var failures []failure
			for _, r := range recs {
				if r.Error != "" {
					failures = append(failures, failure{r.Instance, r.Error})
				}
			}
			return reportFailures(cmd.ErrOrStderr(), failures)
		},
	}
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "list each Warning")
	return cmd
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
	supported := supportedVersion(inst.App, sys.Version)
	rec.Supported = &supported
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
		rows = append(rows, []string{r.Instance, string(r.App), yesNo(r.Reachable), r.displayVersion(), countOrDash(r.Warnings)})
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

// displayVersion gives the version, marked when arr-cli does not support it.
func (r statusRecord) displayVersion() string {
	if r.Supported != nil && !*r.Supported {
		return r.Version + " (unsupported)"
	}
	return r.Version
}
