package cli_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/drzippit/arr-cli/internal/config"
)

func TestVersionFlagPrintsTheArrVersion(t *testing.T) {
	useConfig(t)

	out, stderr, code := run(t, "--version")

	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr)
	}
	// Tests are not stamped with a tag, so this is a dev build.
	if !strings.HasPrefix(out, "arr dev") || strings.Count(out, "\n") != 1 {
		t.Errorf("want one line starting %q, got %q", "arr dev", out)
	}
}

func TestVersionFlagPrintsJSON(t *testing.T) {
	useConfig(t)

	out, stderr, code := run(t, "--version", "--json")

	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("want JSON, got %q: %v", out, err)
	}
	if got["version"] != "dev" {
		t.Errorf("want version dev, got %v", got)
	}
}

func TestVersionFlagWorksWithBrokenConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("this is not [toml"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(config.EnvVar, path)

	out, stderr, code := run(t, "--version")

	if code != 0 || !strings.HasPrefix(out, "arr ") {
		t.Errorf("exit %d, stdout %q, stderr %q", code, out, stderr)
	}
}

func TestVersionHasNoShortFlag(t *testing.T) {
	useConfig(t)

	out, _, code := run(t, "-v")

	if code == 0 || strings.HasPrefix(out, "arr ") {
		t.Errorf("-v should not print the version, got exit %d and %q", code, out)
	}
}

func TestRequestsToInstancesNameTheArrVersion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); got != "arr-cli/dev" {
			t.Errorf("%s: want User-Agent arr-cli/dev, got %q", r.URL.Path, got)
		}
		recordedHandler(t, "sonarr").ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	useConfig(t, instanceDef{"sonarr", "sonarr", srv.URL, testAPIKey})

	if _, stderr, code := run(t, "sonarr", "status"); code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr)
	}
}
