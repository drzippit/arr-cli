//go:build integration

// The tests in this file run arr against real Sonarr and Radarr Instances in
// Docker, before a release. `make integration` starts the Instances, runs
// them and stops the Instances again; see docs/integration-tests.md.

package cli_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/drzippit/arr-cli/internal/config"
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

// records runs arr with args, which must include --json, and decodes its
// output as a list of records. Every record must name the Instance and App
// it came from.
func records(t *testing.T, args ...string) []map[string]any {
	t.Helper()
	out, stderr, code := run(t, args...)
	if code != 0 {
		t.Fatalf("arr %s: exit code %d, stderr: %s\n%s", strings.Join(args, " "), code, stderr, out)
	}
	var recs []map[string]any
	if err := json.Unmarshal([]byte(out), &recs); err != nil {
		t.Fatalf("arr %s: output is not a JSON list: %v\n%s", strings.Join(args, " "), err, out)
	}
	for _, r := range recs {
		if r["instance"] == "" || r["app"] == "" {
			t.Errorf("arr %s: record without instance or app: %v", strings.Join(args, " "), r)
		}
	}
	return recs
}

// useBothInstances makes the config hold both real Instances.
func useBothInstances(t *testing.T) {
	t.Helper()
	useConfig(t, realInstanceFor(t, "sonarr"), realInstanceFor(t, "radarr"))
}

