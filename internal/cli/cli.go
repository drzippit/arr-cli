// Package cli builds the arr command tree. Instance commands are registered
// from the config when it loads (ADR 0002).
package cli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"

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

	path := config.Path()
	cfg, err := config.Load(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		root.Long = fmt.Sprintf("No config found at %s; add Instances there or set %s.", path, config.EnvVar)
		cfg = &config.Config{}
	case err != nil:
		return nil, err
	}

	for _, inst := range cfg.Instances {
		root.AddCommand(newInstanceCmd(inst, flags))
	}
	return root, nil
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
	cmd.AddCommand(newStatusCmd(inst, flags))
	return cmd
}
