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

	"github.com/drzippit/arr-cli/internal/cli"
)

// addedMovie is the part of a POST /api/v3/movie body the tests check.
type addedMovie struct {
	Title               string `json:"title"`
	Year                int    `json:"year"`
	TmdbID              int    `json:"tmdbId"`
	QualityProfileID    int    `json:"qualityProfileId"`
	RootFolderPath      string `json:"rootFolderPath"`
	Monitored           bool   `json:"monitored"`
	MinimumAvailability string `json:"minimumAvailability"`
	AddOptions          struct {
		SearchForMovie bool `json:"searchForMovie"`
	} `json:"addOptions"`
}

// fakeRadarrForAdd replays the recorded Radarr responses and records each
// Movie added with POST /api/v3/movie.
func fakeRadarrForAdd(t *testing.T) (*httptest.Server, *[]addedMovie) {
	t.Helper()
	h, added := radarrForAddHandler(t)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv, added
}

func radarrForAddHandler(t *testing.T) (http.Handler, *[]addedMovie) {
	t.Helper()
	var added []addedMovie
	recorded := recordedHandler(t, "radarr")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v3/movie/lookup/tmdb" {
			lookupByTMDB(t, w, r)
			return
		}
		if r.Method != http.MethodPost {
			recorded.ServeHTTP(w, r)
			return
		}
		if r.URL.Path != "/api/v3/movie" || r.Header.Get("X-Api-Key") != testAPIKey {
			t.Errorf("unexpected POST to %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var m addedMovie
		if err := json.Unmarshal(body, &m); err != nil {
			t.Errorf("POST body is not a Movie: %v\n%s", err, body)
		}
		added = append(added, m)
		// Radarr answers with the Movie as it is now in the Library.
		var created map[string]any
		json.Unmarshal(body, &created)
		created["id"] = 42
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(created)
	}), &added
}

// lookupByTMDB answers GET /api/v3/movie/lookup/tmdb as Radarr does: with the
// recorded Candidate that has the TMDB ID asked for, built afresh from TMDB,
// so without the ID it has in the Library, or 404 when there is none.
func lookupByTMDB(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", "radarr", "movie_lookup.json"))
	if err != nil {
		t.Fatal(err)
	}
	var candidates []map[string]any
	if err := json.Unmarshal(body, &candidates); err != nil {
		t.Fatal(err)
	}
	for _, c := range candidates {
		if fmt.Sprint(c["tmdbId"]) == r.URL.Query().Get("tmdbId") {
			delete(c, "id")
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(c)
			return
		}
	}
	http.NotFound(w, r)
}

func TestAddByTMDBIDAddsMonitoredMovieAndSearches(t *testing.T) {
	srv, added := fakeRadarrForAdd(t)
	useConfig(t, instanceDef{"radarr", "radarr", srv.URL, testAPIKey})

	out, stderr, code := run(t, "radarr", "add", "--tmdb", "604", "--quality", "HD-1080p", "--root", "/movies")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	if len(*added) != 1 {
		t.Fatalf("added %d Movies, want 1", len(*added))
	}
	got := (*added)[0]
	if got.TmdbID != 604 || got.Title != "The Matrix Reloaded" || got.Year != 2003 {
		t.Errorf("added %+v, want The Matrix Reloaded (2003), TMDB 604", got)
	}
	if got.QualityProfileID != 4 || got.RootFolderPath != "/movies" {
		t.Errorf("quality profile %d and root %q, want 4 (HD-1080p) and /movies", got.QualityProfileID, got.RootFolderPath)
	}
	if !got.Monitored || !got.AddOptions.SearchForMovie {
		t.Errorf("Movie should be monitored and searched for, got %+v", got)
	}
	if want := "Added The Matrix Reloaded (2003) to radarr"; !strings.Contains(out, want) {
		t.Errorf("output missing %q:\n%s", want, out)
	}
}

// prompt is one picker the CLI showed.
type prompt struct {
	title   string
	options []string
}

