package cli_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/drzippit/arr-cli/internal/cli"
	"github.com/drzippit/arr-cli/internal/config"
)

const testAPIKey = "test-key"

// fakeInstance serves the recorded /api/v3 responses in testdata/<app>.
func fakeInstance(t *testing.T, app string) *httptest.Server {
	t.Helper()
	routes := map[string]string{
		"/api/v3/system/status": "system_status.json",
		"/api/v3/health":        "health.json",
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != testAPIKey {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		file, ok := routes[r.URL.Path]
		if !ok {
			t.Errorf("unexpected request to %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		body, err := os.ReadFile(filepath.Join("testdata", app, file))
		if err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// instanceDef is one [[instance]] entry for a test config.
type instanceDef struct {
	name, app, url, key string
}

func useConfig(t *testing.T, defs ...instanceDef) {
	t.Helper()
	var b strings.Builder
	for _, d := range defs {
		fmt.Fprintf(&b, "[[instance]]\nname = %q\napp = %q\nurl = %q\napi_key = %q\n\n", d.name, d.app, d.url, d.key)
	}
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(config.EnvVar, path)
}

func run(t *testing.T, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = cli.Run(args, &out, &errOut)
	return out.String(), errOut.String(), code
}

func TestStatusShowsReachableInstanceAsTable(t *testing.T) {
	srv := fakeInstance(t, "sonarr")
	useConfig(t, instanceDef{"sonarr", "sonarr", srv.URL, testAPIKey})

	out, stderr, code := run(t, "sonarr", "status")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	for _, want := range []string{"INSTANCE", "APP", "REACHABLE", "VERSION", "WARNINGS", "sonarr", "yes", "4.0.11.2680", "2"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Missing root folder") {
		t.Errorf("warnings listed without -v:\n%s", out)
	}
}

func TestStatusVerboseListsHealthWarnings(t *testing.T) {
	srv := fakeInstance(t, "sonarr")
	useConfig(t, instanceDef{"sonarr", "sonarr", srv.URL, testAPIKey})

	out, stderr, code := run(t, "sonarr", "status", "-v")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	for _, want := range []string{
		"warning", "IndexerStatusCheck", "Indexers unavailable due to failures: NZBgeek",
		"error", "RootFolderCheck", "Missing root folder: /tv/anime",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestStatusJSONIsFlatUnstyledArray(t *testing.T) {
	srv := fakeInstance(t, "radarr")
	useConfig(t, instanceDef{"radarr", "radarr", srv.URL, testAPIKey})

	out, stderr, code := run(t, "radarr", "status", "--json")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	want := `[
  {
    "instance": "radarr",
    "app": "radarr",
    "reachable": true,
    "version": "5.14.0.9383",
    "warnings": 0
  }
]
`
	if out != want {
		t.Errorf("output:\n%s\nwant:\n%s", out, want)
	}
}

func TestStatusVerboseJSONIncludesHealth(t *testing.T) {
	srv := fakeInstance(t, "sonarr")
	useConfig(t, instanceDef{"sonarr", "sonarr", srv.URL, testAPIKey})

	out, stderr, code := run(t, "sonarr", "status", "--json", "-v")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	var got []map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}
	if len(got) != 1 {
		t.Fatalf("got %d records, want 1", len(got))
	}
	health, ok := got[0]["health"].([]any)
	if !ok || len(health) != 2 {
		t.Fatalf("health = %v, want 2 entries", got[0]["health"])
	}
	first := health[0].(map[string]any)
	if first["source"] != "IndexerStatusCheck" || first["type"] != "warning" {
		t.Errorf("first health entry = %v", first)
	}
}

func TestStatusReportsUnreachableInstance(t *testing.T) {
	srv := fakeInstance(t, "sonarr")
	url := srv.URL
	srv.Close()
	useConfig(t, instanceDef{"sonarr", "sonarr", url, testAPIKey})

	out, stderr, code := run(t, "sonarr", "status", "--json")

	if code == 0 {
		t.Errorf("exit code 0, want non-zero for an unreachable Instance")
	}
	var got []map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s\nstderr: %s", err, out, stderr)
	}
	if got[0]["reachable"] != false || got[0]["warnings"] != nil || got[0]["error"] == "" {
		t.Errorf("record = %v, want unreachable with error and no warning count", got[0])
	}
}

func TestStatusCountsRejectedAPIKeyAsReachable(t *testing.T) {
	srv := fakeInstance(t, "radarr")
	useConfig(t, instanceDef{"radarr", "radarr", srv.URL, "wrong-key"})

	out, stderr, code := run(t, "radarr", "status", "--json")

	if code == 0 {
		t.Errorf("exit code 0, want non-zero when the API key is rejected")
	}
	var got []map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}
	if got[0]["reachable"] != true || got[0]["warnings"] != nil {
		t.Errorf("record = %v, want reachable with no warning count", got[0])
	}
	if !strings.Contains(stderr, "401 Unauthorized") {
		t.Errorf("stderr should explain the failure, got %q", stderr)
	}
}

func TestStatusCountsOnlyWarningsAndErrors(t *testing.T) {
	srv := fakeInstance(t, "sonarr")
	useConfig(t, instanceDef{"sonarr", "sonarr", srv.URL, testAPIKey})

	out, _, _ := run(t, "sonarr", "status", "--json", "-v")

	var got []map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}
	if got[0]["warnings"] != float64(2) {
		t.Errorf("warnings = %v, want 2 (the notice does not count)", got[0]["warnings"])
	}
	if strings.Contains(out, "New update is available") {
		t.Errorf("notice listed among warnings:\n%s", out)
	}
}

func TestEachConfiguredInstanceGetsItsOwnCommand(t *testing.T) {
	sonarr := fakeInstance(t, "sonarr")
	radarr := fakeInstance(t, "radarr")
	useConfig(t,
		instanceDef{"sonarr", "sonarr", sonarr.URL, testAPIKey},
		instanceDef{"radarr-4k", "radarr", radarr.URL, testAPIKey},
	)

	out, stderr, code := run(t, "radarr-4k", "status")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	if !strings.Contains(out, "radarr-4k") || !strings.Contains(out, "5.14.0.9383") {
		t.Errorf("want radarr-4k status:\n%s", out)
	}
	if strings.Contains(out, "4.0.11.2680") {
		t.Errorf("status of another Instance leaked in:\n%s", out)
	}
}

func TestUnknownInstanceFails(t *testing.T) {
	useConfig(t, instanceDef{"sonarr", "sonarr", "http://127.0.0.1:1", testAPIKey})

	_, stderr, code := run(t, "lidarr", "status")

	if code == 0 {
		t.Error("exit code 0 for an Instance that is not in the config")
	}
	if !strings.Contains(stderr, "lidarr") {
		t.Errorf("stderr should name the unknown Instance, got %q", stderr)
	}
}

func TestInvalidConfigFailsBeforeRunning(t *testing.T) {
	useConfig(t, instanceDef{"status", "sonarr", "http://127.0.0.1:1", testAPIKey})

	_, stderr, code := run(t, "status")

	if code == 0 {
		t.Error("exit code 0 with an invalid config")
	}
	if !strings.Contains(stderr, "reserved") {
		t.Errorf("stderr should explain the config error, got %q", stderr)
	}
}
