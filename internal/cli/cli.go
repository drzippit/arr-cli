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
)

// Run executes arr with args and returns the process exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	root, err := newRootCmd(stdout, stderr)
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
// top-level command by that name: config is planned, and search runs only
// against one Instance (ADR 0002) but is kept free in case that changes. A
// config that loads today keeps loading if either becomes a top-level command.
var reservedWithoutCommand = []string{"config", "search"}

func newRootCmd(stdout, stderr io.Writer) (*cobra.Command, error) {
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

	// Top-level commands are registered before the config loads, so every
	// name they use is reserved and cannot be taken by an Instance.
	src := &instanceSource{path: config.Path()}
	root.AddCommand(newStatusCmd(src.all, flags))
	root.AddCommand(newQueueCmd(src.all, flags))
	root.AddCommand(newCalendarCmd(src.all, flags))
	root.AddCommand(newListCmd(src.all, flags))
	root.InitDefaultHelpCmd()
	root.InitDefaultCompletionCmd()

	cfg, err := config.Load(src.path, reservedNames(root)...)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		root.Long = fmt.Sprintf("No config found at %s; add Instances there or set %s.", src.path, config.EnvVar)
		src.missing = true
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

// instanceSource gives commands that run across every Instance the Instances
// from the config, which loads after those commands are registered.
type instanceSource struct {
	path      string
	missing   bool
	instances []config.Instance
}

func (s *instanceSource) all() ([]config.Instance, error) {
	switch {
	case s.missing:
		return nil, fmt.Errorf("no config found at %s; create it or set %s", s.path, config.EnvVar)
	case len(s.instances) == 0:
		return nil, fmt.Errorf("no Instances configured in %s", s.path)
	}
	return s.instances, nil
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
	cmd.AddCommand(newQueueCmd(only, flags))
	cmd.AddCommand(newCalendarCmd(only, flags))
	cmd.AddCommand(newListCmd(only, flags))
	cmd.AddCommand(newSearchCmd(inst, flags))
	switch inst.App {
	case config.Sonarr:
		cmd.AddCommand(newAddSeriesCmd(inst, flags))
	case config.Radarr:
		cmd.AddCommand(newAddMovieCmd(inst, flags))
	}
	return cmd
}
