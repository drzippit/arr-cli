package cli_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSearchShowsRadarrCandidatesWithYearAndTMDBID(t *testing.T) {
	srv := fakeInstance(t, "radarr")
	useConfig(t, instanceDef{"radarr", "radarr", srv.URL, testAPIKey})

	out, stderr, code := run(t, "radarr", "search", "matrix")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	for _, want := range []string{"TMDB", "The Matrix Reloaded (2003)", "604", "The Matrix Resurrections (2021)", "624860"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestSearchMarksSonarrCandidatesAlreadyInLibrary(t *testing.T) {
	srv := fakeInstance(t, "sonarr")
	useConfig(t, instanceDef{"sonarr", "sonarr", srv.URL, testAPIKey})

	out, stderr, code := run(t, "sonarr", "search", "twin", "peaks")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	var inLibrary, notInLibrary string
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.Contains(line, "Twin Peaks (1990)"):
			inLibrary = line
		case strings.Contains(line, "Twin Peaks (2017)"):
			notInLibrary = line
		}
	}
	if !strings.Contains(inLibrary, "70533") || !strings.Contains(inLibrary, "✓ in library") {
		t.Errorf("Twin Peaks (1990) should show TVDB 70533 and be marked in library:\n%s", out)
	}
	if strings.Contains(out, "(2017) (2017)") {
		t.Errorf("a title that already ends in its year should not get it twice:\n%s", out)
	}
	if notInLibrary == "" || strings.Contains(notInLibrary, "in library") {
		t.Errorf("Twin Peaks (2017) should be shown and not marked in library:\n%s", out)
	}
}

func TestSearchJSONIsFlatArrayWithInstanceAppAndInLibrary(t *testing.T) {
	srv := fakeInstance(t, "radarr")
	useConfig(t, instanceDef{"radarr-4k", "radarr", srv.URL, testAPIKey})

	out, stderr, code := run(t, "radarr-4k", "search", "matrix", "--json")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	want := `[
  {
    "instance": "radarr-4k",
    "app": "radarr",
    "title": "The Matrix",
    "year": 1999,
    "tmdbId": 603,
    "inLibrary": true
  },
  {
    "instance": "radarr-4k",
    "app": "radarr",
    "title": "The Matrix Reloaded",
    "year": 2003,
    "tmdbId": 604,
    "inLibrary": false
  },
  {
    "instance": "radarr-4k",
    "app": "radarr",
    "title": "The Matrix Resurrections",
    "year": 2021,
    "tmdbId": 624860,
    "inLibrary": false
  }
]
`
	if out != want {
		t.Errorf("JSON output:\n%s\nwant:\n%s", out, want)
	}
}

func TestSearchSendsQueryToAppLookup(t *testing.T) {
	var gotPath, gotTerm string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotTerm = r.URL.Path, r.URL.Query().Get("term")
		w.Write([]byte("[]"))
	}))
	t.Cleanup(srv.Close)
	useConfig(t, instanceDef{"sonarr", "sonarr", srv.URL, testAPIKey})

	_, stderr, code := run(t, "sonarr", "search", "twin", "peaks")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	if gotPath != "/api/v3/series/lookup" || gotTerm != "twin peaks" {
		t.Errorf("asked %s for term %q, want /api/v3/series/lookup for %q", gotPath, gotTerm, "twin peaks")
	}
}

func TestSearchWithoutQueryFails(t *testing.T) {
	srv := fakeInstance(t, "radarr")
	useConfig(t, instanceDef{"radarr", "radarr", srv.URL, testAPIKey})

	_, _, code := run(t, "radarr", "search")

	if code == 0 {
		t.Error("search without a query should fail")
	}
}