// answerPrompts makes the CLI act as if it runs in a terminal and answers
// each picker by choosing the option containing the text answers gives for
// its title. It returns the pickers shown.
func answerPrompts(t *testing.T, answers map[string]string) *[]prompt {
	t.Helper()
	var shown []prompt
	cli.SetChooser(t, func(title string, options []string) (int, error) {
		shown = append(shown, prompt{title, options})
		want, ok := answers[title]
		if !ok {
			t.Errorf("unexpected picker %q with %v", title, options)
			return 0, fmt.Errorf("unexpected picker %q", title)
		}
		i := slices.IndexFunc(options, func(o string) bool { return strings.Contains(o, want) })
		if i < 0 {
			t.Fatalf("picker %q has no option containing %q: %v", title, want, options)
		}
		return i, nil
	})
	return &shown
}

func TestAddQueryInTerminalPicksCandidateQualityAndRoot(t *testing.T) {
	srv, added := fakeRadarrForAdd(t)
	useConfig(t, instanceDef{"radarr", "radarr", srv.URL, testAPIKey})
	shown := answerPrompts(t, map[string]string{
		"Which Movie?":           "Resurrections",
		"Which quality profile?": "Ultra-HD",
		"Which root folder?":     "/movies-4k",
	})

	_, stderr, code := run(t, "radarr", "add", "matrix")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	wantMovies := []string{
		"The Matrix (1999) · TMDB 603 · ✓ in library",
		"The Matrix Reloaded (2003) · TMDB 604",
		"The Matrix Resurrections (2021) · TMDB 624860",
	}
	if len(*shown) == 0 || !slices.Equal((*shown)[0].options, wantMovies) {
		t.Errorf("Movie picker showed %v, want %v", *shown, wantMovies)
	}
	if len(*added) != 1 {
		t.Fatalf("added %d Movies, want 1", len(*added))
	}
	got := (*added)[0]
	if got.TmdbID != 624860 || got.QualityProfileID != 5 || got.RootFolderPath != "/movies-4k" {
		t.Errorf("added %+v, want TMDB 624860 with profile 5 (Ultra-HD) in /movies-4k", got)
	}
}

func TestAddQueryFromScriptWithSeveralCandidatesFailsAndListsThem(t *testing.T) {
	srv, added := fakeRadarrForAdd(t)
	useConfig(t, instanceDef{"radarr", "radarr", srv.URL, testAPIKey})

	_, stderr, code := run(t, "radarr", "add", "matrix", "--quality", "Any", "--root", "/movies")

	if code == 0 {
		t.Fatal("add should fail when several Candidates match")
	}
	for _, want := range []string{"The Matrix Reloaded (2003)", "604", "The Matrix Resurrections (2021)", "624860", "3 Movies match", "--tmdb"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr missing %q:\n%s", want, stderr)
		}
	}
	if len(*added) != 0 {
		t.Errorf("added %d Movies, want none", len(*added))
	}
}

func TestAddUsesInstanceConfigDefaultsForQualityAndRoot(t *testing.T) {
	srv, added := fakeRadarrForAdd(t)
	useConfigTOML(t, fmt.Sprintf(`[[instance]]
name = "radarr"
app = "radarr"
url = %q
api_key = %q
quality_profile = "ultra-hd"
root_folder = "/movies-4k/"
`, srv.URL, testAPIKey))

	_, stderr, code := run(t, "radarr", "add", "--tmdb", "604")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	if len(*added) != 1 || (*added)[0].QualityProfileID != 5 || (*added)[0].RootFolderPath != "/movies-4k" {
		t.Errorf("added %+v, want profile 5 (Ultra-HD) in /movies-4k", *added)
	}
}

func TestAddFlagsOverrideInstanceConfigDefaults(t *testing.T) {
	srv, added := fakeRadarrForAdd(t)
	useConfigTOML(t, fmt.Sprintf(`[[instance]]
name = "radarr"
app = "radarr"
url = %q
api_key = %q
quality_profile = "Ultra-HD"
root_folder = "/movies-4k"
`, srv.URL, testAPIKey))

	_, stderr, code := run(t, "radarr", "add", "--tmdb", "604", "--quality", "Any", "--root", "/movies")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	if len(*added) != 1 || (*added)[0].QualityProfileID != 1 || (*added)[0].RootFolderPath != "/movies" {
		t.Errorf("added %+v, want profile 1 (Any) in /movies", *added)
	}
}

func TestAddFromScriptWithoutQualityProfileFailsClearly(t *testing.T) {
	srv, added := fakeRadarrForAdd(t)
	useConfig(t, instanceDef{"radarr", "radarr", srv.URL, testAPIKey})

	_, stderr, code := run(t, "radarr", "add", "--tmdb", "604", "--root", "/movies")

	if code == 0 {
		t.Fatal("add should fail without a quality profile")
	}
	for _, want := range []string{"no quality profile given", "--quality", "quality_profile", "HD-1080p"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr missing %q:\n%s", want, stderr)
		}
	}
	if len(*added) != 0 {
		t.Errorf("added %d Movies, want none", len(*added))
	}
}

