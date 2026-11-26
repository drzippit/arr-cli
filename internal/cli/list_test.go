package cli_test

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestListShowsEveryMovieInRadarrLibrary(t *testing.T) {
	srv := fakeInstance(t, "radarr")
	useConfig(t, instanceDef{"radarr", "radarr", srv.URL, testAPIKey})

	out, stderr, code := run(t, "radarr", "list")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	for _, want := range []string{"TITLE", "Good Will Hunting (1997)", "The Matrix (1999)", "Project Hail Mary (2026)"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestListWithQueryShowsOnlyMatchingTitlesIgnoringCase(t *testing.T) {
	srv := fakeInstance(t, "radarr")
	useConfig(t, instanceDef{"radarr", "radarr", srv.URL, testAPIKey})

	out, stderr, code := run(t, "radarr", "list", "good WILL hunting")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	if !strings.Contains(out, "Good Will Hunting (1997)") {
		t.Errorf("output missing the matching Movie:\n%s", out)
	}
	for _, other := range []string{"The Matrix", "Project Hail Mary"} {
		if strings.Contains(out, other) {
			t.Errorf("output should not show %q:\n%s", other, out)
		}
	}
}

func TestListWithQueryFiltersSonarrSeries(t *testing.T) {
	srv := fakeInstance(t, "sonarr")
	useConfig(t, instanceDef{"sonarr", "sonarr", srv.URL, testAPIKey})

	out, stderr, code := run(t, "sonarr", "list", "PEAKS")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	if !strings.Contains(out, "Twin Peaks (1990)") {
		t.Errorf("output missing the matching Series:\n%s", out)
	}
	for _, other := range []string{"Severance", "Shōgun"} {
		if strings.Contains(out, other) {
			t.Errorf("output should not show %q:\n%s", other, out)
		}
	}
}

func TestListShowsEverySeriesInSonarrLibrary(t *testing.T) {
	srv := fakeInstance(t, "sonarr")
	useConfig(t, instanceDef{"sonarr", "sonarr", srv.URL, testAPIKey})

	out, stderr, code := run(t, "sonarr", "list")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	for _, want := range []string{"Severance (2022)", "Shōgun (2024)", "Twin Peaks (1990)"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestListJSONIsFlatArrayWithInstanceAndApp(t *testing.T) {
	sonarr := fakeInstance(t, "sonarr")
	radarr := fakeInstance(t, "radarr")
	useConfig(t,
		instanceDef{"sonarr", "sonarr", sonarr.URL, testAPIKey},
		instanceDef{"radarr-4k", "radarr", radarr.URL, testAPIKey},
	)

	sonarrOut, stderr, code := run(t, "sonarr", "list", "--json")
	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	radarrOut, stderr, code := run(t, "radarr-4k", "list", "--json")
	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}

	wantSonarr := `[
  {
    "instance": "sonarr",
    "app": "sonarr",
    "title": "Severance",
    "year": 2022,
    "status": "continuing",
    "monitored": true,
    "sizeOnDisk": 53687091200,
    "tvdbId": 371980,
    "seasons": 2,
    "episodes": 19,
    "episodeFiles": 19
  },
  {
    "instance": "sonarr",
    "app": "sonarr",
    "title": "Shōgun",
    "year": 2024,
    "status": "continuing",
    "monitored": true,
    "sizeOnDisk": 32212254720,
    "tvdbId": 392573,
    "seasons": 1,
    "episodes": 10,
    "episodeFiles": 9
  },
  {
    "instance": "sonarr",
    "app": "sonarr",
    "title": "Twin Peaks",
    "year": 1990,
    "status": "ended",
    "monitored": false,
    "sizeOnDisk": 0,
    "tvdbId": 70533,
    "seasons": 3,
    "episodes": 0,
    "episodeFiles": 0
  }
]
`
	wantRadarr := `[
  {
    "instance": "radarr-4k",
    "app": "radarr",
    "title": "Good Will Hunting",
    "year": 1997,
    "status": "released",
    "monitored": true,
    "sizeOnDisk": 0,
    "tmdbId": 489,
    "hasFile": false
  },
  {
    "instance": "radarr-4k",
    "app": "radarr",
    "title": "The Matrix",
    "year": 1999,
    "status": "released",
    "monitored": true,
    "sizeOnDisk": 8589934592,
    "tmdbId": 603,
    "hasFile": true
  },
  {
    "instance": "radarr-4k",
    "app": "radarr",
    "title": "Project Hail Mary",
    "year": 2026,
    "status": "released",
    "monitored": false,
    "sizeOnDisk": 0,
    "tmdbId": 687163,
    "hasFile": false
  }
]
`
	if sonarrOut != wantSonarr {
		t.Errorf("sonarr output:\n%s\nwant:\n%s", sonarrOut, wantSonarr)
	}
	if radarrOut != wantRadarr {
		t.Errorf("radarr output:\n%s\nwant:\n%s", radarrOut, wantRadarr)
	}
}

func TestListJSONIsEmptyArrayWhenNothingMatches(t *testing.T) {
	srv := fakeInstance(t, "radarr")
	useConfig(t, instanceDef{"radarr", "radarr", srv.URL, testAPIKey})

	out, stderr, code := run(t, "radarr", "list", "nonexistent", "--json")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	if out != "[]\n" {
		t.Errorf("output = %q, want an empty array", out)
	}
}

func TestListTableShowsWhatIsOnDisk(t *testing.T) {
	sonarr := fakeInstance(t, "sonarr")
	radarr := fakeInstance(t, "radarr")
	useConfig(t,
		instanceDef{"sonarr", "sonarr", sonarr.URL, testAPIKey},
		instanceDef{"radarr", "radarr", radarr.URL, testAPIKey},
	)

	sonarrOut, stderr, code := run(t, "sonarr", "list")
	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	radarrOut, stderr, code := run(t, "radarr", "list")
	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}

	for _, want := range []string{"TITLE", "STATUS", "MONITORED", "FILES", "SIZE"} {
		if !strings.Contains(sonarrOut, want) {
			t.Errorf("sonarr output missing header %q:\n%s", want, sonarrOut)
		}
	}
	for _, line := range [][]string{
		{"Shōgun (2024)", "continuing", "yes", "9/10 episodes", "30.0 GiB"},
		{"Twin Peaks (1990)", "ended", "no", "0/0 episodes", "-"},
	} {
		assertRow(t, sonarrOut, line...)
	}
	for _, line := range [][]string{
		{"The Matrix (1999)", "released", "yes", "downloaded", "8.0 GiB"},
		{"Project Hail Mary (2026)", "released", "no", "missing", "-"},
	} {
		assertRow(t, radarrOut, line...)
	}
}

// assertRow checks that one line of out holds every cell, in order.
func assertRow(t *testing.T, out string, cells ...string) {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if !strings.Contains(line, cells[0]) {
			continue
		}
		at := 0
		for _, c := range cells {
			i := strings.Index(line[at:], c)
			if i < 0 {
				t.Errorf("row %q missing %q after column %d", line, c, at)
				return
			}
			at += i + len(c)
		}
		return
	}
	t.Errorf("no row for %q:\n%s", cells[0], out)
}

func TestListWithoutInstanceCoversEveryLibraryDespiteFailures(t *testing.T) {
	sonarr := fakeInstance(t, "sonarr")
	down := fakeInstance(t, "radarr")
	downURL := down.URL
	down.Close()
	radarr := fakeInstance(t, "radarr")
	useConfig(t,
		instanceDef{"sonarr", "sonarr", sonarr.URL, testAPIKey},
		instanceDef{"radarr-old", "radarr", downURL, testAPIKey},
		instanceDef{"radarr", "radarr", radarr.URL, testAPIKey},
	)

	out, stderr, code := run(t, "list")

	if code != 1 {
		t.Errorf("exit code %d, want 1 when an Instance fails", code)
	}
	if n := strings.Count(out, "INSTANCE"); n != 1 {
		t.Errorf("want one combined table, got %d header rows:\n%s", n, out)
	}
	assertRow(t, out, "sonarr", "Severance (2022)")
	assertRow(t, out, "radarr", "Good Will Hunting (1997)")
	// Each Instance's Titles are sorted, and Instances come in config order.
	assertInOrder(t, out, "Severance", "Shōgun", "Twin Peaks", "Good Will Hunting", "The Matrix", "Project Hail Mary")
	if !strings.HasPrefix(stderr, "arr: radarr-old: ") || strings.Count(stderr, "\n") != 1 {
		t.Errorf("stderr should name only the failed Instance, got %q", stderr)
	}
}

func TestListWithoutInstanceFiltersAndPrintsJSON(t *testing.T) {
	sonarr := fakeInstance(t, "sonarr")
	radarr := fakeInstance(t, "radarr")
	useConfig(t,
		instanceDef{"sonarr", "sonarr", sonarr.URL, testAPIKey},
		instanceDef{"radarr", "radarr", radarr.URL, testAPIKey},
	)

	out, stderr, code := run(t, "list", "in", "--json")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	var recs []struct{ Instance, App, Title string }
	if err := json.Unmarshal([]byte(out), &recs); err != nil {
		t.Fatalf("not a JSON array: %v\n%s", err, out)
	}
	var got []string
	for _, r := range recs {
		got = append(got, r.Instance+"/"+r.App+"/"+r.Title)
	}
	want := []string{"sonarr/sonarr/Twin Peaks", "radarr/radarr/Good Will Hunting"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("got %v, want %v", got, want)
	}
}
