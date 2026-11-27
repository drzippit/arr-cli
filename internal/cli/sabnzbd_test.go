package cli_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/drzippit/arr-cli/internal/config"
)

// fakeSabnzbd serves the recorded SABnzbd API responses in testdata/sabnzbd.
func fakeSabnzbd(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(sabnzbdHandler(t))
	t.Cleanup(srv.Close)
	return srv
}

// sabnzbdHandler replays testdata/sabnzbd/<mode>.json for /api?mode=<mode>,
// and refuses a wrong API key the way SABnzbd does: 403 with a plain-text
// reason.
func sabnzbdHandler(t *testing.T) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.URL.Path != "/api" || q.Get("output") != "json" {
			t.Errorf("unexpected request to %s", r.URL)
			http.NotFound(w, r)
			return
		}
		if q.Get("apikey") != testAPIKey {
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, "API Key Incorrect")
			return
		}
		body, err := os.ReadFile(filepath.Join("testdata", "sabnzbd", q.Get("mode")+".json"))
		if err != nil {
			t.Errorf("no recorded response for mode %q", q.Get("mode"))
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	})
}

// sabnzbdVersion answers the queue as SABnzbd at version, and passes every
// other request to next.
func sabnzbdVersion(t *testing.T, version string, next http.Handler) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("mode") != "queue" {
			next.ServeHTTP(w, r)
			return
		}
		body, err := os.ReadFile(filepath.Join("testdata", "sabnzbd", "queue.json"))
		if err != nil {
			t.Fatal(err)
		}
		var doc map[string]map[string]any
		if err := json.Unmarshal(body, &doc); err != nil {
			t.Fatal(err)
		}
		doc["queue"]["version"] = version
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(doc)
	})
}

func TestStatusOfSabnzbdShowsStateSpeedFreeSpaceAndWarnings(t *testing.T) {
	srv := fakeSabnzbd(t)
	useConfig(t, instanceDef{"sab", "sabnzbd", srv.URL, testAPIKey})

	out, stderr, code := run(t, "sab", "status")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	for _, want := range []string{
		"INSTANCE", "APP", "REACHABLE", "VERSION", "STATE", "SPEED", "FREE", "WARNINGS",
		"sab", "sabnzbd", "yes", "5.1.3", "downloading", "2.0 MiB/s", "812.5 GiB", "1",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Too little diskspace") {
		t.Errorf("warnings listed without -v:\n%s", out)
	}
}