// ensureRootFolder makes path a root folder of inst, unless it already is.
// A new Instance has none, and add needs one.
func ensureRootFolder(t *testing.T, inst instanceDef, path string) {
	t.Helper()
	call := func(method string, body []byte) []byte {
		req, err := http.NewRequest(method, inst.url+"/api/v3/rootfolder", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("X-Api-Key", inst.key)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var buf bytes.Buffer
		buf.ReadFrom(resp.Body)
		if resp.StatusCode >= 300 {
			t.Fatalf("%s rootfolder on %s: %s\n%s", method, inst.name, resp.Status, buf.String())
		}
		return buf.Bytes()
	}
	var folders []struct{ Path string }
	if err := json.Unmarshal(call(http.MethodGet, nil), &folders); err != nil {
		t.Fatal(err)
	}
	if slices.ContainsFunc(folders, func(f struct{ Path string }) bool { return strings.TrimRight(f.Path, "/") == path }) {
		return
	}
	call(http.MethodPost, []byte(fmt.Sprintf(`{"path": %q}`, path)))
}

// The read-only tests below run across both Instances. Another test may have
// added a Title first, so they check the shape of the output, not that it is
// empty.

func TestIntegrationQueueAcrossBothApps(t *testing.T) {
	useBothInstances(t)
	records(t, "queue", "--json")
	t.Error("deliberate failure: proves the queue test runs")
}

func TestIntegrationCalendarAcrossBothApps(t *testing.T) {
	useBothInstances(t)
	records(t, "calendar", "--json")

	out, stderr, code := run(t, "calendar", "--list")
	if code != 0 {
		t.Fatalf("calendar --list: exit code %d, stderr: %s\n%s", code, stderr, out)
	}
}

func TestIntegrationListAcrossBothApps(t *testing.T) {
	useBothInstances(t)
	records(t, "list", "--json")
}

// The search and add tests go through the App to its metadata service, so
// they need the internet and fail when that service is down.

func TestIntegrationSearchRadarr(t *testing.T) {
	useConfig(t, realInstanceFor(t, "radarr"))

	recs := records(t, "radarr", "search", "the matrix", "--json")

	if !slices.ContainsFunc(recs, func(r map[string]any) bool { return r["tmdbId"] == float64(604) }) {
		t.Errorf("want a Candidate with TMDB ID 603 (The Matrix), got %v", recs)
	}
}

func TestIntegrationSearchSonarr(t *testing.T) {
	useConfig(t, realInstanceFor(t, "sonarr"))

	recs := records(t, "sonarr", "search", "breaking bad", "--json")

	if !slices.ContainsFunc(recs, func(r map[string]any) bool { return r["tvdbId"] == float64(81190) }) {
		t.Errorf("want a Candidate with TVDB ID 81189 (Breaking Bad), got %v", recs)
	}
}

// The add tests are the only ones that change an Instance, so each keeps its
// steps in one test, in order.

func TestIntegrationAddThenListMovie(t *testing.T) {
	radarr := realInstanceFor(t, "radarr")
	useConfig(t, radarr)
	ensureRootFolder(t, radarr, "/movies")

	out, stderr, code := run(t, "radarr", "add", "--tmdb", "603", "--quality", "Any", "--root", "/movies", "--no-search", "--json")
	if code != 0 {
		t.Fatalf("add: exit code %d, stderr: %s\n%s", code, stderr, out)
	}
	var added struct {
		ID     int    `json:"id"`
		Title  string `json:"title"`
		TmdbID int    `json:"tmdbId"`
	}
	if err := json.Unmarshal([]byte(out), &added); err != nil || added.ID == 0 || added.TmdbID != 604 {
		t.Fatalf("add --json should describe the added Movie, got %+v (%v):\n%s", added, err, out)
	}

	recs := records(t, "radarr", "list", "matrix", "--json")
	if !slices.ContainsFunc(recs, func(r map[string]any) bool { return r["tmdbId"] == float64(604) }) {
		t.Errorf("list should show the added Movie, got %v", recs)
	}

	_, stderr, code = run(t, "radarr", "add", "--tmdb", "603", "--quality", "Any", "--root", "/movies", "--no-search")
	if code == 0 || !strings.Contains(stderr, "already in library") {
		t.Errorf("adding it again should be refused as already in library, got exit %d, stderr %q", code, stderr)
	}
}

func TestIntegrationAddThenListSeries(t *testing.T) {
	sonarr := realInstanceFor(t, "sonarr")
	useConfig(t, sonarr)
	ensureRootFolder(t, sonarr, "/tv")

	out, stderr, code := run(t, "sonarr", "add", "--tvdb", "81189", "--quality", "Any", "--root", "/tv", "--monitor", "none", "--no-search", "--json")
	if code != 0 {
		t.Fatalf("add: exit code %d, stderr: %s\n%s", code, stderr, out)
	}
	var added struct {
		ID     int    `json:"id"`
		Title  string `json:"title"`
		TvdbID int    `json:"tvdbId"`
	}
	if err := json.Unmarshal([]byte(out), &added); err != nil || added.ID == 0 || added.TvdbID != 81189 {
		t.Fatalf("add --json should describe the added Series, got %+v (%v):\n%s", added, err, out)
	}

	recs := records(t, "sonarr", "list", "breaking", "--json")
	if !slices.ContainsFunc(recs, func(r map[string]any) bool { return r["tvdbId"] == float64(81189) }) {
		t.Errorf("list should show the added Series, got %v", recs)
	}

	_, stderr, code = run(t, "sonarr", "add", "--tvdb", "81189", "--quality", "Any", "--root", "/tv", "--monitor", "none", "--no-search")
	if code == 0 || !strings.Contains(stderr, "already in library") {
		t.Errorf("adding it again should be refused as already in library, got exit %d, stderr %q", code, stderr)
	}
}

func TestIntegrationConfigAddSavesSonarr(t *testing.T) {
	sonarr := realInstanceFor(t, "sonarr")
	path := useMissingConfig(t)
	want := config.Instance{Name: "sonarr", App: config.Sonarr, URL: sonarr.url, APIKey: sonarr.key}
	fillForm(t, want)

	out, stderr, code := run(t, "config", "add")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s\n%s", code, stderr, out)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("config after add: %v", err)
	}
	if len(cfg.Instances) != 1 || cfg.Instances[0] != want {
		t.Errorf("Instances %+v, want %+v", cfg.Instances, want)
	}
}

func TestIntegrationConfigAddRefusesSonarrAsRadarr(t *testing.T) {
	sonarr := realInstanceFor(t, "sonarr")
	path := useMissingConfig(t)
	fillForm(t, config.Instance{Name: "radarr", App: config.Radarr, URL: sonarr.url, APIKey: sonarr.key})

	_, stderr, code := run(t, "config", "add")

	if code == 0 || !strings.Contains(stderr, "runs Radarr, not Sonarr") {
		t.Errorf("want a refusal saying it runs Sonarr, not Radarr, got exit %d, stderr %q", code, stderr)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("nothing should be saved, but %s exists (%v)", filepath.Base(path), err)
	}
}
