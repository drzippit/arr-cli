package cli_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/drzippit/arr-cli/internal/cli"
)

// huntCommand is a POST to a Media App's command API that starts a search.
type huntCommand struct {
	Name     string `json:"name"`
	SeriesID int    `json:"seriesId"`
	MovieIDs []int  `json:"movieIds"`
}

// fakeMediaAppForHunt replays app's recorded responses, with handler in front
// when it is set, and records every command POSTed. A SeriesSearch for a
// Series in failFor is answered with 500, as an App does when it can't take
// the command.
func fakeMediaAppForHunt(t *testing.T, app string, handler http.Handler, failFor ...int) (*httptest.Server, *[]huntCommand) {
	t.Helper()
	var cmds []huntCommand
	next := handler
	if next == nil {
		next = recordedHandler(t, app)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			next.ServeHTTP(w, r)
			return
		}
		if r.URL.Path != "/api/v3/command" {
			t.Errorf("unexpected POST to %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var c huntCommand
		if err := json.Unmarshal(body, &c); err != nil {
			t.Errorf("POST body: %v\n%s", err, body)
		}
		cmds = append(cmds, c)
		for _, id := range failFor {
			if c.SeriesID == id {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"id": 1, "name": "` + c.Name + `", "status": "queued"}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &cmds
}

func TestHuntStartsOneSeriesSearchPerSeries(t *testing.T) {
	srv, cmds := fakeMediaAppForHunt(t, "sonarr", nil)
	useConfig(t, instanceDef{"sonarr", "sonarr", srv.URL, testAPIKey})

	out, stderr, code := run(t, "sonarr", "hunt", "--monitored", "--yes")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	var ids []int
	for _, c := range *cmds {
		if c.Name != "SeriesSearch" {
			t.Errorf("want SeriesSearch, got %q", c.Name)
		}
		ids = append(ids, c.SeriesID)
	}
	if len(ids) != 2 || !(ids[0] == 7 && ids[1] == 12 || ids[0] == 12 && ids[1] == 7) {
		t.Errorf("want one search each for Severance (7) and Shōgun (12), got %v", ids)
	}
	// The Selection as list shows it, plus how each Hunt went.
	for _, want := range []string{"STATUS", "FILES", "HUNT", "Severance", "Shōgun", "continuing", "queued"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

// manySeries answers Sonarr's series list with n monitored Series, IDs 1 to n,
// and passes every other request to the recorded responses.
func manySeries(t *testing.T, n int) http.Handler {
	t.Helper()
	recorded := recordedHandler(t, "sonarr")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/series" {
			recorded.ServeHTTP(w, r)
			return
		}
		series := make([]map[string]any, n)
		for i := range series {
			series[i] = map[string]any{"id": i + 1, "title": fmt.Sprintf("Series %02d", i+1), "monitored": true, "qualityProfileId": 4}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(series)
	})
}

func TestHuntIsCappedAt25TitlesUnlessMaxOrAllSaysOtherwise(t *testing.T) {
	cases := []struct {
		args    []string
		hunted  int
		refusal string
	}{
		{[]string{"--monitored"}, 0, "30 Series, more than 25; pass --max 30 or --all"},
		{[]string{"--monitored", "--max", "29"}, 0, "30 Series, more than 29"},
		{[]string{"--monitored", "--max", "30"}, 30, ""},
		{[]string{"--all"}, 30, ""},
	}
	for _, tc := range cases {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			srv, cmds := fakeMediaAppForHunt(t, "sonarr", manySeries(t, 30))
			useConfig(t, instanceDef{"sonarr", "sonarr", srv.URL, testAPIKey})

			_, stderr, code := run(t, append([]string{"sonarr", "hunt", "--yes"}, tc.args...)...)

			if len(*cmds) != tc.hunted {
				t.Errorf("hunted %d Series, want %d", len(*cmds), tc.hunted)
			}
			if tc.refusal != "" && (code != 1 || !strings.Contains(stderr, tc.refusal) || !strings.Contains(stderr, "--max")) {
				t.Errorf("exit code %d, stderr %q, want a refusal with %q", code, stderr, tc.refusal)
			}
			if tc.refusal == "" && code != 0 {
				t.Errorf("exit code %d, stderr %q", code, stderr)
			}
		})
	}
}

func TestHuntTriesEverySeriesAndReportsTheOnesThatFailed(t *testing.T) {
	srv, cmds := fakeMediaAppForHunt(t, "sonarr", nil, 12)
	useConfig(t, instanceDef{"sonarr", "sonarr", srv.URL, testAPIKey})

	out, stderr, code := run(t, "sonarr", "hunt", "--monitored", "--yes", "--json")

	if code != 1 {
		t.Errorf("exit code %d, want 1 when a Hunt failed", code)
	}
	if len(*cmds) != 2 {
		t.Errorf("want both Series tried, got %d searches", len(*cmds))
	}
	if !strings.HasPrefix(stderr, "arr: sonarr: Shōgun: ") || strings.Count(stderr, "\n") != 1 {
		t.Errorf("stderr should name only the Series that failed: %q", stderr)
	}
	var recs []map[string]any
	if err := json.Unmarshal([]byte(out), &recs); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	hunt := map[string]any{}
	for _, r := range recs {
		hunt[r["title"].(string)] = r["hunt"]
	}
	if hunt["Severance"] != "queued" || hunt["Shōgun"] != "failed" {
		t.Errorf("want Severance queued and Shōgun failed, got %v", hunt)
	}
}

func TestHuntSearchesEveryMovieOfTheSelectionInOneCommand(t *testing.T) {
	srv, cmds := fakeMediaAppForHunt(t, "radarr", nil)
	useConfig(t, instanceDef{"radarr", "radarr", srv.URL, testAPIKey})

	_, stderr, code := run(t, "radarr", "hunt", "--monitored", "--yes")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	if len(*cmds) != 1 || (*cmds)[0].Name != "MoviesSearch" || len((*cmds)[0].MovieIDs) != 2 {
		t.Errorf("want one MoviesSearch for The Matrix and Good Will Hunting, got %+v", *cmds)
	}
}

func TestHuntGuardsItsSelection(t *testing.T) {
	cases := []struct {
		name    string
		handler func(t *testing.T) http.Handler
		args    []string
		code    int
		want    string
	}{
		{"old Sonarr", func(t *testing.T) http.Handler {
			return runningVersion("Sonarr", "3.0.10.1567", recordedHandler(t, "sonarr"))
		}, []string{"--monitored", "--yes"}, 1, "supports Sonarr 4"},
		{"from a script", nil, []string{"--monitored"}, 1, "this would hunt 2 Series; from a script pass --yes, or --dry-run"},
		{"whole Library", nil, []string{"--yes"}, 1, "pass --all"},
		{"dry run", nil, []string{"--monitored", "--dry-run"}, 0, "dry run; nothing hunted"},
		{"nothing matched", nil, []string{"no such series", "--yes"}, 0, "no Titles matched"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var h http.Handler
			if tc.handler != nil {
				h = tc.handler(t)
			}
			srv, cmds := fakeMediaAppForHunt(t, "sonarr", h)
			useConfig(t, instanceDef{"sonarr", "sonarr", srv.URL, testAPIKey})

			_, stderr, code := run(t, append([]string{"sonarr", "hunt"}, tc.args...)...)

			if code != tc.code || !strings.Contains(stderr, tc.want) || len(*cmds) != 0 {
				t.Errorf("exit code %d, stderr %q, %d searches; want exit %d and %q", code, stderr, len(*cmds), tc.code, tc.want)
			}
		})
	}
}

func TestHuntInATerminalAsksFirst(t *testing.T) {
	srv, cmds := fakeMediaAppForHunt(t, "sonarr", nil)
	useConfig(t, instanceDef{"sonarr", "sonarr", srv.URL, testAPIKey})
	var asked string
	cli.SetConfirm(t, func(q string) (bool, error) { asked = q; return false, nil })

	_, _, code := run(t, "sonarr", "hunt", "--monitored")

	if asked != "Hunt for Releases of 2 Series in sonarr?" || code != 1 || len(*cmds) != 0 {
		t.Errorf("asked %q, exit code %d, %d searches", asked, code, len(*cmds))
	}
}

func TestHuntDryRunPreviewsASelectionOverTheCap(t *testing.T) {
	srv, cmds := fakeMediaAppForHunt(t, "sonarr", manySeries(t, 30))
	useConfig(t, instanceDef{"sonarr", "sonarr", srv.URL, testAPIKey})

	out, stderr, code := run(t, "sonarr", "hunt", "--monitored", "--dry-run")

	if code != 0 || len(*cmds) != 0 || !strings.Contains(out, "Series 30") {
		t.Errorf("exit code %d, %d searches, stderr %q:\n%s", code, len(*cmds), stderr, out)
	}
}
