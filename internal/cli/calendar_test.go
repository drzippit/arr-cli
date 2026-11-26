package cli_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/drzippit/arr-cli/internal/cli"
)

// amsterdam is UTC+1 in November, so episodes airing late in the UTC evening
// land on the next local day.
func amsterdam(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Amsterdam")
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

// today pins the CLI's clock to 26 November 2026 in Amsterdam.
func today(t *testing.T) {
	t.Helper()
	loc := amsterdam(t)
	cli.SetNow(t, func() time.Time { return time.Date(2026, 11, 26, 12, 0, 0, 0, loc) })
}

// assertInOrder fails unless each of want appears in out after the one before.
func assertInOrder(t *testing.T, out string, want ...string) {
	t.Helper()
	at := 0
	for _, w := range want {
		i := strings.Index(out[at:], w)
		if i < 0 {
			t.Fatalf("output missing %q after offset %d:\n%s", w, at, out)
		}
		at += i + len(w)
	}
}

func TestCalendarListsEpisodesGroupedByLocalDate(t *testing.T) {
	today(t)
	srv := fakeInstance(t, "sonarr")
	useConfig(t, instanceDef{"sonarr", "sonarr", srv.URL, testAPIKey})

	out, stderr, code := run(t, "sonarr", "calendar")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	// Each heading is followed by that day's entries, in date order.
	want := []string{
		"Sun 1 Nov", "00:15", "Shōgun S02E01", "The Return",
		"Sat 7 Nov", "22:00", "Shōgun S02E02", "Ink and Steel",
		"Sat 28 Nov", "03:00", "Severance S02E08", "Sweet Vitriol",
	}
	assertInOrder(t, out, want...)
	if strings.Contains(out, "S02E09") {
		t.Errorf("an episode airing on 1 Dec local time is not in November:\n%s", out)
	}
}

func TestCalendarListsEachMovieReleaseInTheMonth(t *testing.T) {
	today(t)
	srv := fakeInstance(t, "radarr")
	useConfig(t, instanceDef{"radarr", "radarr", srv.URL, testAPIKey})

	out, stderr, code := run(t, "radarr", "calendar")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	want := []string{
		"Sun 1 Nov", "Avengers: Doomsday", "in cinemas",
		"Tue 17 Nov", "Project Hail Mary", "digital release",
		"Tue 24 Nov", "Project Hail Mary", "physical release",
	}
	assertInOrder(t, out, want...)
	if strings.Contains(out, "The Odyssey") {
		t.Errorf("The Odyssey has no release in November:\n%s", out)
	}
	if strings.Contains(out, "(2026)") {
		t.Errorf("Movies should appear as just their title:\n%s", out)
	}
}

func TestCalendarMonthFlagChoosesTheMonth(t *testing.T) {
	today(t)
	srv := fakeInstance(t, "radarr")
	useConfig(t, instanceDef{"radarr", "radarr", srv.URL, testAPIKey})

	for _, tc := range []struct {
		month, heading, has, lacks string
	}{
		{"2026-10", "October 2026", "The Odyssey", "Project Hail Mary"},
		{"prev", "October 2026", "The Odyssey", "Project Hail Mary"},
		{"next", "December 2026", "The Odyssey", "Project Hail Mary"},
	} {
		t.Run(tc.month, func(t *testing.T) {
			out, stderr, code := run(t, "radarr", "calendar", "--month", tc.month)

			if code != 0 {
				t.Fatalf("exit code %d, stderr: %s", code, stderr)
			}
			if !strings.Contains(out, tc.heading) || !strings.Contains(out, tc.has) || strings.Contains(out, tc.lacks) {
				t.Errorf("want %s with %q and without %q:\n%s", tc.heading, tc.has, tc.lacks, out)
			}
		})
	}
}

func TestCalendarRejectsUnknownMonth(t *testing.T) {
	today(t)
	useConfig(t, instanceDef{"radarr", "radarr", "http://127.0.0.1:1", testAPIKey})

	_, stderr, code := run(t, "radarr", "calendar", "--month", "november")

	if code == 0 {
		t.Error("exit code 0 for an unknown month")
	}
	if !strings.Contains(stderr, "YYYY-MM") {
		t.Errorf("stderr should say what --month accepts, got %q", stderr)
	}
}

func TestCalendarAcceptsListFlag(t *testing.T) {
	today(t)
	srv := fakeInstance(t, "sonarr")
	useConfig(t, instanceDef{"sonarr", "sonarr", srv.URL, testAPIKey})

	out, stderr, code := run(t, "sonarr", "calendar", "--list")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	if !strings.Contains(out, "Severance S02E08") {
		t.Errorf("want the list view:\n%s", out)
	}
}

func TestCalendarSaysWhenNothingIsDue(t *testing.T) {
	today(t)
	srv := fakeInstance(t, "sonarr")
	useConfig(t, instanceDef{"sonarr", "sonarr", srv.URL, testAPIKey})

	out, stderr, code := run(t, "sonarr", "calendar", "--month", "2027-03")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	if out != "Nothing due in March 2027.\n" {
		t.Errorf("output = %q", out)
	}
}

func TestCalendarWithoutInstanceMergesEveryInstanceDespiteFailures(t *testing.T) {
	today(t)
	sonarr := fakeInstance(t, "sonarr")
	radarr := fakeInstance(t, "radarr")
	down := fakeInstance(t, "sonarr")
	downURL := down.URL
	down.Close()
	useConfig(t,
		instanceDef{"sonarr", "sonarr", sonarr.URL, testAPIKey},
		instanceDef{"sonarr-old", "sonarr", downURL, testAPIKey},
		instanceDef{"radarr", "radarr", radarr.URL, testAPIKey},
	)

	out, stderr, code := run(t, "calendar")

	if code != 1 {
		t.Errorf("exit code %d, want 1 when an Instance fails", code)
	}
	// 1 Nov holds both a Movie and an episode, under one heading.
	if n := strings.Count(out, "Sun 1 Nov"); n != 1 {
		t.Errorf("want one heading for 1 Nov, got %d:\n%s", n, out)
	}
	movie := strings.Index(out, "Avengers: Doomsday")
	episode := strings.Index(out, "Shōgun S02E01")
	later := strings.Index(out, "Shōgun S02E02")
	if movie < 0 || episode < 0 || later < 0 {
		t.Fatalf("want entries from both working Instances:\n%s", out)
	}
	if !(movie < episode && episode < later) {
		t.Errorf("entries should be in date order, Movies first within a day:\n%s", out)
	}
	if !strings.HasPrefix(stderr, "arr: sonarr-old: ") || strings.Count(stderr, "\n") != 1 {
		t.Errorf("stderr should name only the failed Instance, got %q", stderr)
	}
}

func TestCalendarJSONIsFlatArrayWithInstanceAndApp(t *testing.T) {
	today(t)
	sonarr := fakeInstance(t, "sonarr")
	radarr := fakeInstance(t, "radarr")
	useConfig(t,
		instanceDef{"sonarr", "sonarr", sonarr.URL, testAPIKey},
		instanceDef{"radarr-4k", "radarr", radarr.URL, testAPIKey},
	)

	out, stderr, code := run(t, "calendar", "--json", "--month", "2026-11")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	want := `[
  {
    "instance": "radarr-4k",
    "app": "radarr",
    "date": "2026-11-01",
    "title": "Avengers: Doomsday",
    "year": 2026,
    "release": "inCinemas",
    "hasFile": false
  },
  {
    "instance": "sonarr",
    "app": "sonarr",
    "date": "2026-11-01",
    "airs": "2026-11-01T00:15:00+01:00",
    "title": "Shōgun",
    "season": 2,
    "episode": 1,
    "episodeTitle": "The Return",
    "hasFile": false
  },
  {
    "instance": "sonarr",
    "app": "sonarr",
    "date": "2026-11-07",
    "airs": "2026-11-07T22:00:00+01:00",
    "title": "Shōgun",
    "season": 2,
    "episode": 2,
    "episodeTitle": "Ink and Steel",
    "hasFile": true
  },
  {
    "instance": "radarr-4k",
    "app": "radarr",
    "date": "2026-11-17",
    "title": "Project Hail Mary",
    "year": 2026,
    "release": "digital",
    "hasFile": false
  },
  {
    "instance": "radarr-4k",
    "app": "radarr",
    "date": "2026-11-24",
    "title": "Project Hail Mary",
    "year": 2026,
    "release": "physical",
    "hasFile": false
  },
  {
    "instance": "sonarr",
    "app": "sonarr",
    "date": "2026-11-28",
    "airs": "2026-11-28T03:00:00+01:00",
    "title": "Severance",
    "season": 2,
    "episode": 8,
    "episodeTitle": "Sweet Vitriol",
    "hasFile": false
  }
]
`
	if out != want {
		t.Errorf("output:\n%s\nwant:\n%s", out, want)
	}
}

func TestCalendarAsksForTheMonthWithADayEitherSide(t *testing.T) {
	today(t)
	var gotStart, gotEnd string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotStart, gotEnd = r.URL.Query().Get("start"), r.URL.Query().Get("end")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("[]"))
	}))
	t.Cleanup(srv.Close)
	useConfig(t, instanceDef{"radarr", "radarr", srv.URL, testAPIKey})

	_, stderr, code := run(t, "radarr", "calendar")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	// November in Amsterdam is 2026-10-31T23:00Z to 2026-11-30T23:00Z.
	if gotStart != "2026-10-30T23:00:00Z" || gotEnd != "2026-12-01T23:00:00Z" {
		t.Errorf("asked for %s to %s", gotStart, gotEnd)
	}
}

func TestCalendarPrevInJanuaryIsDecemberOfTheYearBefore(t *testing.T) {
	loc := amsterdam(t)
	cli.SetNow(t, func() time.Time { return time.Date(2027, 1, 15, 12, 0, 0, 0, loc) })
	srv := fakeInstance(t, "radarr")
	useConfig(t, instanceDef{"radarr", "radarr", srv.URL, testAPIKey})

	out, stderr, code := run(t, "radarr", "calendar", "--month", "prev")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	assertInOrder(t, out, "December 2026", "Tue 1 Dec", "The Odyssey", "physical release")
}
