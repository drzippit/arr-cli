package cli_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/drzippit/arr-cli/internal/cli"
)

// editRequest is a PUT to a Media App's bulk editor: series/editor in Sonarr,
// movie/editor in Radarr.
type editRequest struct {
	Path             string
	SeriesIDs        []int `json:"seriesIds"`
	MovieIDs         []int `json:"movieIds"`
	Monitored        *bool `json:"monitored"`
	QualityProfileID *int  `json:"qualityProfileId"`
}

// fakeMediaAppForEdit replays app's recorded responses, with handler in front
// when it is set, and records every PUT to the bulk editor, answering as the
// App does: 202 Accepted.
func fakeMediaAppForEdit(t *testing.T, app string, handler http.Handler) (*httptest.Server, *[]editRequest) {
	t.Helper()
	var edits []editRequest
	next := handler
	if next == nil {
		next = recordedHandler(t, app)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			next.ServeHTTP(w, r)
			return
		}
		if r.URL.Path != "/api/v3/series/editor" && r.URL.Path != "/api/v3/movie/editor" {
			t.Errorf("unexpected PUT to %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		body, _ := io.ReadAll(r.Body)
		e := editRequest{Path: r.URL.Path}
		if err := json.Unmarshal(body, &e); err != nil {
			t.Errorf("PUT body: %v\n%s", err, body)
		}
		edits = append(edits, e)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte("[]"))
	}))
	t.Cleanup(srv.Close)
	return srv, &edits
}