func TestAddFromScriptWithoutRootFolderFailsClearly(t *testing.T) {
	srv, added := fakeRadarrForAdd(t)
	useConfig(t, instanceDef{"radarr", "radarr", srv.URL, testAPIKey})

	_, stderr, code := run(t, "radarr", "add", "--tmdb", "604", "--quality", "Any")

	if code == 0 {
		t.Fatal("add should fail without a root folder")
	}
	for _, want := range []string{"no root folder given", "--root", "root_folder", "/movies-4k"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr missing %q:\n%s", want, stderr)
		}
	}
	if len(*added) != 0 {
		t.Errorf("added %d Movies, want none", len(*added))
	}
}

func TestAddUnknownQualityProfileFailsAndListsAvailable(t *testing.T) {
	srv, added := fakeRadarrForAdd(t)
	useConfig(t, instanceDef{"radarr", "radarr", srv.URL, testAPIKey})

	_, stderr, code := run(t, "radarr", "add", "--tmdb", "604", "--quality", "4K", "--root", "/movies")

	if code == 0 || !strings.Contains(stderr, `no quality profile "4K"`) || !strings.Contains(stderr, "Any, HD-1080p, Ultra-HD") {
		t.Errorf("want failure naming the unknown profile and the available ones, got exit %d, stderr %q", code, stderr)
	}
	if len(*added) != 0 {
		t.Errorf("added %d Movies, want none", len(*added))
	}
}

func TestAddNoSearchAddsWithoutSearching(t *testing.T) {
	srv, added := fakeRadarrForAdd(t)
	useConfig(t, instanceDef{"radarr", "radarr", srv.URL, testAPIKey})

	out, stderr, code := run(t, "radarr", "add", "--tmdb", "604", "--quality", "Any", "--root", "/movies", "--no-search")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	if len(*added) != 1 || (*added)[0].AddOptions.SearchForMovie || !(*added)[0].Monitored {
		t.Errorf("added %+v, want one monitored Movie with no search", *added)
	}
	if !strings.Contains(out, "without searching") {
		t.Errorf("output should say no search started:\n%s", out)
	}
}

func TestAddCandidateAlreadyInLibraryFails(t *testing.T) {
	srv, added := fakeRadarrForAdd(t)
	useConfig(t, instanceDef{"radarr", "radarr", srv.URL, testAPIKey})
	answerPrompts(t, map[string]string{"Which Movie?": "The Matrix (1999)"})

	_, stderr, code := run(t, "radarr", "add", "matrix")

	if code == 0 || !strings.Contains(stderr, "already in library") {
		t.Errorf("want failure saying already in library, got exit %d, stderr %q", code, stderr)
	}
	if len(*added) != 0 {
		t.Errorf("added %d Movies, want none", len(*added))
	}
}

func TestAddIsOnlyForRadarrInstancesForNow(t *testing.T) {
	srv := fakeInstance(t, "sonarr")
	useConfig(t, instanceDef{"sonarr", "sonarr", srv.URL, testAPIKey})

	_, stderr, code := run(t, "sonarr", "add", "--tmdb", "604")

	if code == 0 || !strings.Contains(stderr, "unknown") {
		t.Errorf("add on a Sonarr Instance should be unknown until Series can be added, got exit %d, stderr %q", code, stderr)
	}
}

func TestAddByTMDBIDOfMovieAlreadyInLibraryFails(t *testing.T) {
	srv, added := fakeRadarrForAdd(t)
	useConfig(t, instanceDef{"radarr", "radarr", srv.URL, testAPIKey})

	_, stderr, code := run(t, "radarr", "add", "--tmdb", "603", "--quality", "Any", "--root", "/movies")

	if code == 0 || !strings.Contains(stderr, "The Matrix (1999) is already in library") {
		t.Errorf("want failure saying The Matrix is already in library, got exit %d, stderr %q", code, stderr)
	}
	if len(*added) != 0 {
		t.Errorf("added %d Movies, want none", len(*added))
	}
}

