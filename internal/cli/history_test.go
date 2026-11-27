package cli_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHistoryListsFinishedDownloadsNewestFirst(t *testing.T) {
	today(t)
	srv := fakeSabnzbd(t)
	useConfig(t, instanceDef{"sab", "sabnzbd", srv.URL, testAPIKey})

	out, stderr, code := run(t, "sab", "history")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	for _, want := range []string{
		"INSTANCE", "FINISHED", "RELEASE", "STATUS", "SIZE", "CATEGORY", "REASON",
		"2026-11-27 10:00", "Dune.Part.Two.2024.2160p.WEB-DL-EXAMPLE", "completed", "18.0 GiB", "movies",
		"2026-11-27 07:13", "Some.Broken.Upload.x264-NOGRP", "failed", "Download failed - Not on your server(s)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	assertInOrder(t, out, "Dune.Part.Two", "Some.Broken.Upload")
	if strings.Contains(out, "The.Bear") {
		t.Errorf("a Download still being post-processed is in the Queue, not the History:\n%s", out)
	}
}

func TestHistoryFailedShowsOnlyFailedDownloads(t *testing.T) {
	today(t)
	var asked string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = r.URL.Query().Get("failed_only")
		sabnzbdHandler(t).ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	useConfig(t, instanceDef{"sab", "sabnzbd", srv.URL, testAPIKey})

	out, stderr, code := run(t, "sab", "history", "--failed")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	if asked != "1" {
		t.Errorf("want SABnzbd asked for failed Downloads only, got failed_only=%q", asked)
	}
	if !strings.Contains(out, "Some.Broken.Upload") || strings.Contains(out, "Dune.Part.Two") {
		t.Errorf("want only the failed Download:\n%s", out)
	}
}

func TestHistoryLimitKeepsTheNewest(t *testing.T) {
	today(t)
	var asked string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = r.URL.Query().Get("limit")
		sabnzbdHandler(t).ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	useConfig(t, instanceDef{"sab", "sabnzbd", srv.URL, testAPIKey})

	out, stderr, code := run(t, "sab", "history", "--limit", "1", "--json")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	if asked == "" {
		t.Errorf("want the limit passed to SABnzbd")
	}
	var recs []map[string]any
	if err := json.Unmarshal([]byte(out), &recs); err != nil || len(recs) != 1 {
		t.Fatalf("want 1 record: %v\n%s", err, out)
	}
	want := map[string]any{
		"instance": "sab", "app": "sabnzbd", "release": "Dune.Part.Two.2024.2160p.WEB-DL-EXAMPLE",
		"downloadId": "SABnzbd_nzo_done111", "status": "completed", "size": float64(19327352832),
		"category": "movies", "finished": "2026-11-27T09:00:00Z",
	}
	for k, v := range want {
		if recs[0][k] != v {
			t.Errorf("%q = %v, want %v", k, recs[0][k], v)
		}
	}
	if _, ok := recs[0]["failMessage"]; ok {
		t.Errorf("a completed Download has no failMessage: %v", recs[0])
	}
}

func TestHistoryRejectsALimitBelowOne(t *testing.T) {
	srv := fakeSabnzbd(t)
	useConfig(t, instanceDef{"sab", "sabnzbd", srv.URL, testAPIKey})

	_, stderr, code := run(t, "sab", "history", "--limit", "0")

	if code != 1 || !strings.Contains(stderr, "--limit") {
		t.Errorf("exit code %d, stderr %q", code, stderr)
	}
}

func TestHistoryWithoutInstanceCoversDownloadClientsOnly(t *testing.T) {
	today(t)
	sonarr := fakeInstance(t, "sonarr") // fails the test on a request it doesn't know
	sab := fakeSabnzbd(t)
	useConfig(t,
		instanceDef{"sonarr", "sonarr", sonarr.URL, testAPIKey},
		instanceDef{"sab", "sabnzbd", sab.URL, testAPIKey},
	)

	out, stderr, code := run(t, "history")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	if !strings.Contains(out, "Dune.Part.Two") {
		t.Errorf("want SABnzbd's History:\n%s", out)
	}
}

func TestHistoryWithoutDownloadClientsSaysSo(t *testing.T) {
	sonarr := fakeInstance(t, "sonarr")
	useConfig(t, instanceDef{"sonarr", "sonarr", sonarr.URL, testAPIKey})

	_, stderr, code := run(t, "history")

	if code != 1 || !strings.Contains(stderr, "no SABnzbd Instances configured") {
		t.Errorf("exit code %d, stderr %q", code, stderr)
	}
	if _, stderr, code := run(t, "sonarr", "history"); code != 1 || !strings.Contains(stderr, "unknown command") {
		t.Errorf("arr sonarr history: exit code %d, stderr %q", code, stderr)
	}
}
