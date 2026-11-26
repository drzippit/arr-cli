package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/drzippit/arr-cli/internal/config"
)

// topLevelCommands lists the commands arr offers at the top level, as shell
// completion sees them.
func topLevelCommands(t *testing.T) []string {
	t.Helper()
	out, stderr, code := run(t, "__complete", "")
	if code != 0 {
		t.Fatalf("__complete failed: %s", stderr)
	}
	var names []string
	for _, line := range strings.Split(out, "\n") {
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		name, _, _ := strings.Cut(line, "\t")
		names = append(names, name)
	}
	if len(names) == 0 {
		t.Fatalf("no top-level commands found in:\n%s", out)
	}
	return names
}

func TestEveryTopLevelCommandIsReservedAsInstanceName(t *testing.T) {
	useConfig(t)
	commands := topLevelCommands(t)
	if !strings.Contains(strings.Join(commands, " "), "status") {
		t.Fatalf("expected status among top-level commands, got %v", commands)
	}

	for _, name := range commands {
		t.Run(name, func(t *testing.T) {
			useConfig(t, instanceDef{name, "sonarr", "http://127.0.0.1:1", testAPIKey})

			_, stderr, code := run(t, "status")

			if code == 0 || !strings.Contains(stderr, "clashes with the top-level command") {
				t.Errorf("an Instance named %q should be rejected, got exit %d, stderr %q", name, code, stderr)
			}
		})
	}
}

func TestStatusWithoutConfigFileExplainsWhereItLooked(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope.toml")
	t.Setenv(config.EnvVar, missing)

	out, stderr, code := run(t, "status")

	if code != 1 {
		t.Errorf("exit code %d, want 1", code)
	}
	if out != "" {
		t.Errorf("want no output, got:\n%s", out)
	}
	if !strings.Contains(stderr, "no config found at "+missing) {
		t.Errorf("stderr should name the missing config, got %q", stderr)
	}
}

func TestStatusWithNoInstancesConfiguredSaysSo(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("# nothing here yet\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(config.EnvVar, path)

	out, stderr, code := run(t, "status")

	if code != 1 {
		t.Errorf("exit code %d, want 1", code)
	}
	if out != "" {
		t.Errorf("want no output, got:\n%s", out)
	}
	if !strings.Contains(stderr, "no Instances configured in "+path) {
		t.Errorf("stderr should say no Instances are configured, got %q", stderr)
	}
}

func TestSearchIsReservedAsInstanceName(t *testing.T) {
	useConfig(t, instanceDef{"search", "sonarr", "http://127.0.0.1:1", testAPIKey})

	_, stderr, code := run(t, "status")

	if code == 0 || !strings.Contains(stderr, "clashes with the top-level command") {
		t.Errorf("an Instance named search should be rejected, got exit %d, stderr %q", code, stderr)
	}
}
