// Command arr runs a self-hosted *arr media stack from the terminal.
package main

import (
	"os"

	"github.com/drzippit/arr-cli/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