func TestStatusVerboseListsSabnzbdWarningsWithoutFailing(t *testing.T) {
	srv := fakeSabnzbd(t)
	useConfig(t, instanceDef{"sab", "sabnzbd", srv.URL, testAPIKey})

	out, stderr, code := run(t, "sab", "status", "-v")

	if code != 0 {
		t.Errorf("exit code %d, want 0: Warnings don't fail status; stderr: %s", code, stderr)
	}
	for _, want := range []string{"warning", "Too little diskspace forcing PAUSE"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestStatusJSONGivesDownloadClientFieldsOnlyForDownloadClients(t *testing.T) {
	sonarr := fakeInstance(t, "sonarr")
	sab := fakeSabnzbd(t)
	useConfig(t,
		instanceDef{"sonarr", "sonarr", sonarr.URL, testAPIKey},
		instanceDef{"sab", "sabnzbd", sab.URL, testAPIKey},
	)

	out, stderr, code := run(t, "status", "--json")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	var recs []map[string]any
	if err := json.Unmarshal([]byte(out), &recs); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if len(recs) != 2 {
		t.Fatalf("want 2 records, got %d:\n%s", len(recs), out)
	}
	for _, key := range []string{"state", "speed", "freeSpace"} {
		if _, ok := recs[0][key]; ok {
			t.Errorf("Sonarr record has %q:\n%s", key, out)
		}
	}
	want := map[string]any{
		"instance": "sab", "app": "sabnzbd", "reachable": true, "version": "5.1.3", "supported": true,
		"state": "downloading", "speed": float64(2048 * 1024), "freeSpace": float64(812.5 * (1 << 30)), "warnings": float64(1),
	}
	for k, v := range want {
		if recs[1][k] != v {
			t.Errorf("sab %q = %v, want %v", k, recs[1][k], v)
		}
	}
}

func TestStatusOfSabnzbdWithWrongAPIKeyIsReachableButFails(t *testing.T) {
	srv := fakeSabnzbd(t)
	useConfig(t, instanceDef{"sab", "sabnzbd", srv.URL, "wrong-key"})

	out, stderr, code := run(t, "sab", "status")

	if code != 1 {
		t.Errorf("exit code %d, want 1", code)
	}
	if !strings.Contains(out, "yes") {
		t.Errorf("an Instance that answers is reachable:\n%s", out)
	}
	if !strings.Contains(stderr, "API Key Incorrect (check the API key)") {
		t.Errorf("stderr should say why: %q", stderr)
	}
	if strings.Contains(stderr, "wrong-key") {
		t.Errorf("stderr leaks the API key: %q", stderr)
	}
}

func TestStatusMarksSabnzbdOlderThanSupported(t *testing.T) {
	srv := httptest.NewServer(sabnzbdVersion(t, "3.7.2", sabnzbdHandler(t)))
	t.Cleanup(srv.Close)
	useConfig(t, instanceDef{"sab", "sabnzbd", srv.URL, testAPIKey})

	out, stderr, code := run(t, "sab", "status")

	if code != 0 {
		t.Errorf("exit code %d, want 0: an old version is not a failure; stderr: %s", code, stderr)
	}
	if !strings.Contains(out, "3.7.2 (unsupported)") {
		t.Errorf("want the version marked unsupported:\n%s", out)
	}
}

func TestListCalendarAndQueueSkipDownloadClients(t *testing.T) {
	sonarr := fakeInstance(t, "sonarr")
	sab := fakeSabnzbd(t) // fails the test on any request
	useConfig(t,
		instanceDef{"sonarr", "sonarr", sonarr.URL, testAPIKey},
		instanceDef{"sab", "sabnzbd", sab.URL, testAPIKey},
	)
	today(t)

	for _, args := range [][]string{{"list"}, {"calendar", "--list"}, {"queue"}} {
		out, stderr, code := run(t, args...)
		if code != 0 {
			t.Errorf("arr %v: exit code %d, stderr: %s", args, code, stderr)
		}
		if strings.Contains(out, "sab ") || strings.Contains(stderr, "sab") {
			t.Errorf("arr %v mentions the Download Client:\n%s%s", args, out, stderr)
		}
	}
}

func TestListWithOnlyDownloadClientsSaysThereAreNoMediaApps(t *testing.T) {
	sab := fakeSabnzbd(t)
	useConfig(t, instanceDef{"sab", "sabnzbd", sab.URL, testAPIKey})

	_, stderr, code := run(t, "list")

	if code != 1 || !strings.Contains(stderr, "no Sonarr or Radarr Instances configured") {
		t.Errorf("exit code %d, stderr %q", code, stderr)
	}
}

func TestSabnzbdInstanceHasNoMediaAppCommands(t *testing.T) {
	sab := fakeSabnzbd(t)
	useConfig(t, instanceDef{"sab", "sabnzbd", sab.URL, testAPIKey})

	for _, cmd := range []string{"list", "calendar", "search", "add", "queue"} {
		_, stderr, code := run(t, "sab", cmd, "x")
		if code != 1 || !strings.Contains(stderr, "unknown command") {
			t.Errorf("arr sab %s: exit code %d, stderr %q; want unknown command", cmd, code, stderr)
		}
	}
	got := complete(t, "sab", "")
	if strings.Join(got, " ") != "status" {
		t.Errorf("completion for arr sab offers %v, want only status", got)
	}
}

func TestConfigAddSavesReachableSabnzbd(t *testing.T) {
	srv := fakeSabnzbd(t)
	path := useMissingConfig(t)
	inst := config.Instance{Name: "sab", App: config.Sabnzbd, URL: srv.URL, APIKey: testAPIKey}
	fillForm(t, inst)

	out, stderr, code := run(t, "config", "add")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("config after add: %v", err)
	}
	if len(cfg.Instances) != 1 || cfg.Instances[0] != inst {
		t.Errorf("Instances %+v, want %+v", cfg.Instances, inst)
	}
	if want := "Added sab (SABnzbd 5.1.3) to " + path; !strings.Contains(out, want) {
		t.Errorf("output missing %q:\n%s", want, out)
	}
}

func TestConfigAddWithWrongSabnzbdKeySavesNothing(t *testing.T) {
	srv := fakeSabnzbd(t)
	path := useMissingConfig(t)
	fillForm(t, config.Instance{Name: "sab", App: config.Sabnzbd, URL: srv.URL, APIKey: "wrong"})

	_, stderr, code := run(t, "config", "add")

	if code != 1 || !strings.Contains(stderr, "check the API key") || !strings.Contains(stderr, "nothing saved") {
		t.Errorf("exit code %d, stderr %q", code, stderr)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("config written despite the failure: %v", err)
	}
}
