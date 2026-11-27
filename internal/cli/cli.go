// Package cli builds the arr command tree. Instance commands are registered
// from the config when it loads (ADR 0002).
package cli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"slices"

	"github.com/spf13/cobra"

	"github.com/drzippit/arr-cli/internal/config"
	"github.com/drzippit/arr-cli/internal/release"
)

// Run executes arr with args and returns the process exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	root, err := newRootCmd(stdout, stderr, args)
	if err != nil {
		fmt.Fprintln(stderr, "arr:", err)
		return 1
	}
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		var silent silentError
		if !errors.As(err, &silent) {
			fmt.Fprintln(stderr, "arr:", err)
		}
		return 1
	}
	return 0
}

// silentError is a failure that has already been reported to the user.
type silentError struct{ error }

type globalFlags struct {
	json bool
}

// reservedWithoutCommand are names no Instance may take although there is no
// top-level command by that name: search runs only against one Instance (ADR
// 0002) but is kept free in case that changes, so a config that loads today
// keeps loading if it becomes a top-level command.
var reservedWithoutCommand = []string{"search"}

// newRootCmd builds the command tree for running arr with args. A config that
// cannot be loaded is an error, except when args ask for shell completion or
// --version: then the tree has the top-level commands only, so a completion
// script can still be printed, TAB still offers them, and a broken setup can
// still say which Release it is.
func newRootCmd(stdout, stderr io.Writer, args []string) (*cobra.Command, error) {
	flags := &globalFlags{}
	root := &cobra.Command{
		Use:           "arr",
		Short:         "Run a self-hosted *arr media stack from the terminal",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.PersistentFlags().BoolVar(&flags.json, "json", false, "print JSON instead of a table")
	// --version is our own flag rather than cobra's, which would also take -v,
	// the shorthand arr status uses for verbose (ADR 0004).
	showVersion := root.Flags().Bool("version", false, "print which Release of arr this is")
	root.RunE = func(cmd *cobra.Command, _ []string) error {
		if !*showVersion {
			return cmd.Help()
		}
		info := release.Current()
		if flags.json {
			return renderJSONValue(cmd.OutOrStdout(), info)
		}
		_, err := fmt.Fprintln(cmd.OutOrStdout(), "arr", info)
		return err
	}

	// Top-level commands are registered before the config loads, so every
	// name they use is reserved and cannot be taken by an Instance.
	src := &instanceSource{path: config.Path()}
	root.AddCommand(newStatusCmd(src.all, flags))
	root.AddCommand(newQueueCmd(src.all, true, flags))
	root.AddCommand(newCalendarCmd(src.mediaApps, flags))
	root.AddCommand(newListCmd(src.mediaApps, flags))
	root.AddCommand(newHistoryCmd(src.downloadClients, flags))
	root.AddCommand(newConfigCmd(src, flags))
	root.InitDefaultHelpCmd()
	root.InitDefaultCompletionCmd()

	src.reserved = reservedNames(root)
	cfg, err := config.Load(src.path, src.reserved...)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		root.Long = fmt.Sprintf("No config found at %s; add an Instance with arr config add, or set %s.", src.path, config.EnvVar)
		src.missing = true
		return root, nil
	case err != nil && (completing(args) || slices.Contains(args, "--version")):
		src.loadErr = err
		return root, nil
	case err != nil:
		return nil, err
	}

	src.instances = cfg.Instances
	for _, inst := range cfg.Instances {
		root.AddCommand(newInstanceCmd(inst, flags))
	}
	return root, nil
}

// completing reports whether args ask for shell completion: a completion
// script, or the words a script offers at TAB.
func completing(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch args[0] {
	// "completion" is the command root.InitDefaultCompletionCmd adds.
	case "completion", cobra.ShellCompRequestCmd, cobra.ShellCompNoDescRequestCmd:
		return true
	}
	return false
}

// reservedNames returns every top-level command name and alias, and the names
// in reservedWithoutCommand.
func reservedNames(root *cobra.Command) []string {
	names := slices.Clone(reservedWithoutCommand)
	for _, cmd := range root.Commands() {
		names = append(names, cmd.Name())
		names = append(names, cmd.Aliases...)
	}
	return names
}

// instanceSource gives top-level commands, which are registered before the
// config loads, what loading it found: the Instances, for commands that run
// across every Instance and for arr config, and reserved, the names no
// Instance may take.
type instanceSource struct {
	path      string
	reserved  []string
	missing   bool
	loadErr   error // why the config did not load, when completing or printing --version anyway
	instances []config.Instance
}

func (s *instanceSource) all() ([]config.Instance, error) {
	switch {
	case s.loadErr != nil:
		return nil, s.loadErr
	case s.missing:
		return nil, fmt.Errorf("no config found at %s; add an Instance with arr config add, or set %s", s.path, config.EnvVar)
	case len(s.instances) == 0:
		return nil, fmt.Errorf("no Instances configured in %s; add one with arr config add", s.path)
	}
	return s.instances, nil
}

// mediaApps is all, keeping only the Instances of Media Apps, for commands
// about Titles and Libraries.
func (s *instanceSource) mediaApps() ([]config.Instance, error) {
	return s.ofKind(config.MediaApp, "Sonarr or Radarr")
}

// downloadClients is all, keeping only the Instances of Download Clients.
func (s *instanceSource) downloadClients() ([]config.Instance, error) {
	return s.ofKind(config.DownloadClient, "SABnzbd")
}

// ofKind is all, keeping only the Instances of Apps of kind, which apps
// names for the error when there are none.
func (s *instanceSource) ofKind(kind config.Kind, apps string) ([]config.Instance, error) {
	insts, err := s.all()
	if err != nil {
		return nil, err
	}
	var out []config.Instance
	for _, inst := range insts {
		if inst.App.Kind() == kind {
			out = append(out, inst)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no %s Instances configured in %s; add one with arr config add", apps, s.path)
	}
	return out, nil
}

func newInstanceCmd(inst config.Instance, flags *globalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   inst.Name,
		Short: fmt.Sprintf("Commands for the %s Instance at %s", inst.App, inst.URL),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	only := func() ([]config.Instance, error) { return []config.Instance{inst}, nil }
	cmd.AddCommand(newStatusCmd(only, flags))
	cmd.AddCommand(newQueueCmd(only, false, flags))
	if inst.App.Kind() == config.DownloadClient {
		cmd.AddCommand(newHistoryCmd(only, flags))
	}
	if inst.App.Kind() == config.MediaApp {
		cmd.AddCommand(newCalendarCmd(only, flags))
		cmd.AddCommand(newListCmd(only, flags))
		cmd.AddCommand(newSearchCmd(inst, flags))
	}
	switch inst.App {
	case config.Sonarr:
		cmd.AddCommand(newAddSeriesCmd(inst, flags))
	case config.Radarr:
		cmd.AddCommand(newAddMovieCmd(inst, flags))
	}
	return cmd
}
