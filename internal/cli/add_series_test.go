package cli_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// addedSeries is the part of a POST /api/v3/series body the tests check.
type addedSeries struct {
	Title            string `json:"title"`
	TitleSlug        string `json:"titleSlug"`
	Year             int    `json:"year"`
	TvdbID           int    `json:"tvdbId"`
	QualityProfileID int    `json:"qualityProfileId"`
	RootFolderPath   string `json:"rootFolderPath"`
	Monitored        bool   `json:"monitored"`
	SeasonFolder     bool   `json:"seasonFolder"`
	Seasons          []struct {
		SeasonNumber int `json:"seasonNumber"`
	} `json:"seasons"`
	AddOptions struct {
		Monitor                  string `json:"monitor"`
		SearchForMissingEpisodes bool   `json:"searchForMissingEpisodes"`
	} `json:"addOptions"`
}

// fakeSonarrForAdd replays the recorded Sonarr responses and records each
// Series added with POST /api/v3/series.
func fakeSonarrForAdd(t *testing.T) (*httptest.Server, *[]addedSeries) {
	t.Helper()
	h, added := sonarrForAddHandler(t)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv, added
}

func sonarrForAddHandler(t *testing.T) (http.Handler, *[]addedSeries) {
	t.Helper()
	var added []addedSeries
	recorded := recordedHandler(t, "sonarr")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if term, ok := strings.CutPrefix(r.URL.Query().Get("term"), "tvdb:"); ok && r.URL.Path == "/api/v3/series/lookup" {
			lookupByTVDB(t, w, term)
			return
		}
		if r.Method != http.MethodPost {
			recorded.ServeHTTP(w, r)
			return
		}
		if r.URL.Path != "/api/v3/series" || r.Header.Get("X-Api-Key") != testAPIKey {
			t.Errorf("unexpected POST to %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var s addedSeries
		if err := json.Unmarshal(body, &s); err != nil {
			t.Errorf("POST body is not a Series: %v\n%s", err, body)
		}
		added = append(added, s)
		// Sonarr answers with the Series as it is now in the Library.
		var created map[string]any
		json.Unmarshal(body, &created)
		created["id"] = 51
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(created)
	}), &added
}