func TestEditSetsTheQualityProfileOfTheSelectionInOneRequest(t *testing.T) {
	srv, edits := fakeMediaAppForEdit(t, "sonarr", nil)
	useConfig(t, instanceDef{"sonarr", "sonarr", srv.URL, testAPIKey})

	out, stderr, code := run(t, "sonarr", "edit", "shōgun", "--set-quality", "hd-1080p", "--yes")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	if len(*edits) != 1 {
		t.Fatalf("want one request to the bulk editor, got %d", len(*edits))
	}
	e := (*edits)[0]
	if e.Path != "/api/v3/series/editor" || len(e.SeriesIDs) != 1 || e.SeriesIDs[0] != 12 ||
		e.QualityProfileID == nil || *e.QualityProfileID != 4 || e.Monitored != nil {
		t.Errorf("want Shōgun (12) set to HD-1080p (4) and monitoring left alone, got %+v", e)
	}
	// The Selection as list shows it, plus what changed.
	for _, want := range []string{"STATUS", "MONITORED", "FILES", "SIZE", "Shōgun", "continuing", "9/10", "CHANGED", "quality: WEB-2160p → HD-1080p"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestEditUnmonitorsMissingMoviesAndGivesTheChangesAsJSON(t *testing.T) {
	srv, edits := fakeMediaAppForEdit(t, "radarr", nil)
	useConfig(t, instanceDef{"radarr", "radarr", srv.URL, testAPIKey})

	out, stderr, code := run(t, "radarr", "edit", "--missing", "--set-monitored=false", "--yes", "--json")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	if len(*edits) != 1 || (*edits)[0].Path != "/api/v3/movie/editor" || len((*edits)[0].MovieIDs) != 1 ||
		(*edits)[0].MovieIDs[0] != 1 || (*edits)[0].Monitored == nil || *(*edits)[0].Monitored {
		t.Fatalf("want Good Will Hunting (1) unmonitored in one request, got %+v", *edits)
	}
	var recs []map[string]any
	if err := json.Unmarshal([]byte(out), &recs); err != nil || len(recs) != 1 {
		t.Fatalf("want one record: %v\n%s", err, out)
	}
	if recs[0]["title"] != "Good Will Hunting" || recs[0]["monitored"] != false {
		t.Errorf("want the Movie as it is now, got %v", recs[0])
	}
	want := map[string]any{"monitored": map[string]any{"from": true, "to": false}}
	if got, _ := json.Marshal(recs[0]["changes"]); string(got) != mustJSON(want) {
		t.Errorf("changes = %s, want %s", got, mustJSON(want))
	}
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func TestEditRefusesWhatItCannotDoBeforeSendingAnything(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"no change", []string{"shōgun", "--yes"}, "--set-monitored or --set-quality"},
		{"whole Library without --all", []string{"--set-monitored=false", "--yes"}, "pass --all"},
		{"from a script without --yes", []string{"shōgun", "--set-monitored=false"}, "this would change 1 Series; from a script pass --yes, or --dry-run"},
		{"unknown profile", []string{"shōgun", "--set-quality", "4K", "--yes"}, "Any, HD-1080p, WEB-2160p"},
		{"unknown filter value", []string{"--status", "released", "--set-monitored=false", "--yes"}, `--status "released"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, edits := fakeMediaAppForEdit(t, "sonarr", nil)
			useConfig(t, instanceDef{"sonarr", "sonarr", srv.URL, testAPIKey})

			_, stderr, code := run(t, append([]string{"sonarr", "edit"}, tc.args...)...)

			if code != 1 || !strings.Contains(stderr, tc.want) {
				t.Errorf("exit code %d, stderr %q, want %q", code, stderr, tc.want)
			}
			if len(*edits) != 0 {
				t.Errorf("sent %d edits", len(*edits))
			}
		})
	}
}

func TestEditAllEditsTheWholeLibrary(t *testing.T) {
	srv, edits := fakeMediaAppForEdit(t, "sonarr", nil)
	useConfig(t, instanceDef{"sonarr", "sonarr", srv.URL, testAPIKey})

	_, stderr, code := run(t, "sonarr", "edit", "--all", "--set-monitored=true", "--yes")

	if code != 0 || len(*edits) != 1 || len((*edits)[0].SeriesIDs) != 3 {
		t.Errorf("exit code %d, stderr %q, edits %+v", code, stderr, *edits)
	}
}

func TestEditDryRunShowsThePlanAndSendsNothing(t *testing.T) {
	srv, edits := fakeMediaAppForEdit(t, "sonarr", nil)
	useConfig(t, instanceDef{"sonarr", "sonarr", srv.URL, testAPIKey})

	out, stderr, code := run(t, "sonarr", "edit", "--quality", "HD-1080p", "--set-quality", "Any", "--dry-run")

	if code != 0 || len(*edits) != 0 {
		t.Fatalf("exit code %d, %d edits, stderr %s", code, len(*edits), stderr)
	}
	for _, want := range []string{"Severance", "Twin Peaks", "quality: HD-1080p → Any"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if !strings.Contains(stderr, "dry run; nothing changed") {
		t.Errorf("stderr should say nothing changed: %q", stderr)
	}
}

func TestEditInATerminalAsksFirst(t *testing.T) {
	for _, answer := range []bool{true, false} {
		srv, edits := fakeMediaAppForEdit(t, "radarr", nil)
		useConfig(t, instanceDef{"radarr", "radarr", srv.URL, testAPIKey})
		var asked string
		cli.SetConfirm(t, func(q string) (bool, error) { asked = q; return answer, nil })

		_, stderr, code := run(t, "radarr", "edit", "--monitored", "--set-quality", "Ultra-HD")

		if asked != "Change 2 Movies in radarr: quality Ultra-HD?" {
			t.Errorf("asked %q", asked)
		}
		if answer && (code != 0 || len(*edits) != 1) {
			t.Errorf("yes: exit code %d, %d edits, stderr %s", code, len(*edits), stderr)
		}
		if !answer && (code != 1 || len(*edits) != 0 || !strings.Contains(stderr, "cancelled")) {
			t.Errorf("no: exit code %d, %d edits, stderr %q", code, len(*edits), stderr)
		}
	}
}

func TestEditWithNothingMatchingChangesNothingAndSucceeds(t *testing.T) {
	srv, edits := fakeMediaAppForEdit(t, "sonarr", nil)
	useConfig(t, instanceDef{"sonarr", "sonarr", srv.URL, testAPIKey})

	out, stderr, code := run(t, "sonarr", "edit", "no such series", "--set-monitored=false", "--json")

	if code != 0 || len(*edits) != 0 || strings.TrimSpace(out) != "[]" || !strings.Contains(stderr, "no Titles matched") {
		t.Errorf("exit code %d, %d edits, out %q, stderr %q", code, len(*edits), out, stderr)
	}
}

func TestEditRefusesSonarrOlderThanSupported(t *testing.T) {
	srv, edits := fakeMediaAppForEdit(t, "sonarr", runningVersion("Sonarr", "3.0.10.1567", recordedHandler(t, "sonarr")))
	useConfig(t, instanceDef{"sonarr", "sonarr", srv.URL, testAPIKey})

	_, stderr, code := run(t, "sonarr", "edit", "shōgun", "--set-monitored=false", "--yes")

	if code != 1 || !strings.Contains(stderr, "supports Sonarr 4") || len(*edits) != 0 {
		t.Errorf("exit code %d, %d edits, stderr %q", code, len(*edits), stderr)
	}
}
