package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"github.com/drzippit/arr-cli/internal/arrapi"
	"github.com/drzippit/arr-cli/internal/config"
	"github.com/drzippit/arr-cli/internal/sabapi"
)

// newConfigCmd builds `arr config`, which manages the Instances in the config
// file.
func newConfigCmd(src *instanceSource, flags *globalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Add Instances to the config and list them",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newConfigAddCmd(src))
	cmd.AddCommand(newConfigListCmd(src, flags))
	return cmd
}

// newConfigAddCmd builds `arr config add`, which asks for a new Instance,
// checks that arr can reach it, and adds it to the config file.
func newConfigAddCmd(src *instanceSource) *cobra.Command {
	return &cobra.Command{
		Use:   "add",
		Short: "Add an Instance to the config, after checking arr can reach it",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, reserved := src.path, src.reserved
			form := newInstanceForm(cmd.InOrStdin(), cmd.OutOrStdout())
			if form == nil {
				return fmt.Errorf("arr config add asks for the Instance in a terminal; from a script, add it to %s instead", path)
			}
			cfg := &config.Config{Instances: src.instances}
			inst, err := form(func(name string) error { return cfg.CheckName(name, reserved...) })
			if err != nil {
				return err
			}
			if err := cfg.CanAdd(inst, reserved...); err != nil {
				return err
			}
			key, err := inst.ResolveAPIKey()
			if err != nil {
				return fmt.Errorf("%w; nothing saved", err)
			}
			sys, err := systemStatus(cmd.Context(), inst, key)
			if err != nil {
				return fmt.Errorf("cannot reach %s at %s: %w; nothing saved", inst.Name, inst.URL, err)
			}
			if sys.AppName != "" && !strings.EqualFold(sys.AppName, string(inst.App)) {
				return fmt.Errorf("%s runs %s, not %s; nothing saved", inst.URL, sys.AppName, inst.App.DisplayName())
			}
			if err := checkVersion(inst, sys); err != nil {
				return fmt.Errorf("%w; nothing saved", err)
			}
			if err := config.Append(path, inst); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Added %s (%s %s) to %s\n", inst.Name, sys.AppName, sys.Version, path)
			return err
		},
	}
}

// systemStatus asks the Instance which App and version it runs, in a request
// that needs the API key, so a wrong key is caught too. SABnzbd answers that
// with its queue; its own version call needs no key.
func systemStatus(ctx context.Context, inst config.Instance, key string) (arrapi.SystemStatus, error) {
	if inst.App.Kind() == config.DownloadClient {
		q, err := sabapi.New(inst.URL, key).Queue(ctx)
		return arrapi.SystemStatus{AppName: inst.App.DisplayName(), Version: q.Version}, err
	}
	return arrapi.New(inst.URL, key).SystemStatus(ctx)
}

// configRecord is one Instance in the config, without its API key.
// APIKeyFrom names the config key that gives the API key: api_key or
// api_key_cmd.
type configRecord struct {
	Name       string     `json:"name"`
	App        config.App `json:"app"`
	URL        string     `json:"url"`
	APIKeyFrom string     `json:"apiKeyFrom"`
}

// newConfigListCmd builds `arr config list`, which shows the Instances in the
// config. API keys, and the commands that print them, are never shown.
func newConfigListCmd(src *instanceSource, flags *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Show the Instances in the config, with API keys hidden",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			instances, err := src.all()
			if err != nil {
				return err
			}
			recs := make([]configRecord, len(instances))
			for i, inst := range instances {
				recs[i] = configRecord{Name: inst.Name, App: inst.App, URL: inst.URL, APIKeyFrom: inst.APIKeySource()}
			}
			if flags.json {
				return renderJSON(cmd.OutOrStdout(), recs)
			}
			rows := make([][]string, len(recs))
			for i, r := range recs {
				rows[i] = []string{r.Name, string(r.App), r.URL, r.APIKeyFrom + " (hidden)"}
			}
			return renderTable(cmd.OutOrStdout(), []string{"NAME", "APP", "URL", "API KEY"}, rows)
		},
	}
}

// instanceForm asks the person at the terminal for a new Instance, with
// leading and trailing spaces trimmed and only one of APIKey and APIKeyCmd
// set. checkName says why a name cannot be used, so the form can ask again.
type instanceForm func(checkName func(name string) error) (config.Instance, error)

// newInstanceForm returns an instanceForm that asks through in and out, or nil
// when either is not a terminal. Tests replace it.
var newInstanceForm = func(in io.Reader, out io.Writer) instanceForm {
	if !isTerminal(in) || !isTerminal(out) {
		return nil
	}
	return func(checkName func(string) error) (config.Instance, error) {
		return huhInstanceForm(in, out, checkName)
	}
}

// How a new Instance's API key is given.
const (
	keyLiteral = "key"
	keyCommand = "command"
)

// huhInstanceForm asks for a new Instance's name, App, URL and API key, or a
// command that prints the key, asking again for a field that is not valid.
func huhInstanceForm(in io.Reader, out io.Writer, checkName func(string) error) (config.Instance, error) {
	var (
		inst   config.Instance
		source = keyLiteral
	)
	apps := make([]huh.Option[config.App], len(config.Apps))
	for i, a := range config.Apps {
		apps[i] = huh.NewOption(a.DisplayName(), a)
	}
	required := func(what string) func(string) error {
		return func(s string) error {
			if strings.TrimSpace(s) == "" {
				return fmt.Errorf("enter the %s", what)
			}
			return nil
		}
	}
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().Title("Name").Description("What you call the Instance in commands, as in arr <name> status").
				Value(&inst.Name).Validate(func(s string) error { return checkName(strings.TrimSpace(s)) }),
			huh.NewSelect[config.App]().Title("App").Options(apps...).Value(&inst.App),
			huh.NewInput().Title("URL").Placeholder("http://localhost:8989").Value(&inst.URL).Validate(checkURL),
			huh.NewSelect[string]().Title("API key").Options(
				huh.NewOption("Enter the key", keyLiteral),
				huh.NewOption("Run a command that prints it (api_key_cmd)", keyCommand),
			).Value(&source),
		),
		huh.NewGroup(
			huh.NewInput().Title("API key").Description("Settings › General in Sonarr or Radarr, Config › General in SABnzbd").
				EchoMode(huh.EchoModePassword).Value(&inst.APIKey).Validate(required("API key")),
		).WithHideFunc(func() bool { return source != keyLiteral }),
		huh.NewGroup(
			huh.NewInput().Title("Command that prints the API key").Placeholder("pass show sonarr").
				Value(&inst.APIKeyCmd).Validate(required("command")),
		).WithHideFunc(func() bool { return source != keyCommand }),
	).WithInput(in).WithOutput(out)
	err := form.Run()
	if errors.Is(err, huh.ErrUserAborted) {
		return config.Instance{}, errCancelled
	}
	inst.Name = strings.TrimSpace(inst.Name)
	inst.URL = strings.TrimSpace(inst.URL)
	// The person may have filled in both before settling on one.
	if source == keyLiteral {
		inst.APIKey, inst.APIKeyCmd = strings.TrimSpace(inst.APIKey), ""
	} else {
		inst.APIKey, inst.APIKeyCmd = "", strings.TrimSpace(inst.APIKeyCmd)
	}
	return inst, err
}

// checkURL says why s is not an http or https URL arr can call.
func checkURL(s string) error {
	u, err := url.Parse(strings.TrimSpace(s))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.New("enter a URL starting with http:// or https://")
	}
	return nil
}
