package cli

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/drzippit/arr-cli/internal/arrapi"
	"github.com/drzippit/arr-cli/internal/config"
)

// newAddSeriesCmd builds `arr <sonarr-instance> add`, which adds a Series to
// the Instance's Library, monitored, from a query or a TVDB ID.
func newAddSeriesCmd(inst config.Instance, flags *globalFlags) *cobra.Command {
	var (
		tvdbID                     int
		quality, root, monitorFlag string
		noSearch                   bool
	)
	cmd := &cobra.Command{
		Use:   "add [query]",
		Short: "Add a Series to this Instance's Library, monitored, and search for it",
		RunE: func(cmd *cobra.Command, args []string) error {
			query := strings.Join(args, " ")
			switch {
			case tvdbID == 0 && query == "":
				return fmt.Errorf("give a query or --tvdb <id>")
			case tvdbID != 0 && query != "":
				return fmt.Errorf("give a query or --tvdb <id>, not both")
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

			// Both are checked before anything is looked up; either may be empty.
			if monitorFlag, err = monitorOption(monitorFlag); err != nil {
				return err
			}
			monitorDefault, err := monitorOption(inst.Monitor)
			if err != nil {
				return err
			}
			if err := requireSupportedVersion(ctx, inst, a.client); err != nil {
				return err
			}
			series, err := a.pickSeries(ctx, query, tvdbID)
			if err != nil {
				return err
			}
			if series.ID != 0 {
				return fmt.Errorf("%s is already in library", titleWithYear(series.Title, series.Year))
			}
			profile, err := a.qualityProfile(ctx, quality)
			if err != nil {
				return err
			}
			rootPath, err := a.rootFolder(ctx, root)
			if err != nil {
				return err
			}
			monitor, err := a.monitor(monitorFlag, monitorDefault)
			if err != nil {
				return err
			}
			added, err := a.client.AddSeries(ctx, arrapi.NewSeries{
				Candidate:        series,
				QualityProfileID: profile.ID,
				RootFolderPath:   rootPath,
				Monitor:          monitor,
				Search:           !noSearch,
			})
			if err != nil {
				return err
			}
			if flags.json {
				return renderJSONValue(cmd.OutOrStdout(), addedSeriesRecord{
					Instance:       inst.Name,
					App:            inst.App,
					ID:             added.ID,
					Title:          series.Title,
					Year:           series.Year,
					TvdbID:         series.TvdbID,
					QualityProfile: profile.Name,
					RootFolder:     rootPath,
					Monitor:        monitor,
					Monitored:      true,
					Searching:      !noSearch,
				})
			}
			msg := fmt.Sprintf("Added %s to %s", titleWithYear(series.Title, series.Year), inst.Name)
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
	f.IntVar(&tvdbID, "tvdb", 0, "add the Series with this TVDB ID instead of searching")
	f.StringVar(&quality, "quality", "", "name of the quality profile (default: quality_profile in the config)")
	f.StringVar(&root, "root", "", "root folder to keep the Series in (default: root_folder in the config)")
	f.StringVar(&monitorFlag, "monitor", "", "which episodes to monitor: "+strings.Join(arrapi.MonitorOptions, ", ")+" (default: monitor in the config, else asked in a terminal)")
	f.BoolVar(&noSearch, "no-search", false, "add the Series without searching for it")
	return cmd
}

// addedSeriesRecord is a Series add put in the Library, for --json. ID is its
// ID in the Library; Monitor says which of its episodes are monitored.
type addedSeriesRecord struct {
	Instance       string     `json:"instance"`
	App            config.App `json:"app"`
	ID             int        `json:"id"`
	Title          string     `json:"title"`
	Year           int        `json:"year,omitempty"`
	TvdbID         int        `json:"tvdbId"`
	QualityProfile string     `json:"qualityProfile"`
	RootFolder     string     `json:"rootFolder"`
	Monitor        string     `json:"monitor"`
	Monitored      bool       `json:"monitored"`
	Searching      bool       `json:"searching"`
}

// monitorOption returns Sonarr's spelling of value, one of
// arrapi.MonitorOptions, ignoring case. An empty value stays empty.
func monitorOption(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	return oneOf(arrapi.MonitorOptions, "monitor option", value)
}

// pickSeries finds the Candidate to add: the one with TVDB ID tvdbID, or the
// one picked from those matching query. Without a terminal to ask in, a
// query must match exactly one Candidate. A Series already in the Library
// comes back with its Library ID.
func (a *adder) pickSeries(ctx context.Context, query string, tvdbID int) (arrapi.Series, error) {
	if tvdbID != 0 {
		return a.client.LookupSeriesByTVDB(ctx, tvdbID)
	}
	series, err := a.client.LookupSeries(ctx, query)
	if err != nil {
		return arrapi.Series{}, err
	}
	recs := make([]candidateRecord, len(series))
	for i, s := range series {
		recs[i] = seriesCandidate(a.inst, s)
	}
	i, err := a.pickCandidate(recs, query, "Series", "--tvdb")
	if err != nil {
		return arrapi.Series{}, err
	}
	return series[i], nil
}

// monitor returns which episodes of the Series to monitor: flag if given;
// else, at the terminal, the one picked, starting from the Instance's config
// default (def) or "all"; else def. Both are one of arrapi.MonitorOptions or
// empty.
func (a *adder) monitor(flag, def string) (string, error) {
	switch {
	case flag != "":
		return flag, nil
	case a.choose == nil && def != "":
		return def, nil
	case a.choose == nil:
		return "", a.notGiven(setting{noun: "monitor option", flag: "monitor", configKey: "monitor", options: arrapi.MonitorOptions})
	}
	preset := max(slices.Index(arrapi.MonitorOptions, def), 0)
	i, err := a.choose("Which episodes to monitor?", arrapi.MonitorOptions, preset)
	if err != nil {
		return "", err
	}
	return arrapi.MonitorOptions[i], nil
}
