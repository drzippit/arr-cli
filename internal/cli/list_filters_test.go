package cli_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// assertTitles fails unless out mentions every one of want and none of the
// others.
func assertTitles(t *testing.T, out string, want []string, others ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(out, w) {
			t.Errorf("output missing %q:\n%s", w, out)
		}
	}
	for _, o := range others {
		if strings.Contains(out, o) {
			t.Errorf("output should not show %q:\n%s", o, out)
		}
	}
}

func TestListMissingShowsSeriesWithAiredMonitoredEpisodesWithoutFiles(t *testing.T) {
	srv := fakeInstance(t, "sonarr")
	useConfig(t, instanceDef{"sonarr", "sonarr", srv.URL, testAPIKey})

	out, stderr, code := run(t, "sonarr", "list", "--missing")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	assertTitles(t, out, []string{"Shōgun"}, "Severance", "Twin Peaks")
}

func TestListMissingShowsAvailableMoviesWithoutAFile(t *testing.T) {
	srv := fakeInstance(t, "radarr")
	useConfig(t, instanceDef{"radarr", "radarr", srv.URL, testAPIKey})

	out, stderr, code := run(t, "radarr", "list", "--missing")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	// The Matrix has its file; Project Hail Mary is not monitored.
	assertTitles(t, out, []string{"Good Will Hunting"}, "The Matrix", "Project Hail Mary")
}

func TestListMissingLeavesOutAMovieThatIsNotOutYet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v3/movie":
			fmt.Fprint(w, `[{"title": "Dune: Part Three", "year": 2026, "status": "announced", "monitored": true,
				"hasFile": false, "isAvailable": false, "tmdbId": 1170608, "qualityProfileId": 4}]`)
		default:
			recordedHandler(t, "radarr").ServeHTTP(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	useConfig(t, instanceDef{"radarr", "radarr", srv.URL, testAPIKey})

	out, stderr, code := run(t, "radarr", "list", "--missing", "--json")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	if strings.TrimSpace(out) != "[]" {
		t.Errorf("a Movie that is not out yet is not Missing:\n%s", out)
	}
}

func TestListMonitoredAndUnmonitored(t *testing.T) {
	srv := fakeInstance(t, "sonarr")
	useConfig(t, instanceDef{"sonarr", "sonarr", srv.URL, testAPIKey})

	out, _, code := run(t, "sonarr", "list", "--unmonitored")
	if code != 0 {
		t.Fatalf("--unmonitored: exit code %d", code)
	}
	assertTitles(t, out, []string{"Twin Peaks"}, "Shōgun", "Severance")

	out, _, code = run(t, "sonarr", "list", "--monitored")
	if code != 0 {
		t.Fatalf("--monitored: exit code %d", code)
	}
	assertTitles(t, out, []string{"Shōgun", "Severance"}, "Twin Peaks")

	_, stderr, code := run(t, "sonarr", "list", "--monitored", "--unmonitored")
	if code != 1 || !strings.Contains(stderr, "--monitored and --unmonitored") {
		t.Errorf("both: exit code %d, stderr %q", code, stderr)
	}
}

func TestListStatusUsesTheAppsOwnValues(t *testing.T) {
	sonarr, radarr := fakeInstance(t, "sonarr"), fakeInstance(t, "radarr")
	useConfig(t,
		instanceDef{"sonarr", "sonarr", sonarr.URL, testAPIKey},
		instanceDef{"radarr", "radarr", radarr.URL, testAPIKey},
	)

	out, stderr, code := run(t, "list", "--status", "ended")
	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	assertTitles(t, out, []string{"Twin Peaks"}, "Shōgun", "Severance", "The Matrix")

	_, stderr, code = run(t, "radarr", "list", "--status", "ended")
	if code != 1 || !strings.Contains(stderr, `"ended"`) || !strings.Contains(stderr, "announced, inCinemas, released") {
		t.Errorf("a status Radarr doesn't have: exit code %d, stderr %q", code, stderr)
	}
}

func TestListQualityMatchesTheProfileByNameIgnoringCase(t *testing.T) {
	srv := fakeInstance(t, "sonarr")
	useConfig(t, instanceDef{"sonarr", "sonarr", srv.URL, testAPIKey})

	out, stderr, code := run(t, "sonarr", "list", "--quality", "hd-1080p", "--json")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	assertTitles(t, out, []string{"Severance", "Twin Peaks", `"qualityProfile": "HD-1080p"`}, "Shōgun")

	_, stderr, code = run(t, "sonarr", "list", "--quality", "4K")
	if code != 1 || !strings.Contains(stderr, `"4K"`) || !strings.Contains(stderr, "Any, HD-1080p, WEB-2160p") {
		t.Errorf("an unknown profile: exit code %d, stderr %q", code, stderr)
	}
}

func TestListFiltersCombineWithEachOtherAndTheQuery(t *testing.T) {
	sonarr, radarr := fakeInstance(t, "sonarr"), fakeInstance(t, "radarr")
	useConfig(t,
		instanceDef{"sonarr", "sonarr", sonarr.URL, testAPIKey},
		instanceDef{"radarr", "radarr", radarr.URL, testAPIKey},
	)

	out, stderr, code := run(t, "list", "--monitored", "--quality", "HD-1080p")
	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	assertTitles(t, out, []string{"Severance", "The Matrix"}, "Twin Peaks", "Shōgun", "Good Will Hunting", "Project Hail Mary")

	out, _, _ = run(t, "list", "matrix", "--monitored", "--quality", "HD-1080p")
	assertTitles(t, out, []string{"The Matrix"}, "Severance")
}
