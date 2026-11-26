package cli_test

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/drzippit/arr-cli/internal/config"
)

// complete returns the words arr offers a shell completing the last of
// words, as the completion scripts ask for them.
func complete(t *testing.T, words ...string) []string {
	t.Helper()
	out, stderr, code := run(t, append([]string{"__complete"}, words...)...)
	if code != 0 {
		t.Fatalf("__complete %q: exit code %d, stderr: %s", words, code, stderr)
	}
	var offered []string
	for _, line := range strings.Split(out, "\n") {
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		word, _, _ := strings.Cut(line, "\t")
		offered = append(offered, word)
	}
	return offered
}

// useRadarrRadarr4kAndSonarr configures Instances named radarr, radarr-4k and
// sonarr at URLs that cannot be reached, since completion never calls them.
func useRadarrRadarr4kAndSonarr(t *testing.T) {
	t.Helper()
	useConfig(t,
		instanceDef{"radarr", "radarr", "http://radarr.invalid", testAPIKey},
		instanceDef{"radarr-4k", "radarr", "http://radarr-4k.invalid", testAPIKey},
		instanceDef{"sonarr", "sonarr", "http://sonarr.invalid", testAPIKey},
	)
}

// useUnparseableConfig points arr at a config file that is not valid TOML.
func useUnparseableConfig(t *testing.T) {
	t.Helper()
	useConfigTOML(t, "[[instance]]\nname = \"radarr\"\napp = ")
}

// assertCompletionScripts checks that arr prints a completion script for
// each shell.
func assertCompletionScripts(t *testing.T) {
	t.Helper()
	scripts := map[string]string{"fish": "complete -c arr", "zsh": "#compdef arr", "bash": "__start_arr"}
	for shell, want := range scripts {
		t.Run(shell, func(t *testing.T) {
			out, stderr, code := run(t, "completion", shell)

			if code != 0 || !strings.Contains(out, want) {
				t.Errorf("completion %s: exit %d, stderr %q, script missing %q", shell, code, stderr, want)
			}
		})
	}
}

func TestCompletionScriptForEachShell(t *testing.T) {
	useRadarrRadarr4kAndSonarr(t)

	assertCompletionScripts(t)
}

func TestCompletionOfInstanceNamePrefix(t *testing.T) {
	useRadarrRadarr4kAndSonarr(t)

	got := complete(t, "ra")

	if want := []string{"radarr", "radarr-4k"}; !slices.Equal(got, want) {
		t.Errorf("arr ra<TAB> offered %q, want %q", got, want)
	}
}

func TestCompletionOfInstanceCommands(t *testing.T) {
	useRadarrRadarr4kAndSonarr(t)

	got := complete(t, "radarr", "")

	for _, want := range []string{"add", "calendar", "list", "queue", "search", "status"} {
		if !slices.Contains(got, want) {
			t.Errorf("arr radarr <TAB> offered %q, missing %q", got, want)
		}
	}
	for _, unwanted := range []string{"radarr-4k", "sonarr", "config"} {
		if slices.Contains(got, unwanted) {
			t.Errorf("arr radarr <TAB> offered %q, which is not a command of the Instance", unwanted)
		}
	}
}

func TestCompletionOffersTopLevelCommandsAlongsideInstances(t *testing.T) {
	useRadarrRadarr4kAndSonarr(t)

	got := complete(t, "")

	for _, want := range []string{"calendar", "config", "list", "queue", "status", "radarr", "radarr-4k", "sonarr"} {
		if !slices.Contains(got, want) {
			t.Errorf("arr <TAB> offered %q, missing %q", got, want)
		}
	}
}

func TestCompletionScriptsWhenConfigCannotBeParsed(t *testing.T) {
	useUnparseableConfig(t)

	assertCompletionScripts(t)
}

func TestCompletionOffersTopLevelCommandsWhenConfigCannotBeParsed(t *testing.T) {
	useUnparseableConfig(t)

	got := complete(t, "")

	for _, want := range []string{"config", "status"} {
		if !slices.Contains(got, want) {
			t.Errorf("arr <TAB> offered %q, missing %q", got, want)
		}
	}
}

func TestCompletionOffersTopLevelCommandsWithoutConfig(t *testing.T) {
	t.Setenv(config.EnvVar, filepath.Join(t.TempDir(), "missing.toml"))

	got := complete(t, "")

	for _, want := range []string{"config", "status"} {
		if !slices.Contains(got, want) {
			t.Errorf("arr <TAB> offered %q, missing %q", got, want)
		}
	}
}
