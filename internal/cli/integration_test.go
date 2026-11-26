//go:build integration

// The tests in this file run arr against real Sonarr and Radarr Instances in
// Docker, before a release. `make integration` starts the Instances, runs
// them and stops the Instances again; see docs/integration-tests.md.

package cli_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// realInstanceFor returns the Instance of app that `make integration`
// started, named after its App, from the ARR_IT_<APP>_URL and
// ARR_IT_<APP>_API_KEY variables it sets. Without them the test fails, since
// only `make integration` should run this file.
func realInstanceFor(t *testing.T, app string) instanceDef {
	t.Helper()
	prefix := "ARR_IT_" + strings.ToUpper(app) + "_"
	url, key := os.Getenv(prefix+"URL"), os.Getenv(prefix+"API_KEY")
	if url == "" || key == "" {
		t.Fatalf("%sURL and %sAPI_KEY are not set; run the integration tests with make integration", prefix, prefix)
	}
	return instanceDef{name: app, app: app, url: url, key: key}
}

func TestIntegrationStatusOfSonarr(t *testing.T) {
	useConfig(t, realInstanceFor(t, "sonarr"))

	out, stderr, code := run(t, "sonarr", "status")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s\n%s", code, stderr, out)
	}
	assertRow(t, out, "sonarr", "sonarr", "yes", " 4.0.")
}

func TestIntegrationStatusOfRadarr(t *testing.T) {
	useConfig(t, realInstanceFor(t, "radarr"))

	out, stderr, code := run(t, "radarr", "status")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s\n%s", code, stderr, out)
	}
	assertRow(t, out, "radarr", "radarr", "yes", " 7.0.")
}

func TestIntegrationStatusAcrossBothApps(t *testing.T) {
	useConfig(t, realInstanceFor(t, "sonarr"), realInstanceFor(t, "radarr"))

	out, stderr, code := run(t, "status", "--json")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s\n%s", code, stderr, out)
	}
	var recs []struct {
		Instance  string `json:"instance"`
		Reachable bool   `json:"reachable"`
		Supported bool   `json:"supported"`
	}
	if err := json.Unmarshal([]byte(out), &recs); err != nil {
		t.Fatalf("status --json is not a list: %v\n%s", err, out)
	}
	if len(recs) != 2 {
		t.Fatalf("got %d records, want one per Instance:\n%s", len(recs), out)
	}
	for _, r := range recs {
		if !r.Reachable || !r.Supported {
			t.Errorf("%s should be reachable and supported:\n%s", r.Instance, out)
		}
	}
}
