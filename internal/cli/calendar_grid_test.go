package cli_test

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/drzippit/arr-cli/internal/cli"
)

func TestCalendarOnATerminalIsAMonthGrid(t *testing.T) {
	today(t)
	cli.SetTerminal(t, 140)
	srv := fakeInstance(t, "sonarr")
	useConfig(t, instanceDef{"sonarr", "sonarr", srv.URL, testAPIKey})

	out, stderr, code := run(t, "sonarr", "calendar")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	plain := ansi.Strip(out)
	// November 2026 starts on a Sunday, so 1 Nov is the last cell of the
	// first week, and 30 Nov the first cell of the sixth.
	lines := strings.Split(plain, "\n")
	header := lineWith(t, lines, "Mon")
	assertInOrder(t, header, "Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun")
	assertInOrder(t, plain, "November 2026", "Mon", "1", "Shōgun S02E01", "2", "3", "28", "Severance S02E08", "30")
	if strings.Contains(plain, "Sun 1 Nov") {
		t.Errorf("want the grid, not the list:\n%s", plain)
	}
	if col := cellColumn(t, lines, "Shōgun S02E01"); col != 6 {
		t.Errorf("1 Nov is a Sunday, but its entry is in column %d:\n%s", col, plain)
	}
	if col := cellColumn(t, lines, "Severance S02E08"); col != 5 {
		t.Errorf("28 Nov is a Saturday, but its entry is in column %d:\n%s", col, plain)
	}
	for _, l := range lines {
		if w := ansi.StringWidth(l); w > 140 {
			t.Errorf("line is %d columns, wider than the terminal: %q", w, l)
		}
	}
}

// lineWith returns the first line containing s.
func lineWith(t *testing.T, lines []string, s string) string {
	t.Helper()
	for _, l := range lines {
		if strings.Contains(l, s) {
			return l
		}
	}
	t.Fatalf("no line contains %q", s)
	return ""
}

// cellColumn returns which grid column (0 for Monday) s is drawn in, counting
// the vertical borders before it on its line.
func cellColumn(t *testing.T, lines []string, s string) int {
	t.Helper()
	l := lineWith(t, lines, s)
	return strings.Count(l[:strings.Index(l, s)], "│") - 1
}

// busyRadarr is a Radarr Instance with six digital releases on 3 November
// 2026, one of them with a long title.
func busyRadarr(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var movies []string
		for _, title := range []string{"Dune: Part Three of Many Parts", "Heat 2", "Alien", "Brazil", "Clue", "Fargo"} {
			movies = append(movies, `{"title":"`+title+`","digitalRelease":"2026-11-03T00:00:00Z","monitored":true}`)
		}
		w.Write([]byte("[" + strings.Join(movies, ",") + "]"))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestCalendarGridTruncatesEntriesAndCountsTheRest(t *testing.T) {
	today(t)
	cli.SetTerminal(t, 100)
	useConfig(t, instanceDef{"radarr", "radarr", busyRadarr(t).URL, testAPIKey})

	out, stderr, code := run(t, "radarr", "calendar")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	plain := ansi.Strip(out)
	// A 100-column terminal leaves 12 columns a cell: a space and 11 of text.
	assertInOrder(t, plain, "Dune: Part…", "Heat 2", "Alien", "+3 more")
	for _, hidden := range []string{"Dune: Part ", "Brazil", "Clue", "Fargo"} {
		if strings.Contains(plain, hidden) {
			t.Errorf("%q should not fit in the cell:\n%s", hidden, plain)
		}
	}
	for _, l := range strings.Split(plain, "\n") {
		if w := ansi.StringWidth(l); w > 100 {
			t.Errorf("line is %d columns, wider than the terminal: %q", w, l)
		}
	}
}

// styleOf returns the ANSI style sequences immediately before the first
// styled occurrence of s in out.
func styleOf(t *testing.T, out, s string) string {
	t.Helper()
	m := regexp.MustCompile(`((?:\x1b\[[0-9;]*m)+)` + regexp.QuoteMeta(s)).FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("output has no styled %q:\n%q", s, out)
	}
	return m[1]
}

func TestCalendarGridColoursSonarrAndRadarrApart(t *testing.T) {
	today(t)
	cli.SetTerminal(t, 140)
	sonarr := fakeInstance(t, "sonarr")
	radarr := fakeInstance(t, "radarr")
	useConfig(t,
		instanceDef{"sonarr", "sonarr", sonarr.URL, testAPIKey},
		instanceDef{"radarr", "radarr", radarr.URL, testAPIKey},
	)

	out, stderr, code := run(t, "calendar")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	episode, otherEpisode := styleOf(t, out, "Shōgun S02E01"), styleOf(t, out, "Severance")
	movie := styleOf(t, out, "Avengers")
	if episode == "" || movie == "" {
		t.Fatalf("entries should be coloured, got %q and %q", episode, movie)
	}
	if episode == movie {
		t.Errorf("Sonarr and Radarr entries share the style %q", episode)
	}
	if episode != otherEpisode {
		t.Errorf("Sonarr entries should share a style, got %q and %q", episode, otherEpisode)
	}
}