func TestAddByUnknownTMDBIDFails(t *testing.T) {
	srv, added := fakeRadarrForAdd(t)
	useConfig(t, instanceDef{"radarr", "radarr", srv.URL, testAPIKey})

	_, stderr, code := run(t, "radarr", "add", "--tmdb", "999999", "--quality", "Any", "--root", "/movies")

	if code == 0 || !strings.Contains(stderr, "no Movie with TMDB ID 999999") {
		t.Errorf("want failure naming the unknown TMDB ID, got exit %d, stderr %q", code, stderr)
	}
	if len(*added) != 0 {
		t.Errorf("added %d Movies, want none", len(*added))
	}
}

func TestAddJSONPrintsTheAddedMovie(t *testing.T) {
	srv, _ := fakeRadarrForAdd(t)
	useConfig(t, instanceDef{"radarr", "radarr", srv.URL, testAPIKey})

	out, stderr, code := run(t, "radarr", "add", "--tmdb", "604", "--quality", "hd-1080p", "--root", "/movies/", "--json")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	want := `{
  "instance": "radarr",
  "app": "radarr",
  "id": 42,
  "title": "The Matrix Reloaded",
  "year": 2003,
  "tmdbId": 604,
  "qualityProfile": "HD-1080p",
  "rootFolder": "/movies",
  "minimumAvailability": "released",
  "monitored": true,
  "searching": true
}
`
	if out != want {
		t.Errorf("JSON output:\n%s\nwant:\n%s", out, want)
	}
}

func TestAddMinimumAvailabilityDefaultsToReleased(t *testing.T) {
	srv, added := fakeRadarrForAdd(t)
	useConfig(t, instanceDef{"radarr", "radarr", srv.URL, testAPIKey})

	_, stderr, code := run(t, "radarr", "add", "--tmdb", "604", "--quality", "Any", "--root", "/movies")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	if got := (*added)[0].MinimumAvailability; got != "released" {
		t.Errorf("minimum availability %q, want released", got)
	}
}

func TestAddMinimumAvailabilityComesFromFlagThenConfig(t *testing.T) {
	srv, added := fakeRadarrForAdd(t)
	useConfigTOML(t, fmt.Sprintf(`[[instance]]
name = "radarr"
app = "radarr"
url = %q
api_key = %q
quality_profile = "Any"
root_folder = "/movies"
minimum_availability = "announced"
`, srv.URL, testAPIKey))

	_, stderr, code := run(t, "radarr", "add", "--tmdb", "604")
	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	_, stderr, code = run(t, "radarr", "add", "--tmdb", "624860", "--availability", "incinemas")
	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}

	if len(*added) != 2 || (*added)[0].MinimumAvailability != "announced" || (*added)[1].MinimumAvailability != "inCinemas" {
		t.Errorf("added %+v, want announced from the config, then inCinemas from the flag", *added)
	}
}

func TestAddUnknownMinimumAvailabilityFails(t *testing.T) {
	srv, added := fakeRadarrForAdd(t)
	useConfig(t, instanceDef{"radarr", "radarr", srv.URL, testAPIKey})

	_, stderr, code := run(t, "radarr", "add", "--tmdb", "604", "--quality", "Any", "--root", "/movies", "--availability", "soon")

	if code == 0 || !strings.Contains(stderr, `"soon"`) || !strings.Contains(stderr, "announced, inCinemas, released") {
		t.Errorf("want failure naming the value and the valid ones, got exit %d, stderr %q", code, stderr)
	}
	if len(*added) != 0 {
		t.Errorf("added %d Movies, want none", len(*added))
	}
}

func TestAddRefusesRadarrOlderThanSupported(t *testing.T) {
	h, added := radarrForAddHandler(t)
	srv := httptest.NewServer(runningVersion("Radarr", "4.7.5.7809", h))
	t.Cleanup(srv.Close)
	useConfig(t, instanceDef{"radarr", "radarr", srv.URL, testAPIKey})

	_, stderr, code := run(t, "radarr", "add", "--tmdb", "604", "--quality", "Any", "--root", "/movies")

	if code == 0 || !strings.Contains(stderr, "radarr runs Radarr 4.7.5.7809; arr-cli supports Radarr 5 and later") {
		t.Errorf("want failure naming the unsupported version, got exit %d, stderr %q", code, stderr)
	}
	if len(*added) != 0 {
		t.Errorf("added %d Movies, want none", len(*added))
	}
}