// lookupByTVDB answers GET /api/v3/series/lookup?term=tvdb:<id> as Sonarr
// does: with a list holding the recorded Candidate with that TVDB ID, which
// keeps its Library ID if it has one, or an empty list.
func lookupByTVDB(t *testing.T, w http.ResponseWriter, tvdbID string) {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", "sonarr", "series_lookup.json"))
	if err != nil {
		t.Fatal(err)
	}
	var candidates []map[string]any
	if err := json.Unmarshal(body, &candidates); err != nil {
		t.Fatal(err)
	}
	found := []map[string]any{}
	for _, c := range candidates {
		if jsonNumber(c["tvdbId"]) == tvdbID {
			found = append(found, c)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(found)
}

func jsonNumber(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func TestAddSeriesByTVDBIDAddsMonitoredSeriesAndSearches(t *testing.T) {
	srv, added := fakeSonarrForAdd(t)
	useConfig(t, instanceDef{"sonarr", "sonarr", srv.URL, testAPIKey})

	out, stderr, code := run(t, "sonarr", "add", "--tvdb", "301474", "--quality", "HD-1080p", "--root", "/tv", "--monitor", "future")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	if len(*added) != 1 {
		t.Fatalf("added %d Series, want 1", len(*added))
	}
	got := (*added)[0]
	if got.TvdbID != 301474 || got.Title != "Twin Peaks (2017)" || got.Year != 2017 || got.TitleSlug != "twin-peaks-2017" {
		t.Errorf("added %+v, want Twin Peaks (2017), TVDB 301474", got)
	}
	if got.QualityProfileID != 4 || got.RootFolderPath != "/tv" {
		t.Errorf("quality profile %d and root %q, want 4 (HD-1080p) and /tv", got.QualityProfileID, got.RootFolderPath)
	}
	if !got.Monitored || !got.SeasonFolder || got.AddOptions.Monitor != "future" || !got.AddOptions.SearchForMissingEpisodes {
		t.Errorf("Series should be monitored in season folders with monitor future and searched for, got %+v", got)
	}
	if len(got.Seasons) != 2 {
		t.Errorf("Series should be added with the 2 seasons the lookup gave, got %+v", got.Seasons)
	}
	if want := "Added Twin Peaks (2017) to sonarr"; !strings.Contains(out, want) {
		t.Errorf("output missing %q:\n%s", want, out)
	}
}

func TestAddSeriesQueryInTerminalPicksCandidateQualityAndRoot(t *testing.T) {
	srv, added := fakeSonarrForAdd(t)
	useConfig(t, instanceDef{"sonarr", "sonarr", srv.URL, testAPIKey})
	shown := answerPrompts(t, map[string]string{
		"Which Series?":          "2017",
		"Which quality profile?": "WEB-2160p",
		"Which root folder?":     "/anime",
	})

	_, stderr, code := run(t, "sonarr", "add", "twin", "peaks", "--monitor", "all")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	wantSeries := []string{
		"Twin Peaks (1990) · TVDB 70533 · ✓ in library",
		"Twin Peaks (2017) · TVDB 301474",
	}
	if len(*shown) == 0 || !slices.Equal((*shown)[0].options, wantSeries) {
		t.Errorf("Series picker showed %v, want %v", *shown, wantSeries)
	}
	if len(*added) != 1 {
		t.Fatalf("added %d Series, want 1", len(*added))
	}
	got := (*added)[0]
	if got.TvdbID != 301474 || got.QualityProfileID != 6 || got.RootFolderPath != "/anime" {
		t.Errorf("added %+v, want TVDB 301474 with profile 6 (WEB-2160p) in /anime", got)
	}
}

func TestAddSeriesQueryFromScriptWithSeveralCandidatesFailsAndListsThem(t *testing.T) {
	srv, added := fakeSonarrForAdd(t)
	useConfig(t, instanceDef{"sonarr", "sonarr", srv.URL, testAPIKey})

	_, stderr, code := run(t, "sonarr", "add", "twin peaks", "--quality", "Any", "--root", "/tv", "--monitor", "all")

	if code == 0 {
		t.Fatal("add should fail when several Candidates match")
	}
	for _, want := range []string{"Twin Peaks (1990)", "70533", "Twin Peaks (2017)", "301474", "2 Series match", "--tvdb"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr missing %q:\n%s", want, stderr)
		}
	}
	if len(*added) != 0 {
		t.Errorf("added %d Series, want none", len(*added))
	}
}

func TestAddSeriesInTerminalWithoutMonitorAsksWithAllPreselected(t *testing.T) {
	srv, added := fakeSonarrForAdd(t)
	useConfig(t, instanceDef{"sonarr", "sonarr", srv.URL, testAPIKey})
	shown := answerPrompts(t, map[string]string{"Which episodes to monitor?": "firstSeason"})

	_, stderr, code := run(t, "sonarr", "add", "--tvdb", "301474", "--quality", "Any", "--root", "/tv")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	want := []string{"all", "future", "missing", "existing", "firstSeason", "latestSeason", "none"}
	if len(*shown) != 1 || !slices.Equal((*shown)[0].options, want) || (*shown)[0].selected != 0 {
		t.Errorf("monitor picker showed %+v, want %v with all pre-selected", *shown, want)
	}
	if len(*added) != 1 || (*added)[0].AddOptions.Monitor != "firstSeason" {
		t.Errorf("added %+v, want monitor firstSeason", *added)
	}
}

// useSonarrConfigWithMonitor configures one Sonarr Instance at url with
// defaults for adding, including monitor.
func useSonarrConfigWithMonitor(t *testing.T, url, monitor string) {
	t.Helper()
	useConfigTOML(t, fmt.Sprintf(`[[instance]]
name = "sonarr"
app = "sonarr"
url = %q
api_key = %q
quality_profile = "Any"
root_folder = "/tv"
monitor = %q
`, url, testAPIKey, monitor))
}

func TestAddSeriesInTerminalPreselectsConfigMonitor(t *testing.T) {
	srv, added := fakeSonarrForAdd(t)
	useSonarrConfigWithMonitor(t, srv.URL, "latestseason")
	shown := answerPrompts(t, map[string]string{"Which episodes to monitor?": "missing"})

	_, stderr, code := run(t, "sonarr", "add", "--tvdb", "301474")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	if len(*shown) != 1 || (*shown)[0].selected != 5 {
		t.Errorf("monitor picker showed %+v, want latestSeason (index 5) pre-selected", *shown)
	}
	if len(*added) != 1 || (*added)[0].AddOptions.Monitor != "missing" {
		t.Errorf("added %+v, want monitor missing as picked", *added)
	}
}

func TestAddSeriesFromScriptUsesConfigMonitor(t *testing.T) {
	srv, added := fakeSonarrForAdd(t)
	useSonarrConfigWithMonitor(t, srv.URL, "existing")

	_, stderr, code := run(t, "sonarr", "add", "--tvdb", "301474")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	if len(*added) != 1 || (*added)[0].AddOptions.Monitor != "existing" {
		t.Errorf("added %+v, want monitor existing from the config", *added)
	}
}

func TestAddSeriesMonitorFlagOverridesConfig(t *testing.T) {
	srv, added := fakeSonarrForAdd(t)
	useSonarrConfigWithMonitor(t, srv.URL, "existing")

	_, stderr, code := run(t, "sonarr", "add", "--tvdb", "301474", "--monitor", "None")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	if len(*added) != 1 || (*added)[0].AddOptions.Monitor != "none" {
		t.Errorf("added %+v, want monitor none from the flag", *added)
	}
}

func TestAddSeriesFromScriptWithoutMonitorFailsClearly(t *testing.T) {
	srv, added := fakeSonarrForAdd(t)
	useConfig(t, instanceDef{"sonarr", "sonarr", srv.URL, testAPIKey})

	_, stderr, code := run(t, "sonarr", "add", "--tvdb", "301474", "--quality", "Any", "--root", "/tv")

	if code == 0 {
		t.Fatal("add should fail without a monitor option")
	}
	for _, want := range []string{"no monitor option given", "--monitor", "monitor", "firstSeason"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr missing %q:\n%s", want, stderr)
		}
	}
	if len(*added) != 0 {
		t.Errorf("added %d Series, want none", len(*added))
	}
}

func TestAddSeriesUnknownMonitorFailsBeforeLookingUp(t *testing.T) {
	srv, added := fakeSonarrForAdd(t)
	useConfig(t, instanceDef{"sonarr", "sonarr", srv.URL, testAPIKey})

	_, stderr, code := run(t, "sonarr", "add", "--tvdb", "301474", "--quality", "Any", "--root", "/tv", "--monitor", "pilot-only")

	if code == 0 || !strings.Contains(stderr, `"pilot-only"`) || !strings.Contains(stderr, "all, future, missing, existing, firstSeason, latestSeason, none") {
		t.Errorf("want failure naming the value and the valid ones, got exit %d, stderr %q", code, stderr)
	}
	if len(*added) != 0 {
		t.Errorf("added %d Series, want none", len(*added))
	}
}

func TestAddSeriesCandidateAlreadyInLibraryFails(t *testing.T) {
	srv, added := fakeSonarrForAdd(t)
	useConfig(t, instanceDef{"sonarr", "sonarr", srv.URL, testAPIKey})
	answerPrompts(t, map[string]string{"Which Series?": "Twin Peaks (1990)"})

	_, stderr, code := run(t, "sonarr", "add", "twin peaks", "--monitor", "all")

	if code == 0 || !strings.Contains(stderr, "Twin Peaks (1990) is already in library") {
		t.Errorf("want failure saying already in library, got exit %d, stderr %q", code, stderr)
	}
	if len(*added) != 0 {
		t.Errorf("added %d Series, want none", len(*added))
	}
}

func TestAddSeriesByTVDBIDOfSeriesAlreadyInLibraryFails(t *testing.T) {
	srv, added := fakeSonarrForAdd(t)
	useConfig(t, instanceDef{"sonarr", "sonarr", srv.URL, testAPIKey})

	_, stderr, code := run(t, "sonarr", "add", "--tvdb", "70533", "--quality", "Any", "--root", "/tv", "--monitor", "all")

	if code == 0 || !strings.Contains(stderr, "Twin Peaks (1990) is already in library") {
		t.Errorf("want failure saying Twin Peaks is already in library, got exit %d, stderr %q", code, stderr)
	}
	if len(*added) != 0 {
		t.Errorf("added %d Series, want none", len(*added))
	}
}

func TestAddSeriesByUnknownTVDBIDFails(t *testing.T) {
	srv, added := fakeSonarrForAdd(t)
	useConfig(t, instanceDef{"sonarr", "sonarr", srv.URL, testAPIKey})

	_, stderr, code := run(t, "sonarr", "add", "--tvdb", "999999", "--quality", "Any", "--root", "/tv", "--monitor", "all")

	if code == 0 || !strings.Contains(stderr, "no Series with TVDB ID 999999") {
		t.Errorf("want failure naming the unknown TVDB ID, got exit %d, stderr %q", code, stderr)
	}
	if len(*added) != 0 {
		t.Errorf("added %d Series, want none", len(*added))
	}
}

func TestAddSeriesNoSearchAddsWithoutSearching(t *testing.T) {
	srv, added := fakeSonarrForAdd(t)
	useConfig(t, instanceDef{"sonarr", "sonarr", srv.URL, testAPIKey})

	out, stderr, code := run(t, "sonarr", "add", "--tvdb", "301474", "--quality", "Any", "--root", "/tv", "--monitor", "all", "--no-search")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	if len(*added) != 1 || (*added)[0].AddOptions.SearchForMissingEpisodes || !(*added)[0].Monitored {
		t.Errorf("added %+v, want one monitored Series with no search", *added)
	}
	if !strings.Contains(out, "without searching") {
		t.Errorf("output should say no search started:\n%s", out)
	}
}

func TestAddSeriesRefusesSonarrOlderThanSupported(t *testing.T) {
	h, added := sonarrForAddHandler(t)
	srv := httptest.NewServer(runningVersion("Sonarr", "3.0.10.1567", h))
	t.Cleanup(srv.Close)
	useConfig(t, instanceDef{"sonarr", "sonarr", srv.URL, testAPIKey})

	_, stderr, code := run(t, "sonarr", "add", "--tvdb", "301474", "--quality", "Any", "--root", "/tv", "--monitor", "all")

	if code == 0 || !strings.Contains(stderr, "sonarr runs Sonarr 3.0.10.1567; arr-cli supports Sonarr 4 and later") {
		t.Errorf("want failure naming the unsupported version, got exit %d, stderr %q", code, stderr)
	}
	if len(*added) != 0 {
		t.Errorf("added %d Series, want none", len(*added))
	}
}

func TestAddSeriesJSONPrintsTheAddedSeries(t *testing.T) {
	srv, _ := fakeSonarrForAdd(t)
	useConfig(t, instanceDef{"sonarr", "sonarr", srv.URL, testAPIKey})

	out, stderr, code := run(t, "sonarr", "add", "--tvdb", "301474", "--quality", "hd-1080p", "--root", "/tv/", "--monitor", "firstseason", "--json")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	want := `{
  "instance": "sonarr",
  "app": "sonarr",
  "id": 51,
  "title": "Twin Peaks (2017)",
  "year": 2017,
  "tvdbId": 301474,
  "qualityProfile": "HD-1080p",
  "rootFolder": "/tv",
  "monitor": "firstSeason",
  "monitored": true,
  "searching": true
}
`
	if out != want {
		t.Errorf("JSON output:\n%s\nwant:\n%s", out, want)
	}
}

func TestAddSeriesUnknownConfigMonitorFailsBeforeLookingUp(t *testing.T) {
	var requests []string
	h, added := sonarrForAddHandler(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.Path)
		h.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	useSonarrConfigWithMonitor(t, srv.URL, "weekly")

	_, stderr, code := run(t, "sonarr", "add", "--tvdb", "301474")

	if code == 0 || !strings.Contains(stderr, `monitor option "weekly" is not one of`) {
		t.Errorf("want failure naming the bad config value, got exit %d, stderr %q", code, stderr)
	}
	if len(requests) != 0 || len(*added) != 0 {
		t.Errorf("made requests %v and added %d Series, want none", requests, len(*added))
	}
}