func TestCalendarGridHighlightsToday(t *testing.T) {
	today(t)
	cli.SetTerminal(t, 140)
	srv := fakeInstance(t, "sonarr")
	useConfig(t, instanceDef{"sonarr", "sonarr", srv.URL, testAPIKey})

	out, _, _ := run(t, "sonarr", "calendar")
	if styleOf(t, out, "26") == styleOf(t, out, "25") {
		t.Errorf("26 Nov is today and should stand out from 25 Nov:\n%q", out)
	}

	if !strings.Contains(ansi.Strip(out), "26 today") {
		t.Errorf("today should be labelled, for terminals without colour:\n%s", ansi.Strip(out))
	}

	out, _, _ = run(t, "sonarr", "calendar", "--month", "next")
	if styleOf(t, out, "26") != styleOf(t, out, "25") || strings.Contains(out, "today") {
		t.Errorf("26 Dec is not today and should look like 25 Dec:\n%q", out)
	}
}

func TestCalendarGridOfAMonthStartingOnMondayHasNoBlankWeek(t *testing.T) {
	today(t)
	cli.SetTerminal(t, 140)
	srv := fakeInstance(t, "sonarr")
	useConfig(t, instanceDef{"sonarr", "sonarr", srv.URL, testAPIKey})

	// February 2027 starts on a Monday and fills exactly four weeks.
	out, stderr, code := run(t, "sonarr", "calendar", "--month", "2027-02")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	lines := strings.Split(ansi.Strip(out), "\n")
	if col := cellColumn(t, lines, " 1 "); col != 0 {
		t.Errorf("1 Feb 2027 is a Monday, but is in column %d", col)
	}
	if col := cellColumn(t, lines, " 28 "); col != 6 {
		t.Errorf("28 Feb 2027 is a Sunday, but is in column %d", col)
	}
	// One rule under the header and one between each of the four weeks.
	if n := strings.Count(ansi.Strip(out), "├"); n != 4 {
		t.Errorf("want 4 weeks, got %d:\n%s", n, ansi.Strip(out))
	}
}

func TestCalendarFallsBackToTheListOnANarrowTerminal(t *testing.T) {
	today(t)
	cli.SetTerminal(t, 80)
	srv := fakeInstance(t, "sonarr")
	useConfig(t, instanceDef{"sonarr", "sonarr", srv.URL, testAPIKey})

	out, stderr, code := run(t, "sonarr", "calendar")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	assertInOrder(t, ansi.Strip(out), "Sun 1 Nov", "Shōgun S02E01", "The Return")
	if strings.Contains(out, "│") {
		t.Errorf("an 80-column terminal is too narrow for the grid:\n%s", out)
	}
}

func TestCalendarListAndJSONFlagsOverrideTheGrid(t *testing.T) {
	today(t)
	cli.SetTerminal(t, 140)
	srv := fakeInstance(t, "sonarr")
	useConfig(t, instanceDef{"sonarr", "sonarr", srv.URL, testAPIKey})

	out, _, _ := run(t, "sonarr", "calendar", "--list")
	assertInOrder(t, ansi.Strip(out), "Sun 1 Nov", "Shōgun S02E01", "The Return")
	if strings.Contains(out, "│") {
		t.Errorf("--list should not draw the grid:\n%s", out)
	}

	out, _, _ = run(t, "sonarr", "calendar", "--json")
	if !strings.HasPrefix(out, "[\n  {\n    \"instance\": \"sonarr\"") || strings.Contains(out, "\x1b") {
		t.Errorf("--json should print plain JSON on a terminal too:\n%q", out)
	}
}

func TestCalendarWithoutATerminalIsAnUnstyledList(t *testing.T) {
	today(t)
	srv := fakeInstance(t, "sonarr")
	useConfig(t, instanceDef{"sonarr", "sonarr", srv.URL, testAPIKey})

	out, _, _ := run(t, "sonarr", "calendar")

	assertInOrder(t, out, "Sun 1 Nov", "Shōgun S02E01")
	if strings.Contains(out, "\x1b") || strings.Contains(out, "│") {
		t.Errorf("output to a pipe should be a plain list:\n%q", out)
	}
}
