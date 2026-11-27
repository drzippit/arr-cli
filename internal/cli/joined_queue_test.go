package cli_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// queueRecords decodes arr queue --json output.
func queueRecords(t *testing.T, out string) []map[string]any {
	t.Helper()
	var recs []map[string]any
	if err := json.Unmarshal([]byte(out), &recs); err != nil {
		t.Fatalf("not a JSON list: %v\n%s", err, out)
	}
	return recs
}

// withDownloadID returns the records whose downloadId is id.
func withDownloadID(recs []map[string]any, id string) []map[string]any {
	var out []map[string]any
	for _, r := range recs {
		if r["downloadId"] == id {
			out = append(out, r)
		}
	}
	return out
}

// sonarrAndSabnzbd configures a Sonarr and a SABnzbd Instance, either of
// which can be down.
func sonarrAndSabnzbd(t *testing.T, sonarrUp, sabUp bool) {
	t.Helper()
	sonarr, sab := fakeInstance(t, "sonarr"), fakeSabnzbd(t)
	if !sonarrUp {
		sonarr.Close()
	}
	if !sabUp {
		sab.Close()
	}
	useConfig(t,
		instanceDef{"sonarr", "sonarr", sonarr.URL, testAPIKey},
		instanceDef{"sab", "sabnzbd", sab.URL, testAPIKey},
	)
}

func TestQueueShowsADownloadInBothAppsOnceWithLiveValuesFromTheDownloadClient(t *testing.T) {
	sonarrAndSabnzbd(t, true, true)

	out, stderr, code := run(t, "queue", "--json")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	recs := queueRecords(t, out)
	joined := withDownloadID(recs, "SABnzbd_nzo_abc123")
	if len(joined) != 1 {
		t.Fatalf("want the Download once, got %d rows:\n%s", len(joined), out)
	}
	want := map[string]any{
		// From Sonarr: what the Download is for.
		"instance": "sonarr", "app": "sonarr", "title": "Severance", "season": float64(2), "episode": float64(7),
		"episodeTitle": "Chikhai Bardo",
		// From SABnzbd: how it is going.
		"downloadClientInstance": "sab", "progress": float64(40), "sizeLeft": float64(2400 << 20),
		"timeLeft": "00:25:00", "status": "downloading", "priority": "high", "category": "tv", "position": float64(1),
	}
	for k, v := range want {
		if joined[0][k] != v {
			t.Errorf("%q = %v, want %v", k, joined[0][k], v)
		}
	}
}

func TestQueueShowsDownloadsNoMediaAppGrabbedUnderTheirOwnName(t *testing.T) {
	sonarrAndSabnzbd(t, true, true)

	out, stderr, code := run(t, "queue")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	for _, want := range []string{"Severance", "Shōgun", "qBittorrent", "Some.Linux.ISO.x64", "The.Bear.S03E01.1080p.WEB-DL-EXAMPLE"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Severance.S02E07") {
		t.Errorf("the joined Download should show as its Title, not its NZB name:\n%s", out)
	}
	assertInOrder(t, out, "Severance", "Shōgun", "Some.Linux.ISO.x64", "The.Bear.S03E01")
}

func TestQueueJoinedStatusIsTheMediaAppsImportStateWhenItHasOne(t *testing.T) {
	sonarr := httptest.NewServer(sonarrQueueOf(t, `{"totalRecords": 1, "records": [{
		"title": "The.Bear.S03E01.1080p.WEB-DL-EXAMPLE", "series": {"title": "The Bear"},
		"episode": {"seasonNumber": 3, "episodeNumber": 1, "title": "Tomorrow"},
		"size": 2147483648, "sizeleft": 0, "status": "completed", "trackedDownloadState": "importBlocked",
		"downloadId": "SABnzbd_nzo_pp789", "downloadClient": "SABnzbd", "protocol": "usenet"}]}`))
	t.Cleanup(sonarr.Close)
	sab := fakeSabnzbd(t)
	useConfig(t,
		instanceDef{"sonarr", "sonarr", sonarr.URL, testAPIKey},
		instanceDef{"sab", "sabnzbd", sab.URL, testAPIKey},
	)

	out, stderr, code := run(t, "queue", "--json")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	joined := withDownloadID(queueRecords(t, out), "SABnzbd_nzo_pp789")
	if len(joined) != 1 || joined[0]["status"] != "importBlocked" || joined[0]["title"] != "The Bear" {
		t.Errorf("want one The Bear row, importBlocked, got %v", joined)
	}
}

// sonarrQueueOf answers Sonarr's queue with body.
func sonarrQueueOf(t *testing.T, body string) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/queue" {
			t.Errorf("unexpected request to %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, body)
	})
}

func TestQueueWithDownloadClientDownShowsMediaAppRowsUnjoined(t *testing.T) {
	sonarrAndSabnzbd(t, true, false)

	out, stderr, code := run(t, "queue", "--json")

	if code != 1 {
		t.Errorf("exit code %d, want 1", code)
	}
	if !strings.HasPrefix(stderr, "arr: sab: ") {
		t.Errorf("stderr should name sab: %q", stderr)
	}
	rows := withDownloadID(queueRecords(t, out), "SABnzbd_nzo_abc123")
	if len(rows) != 1 || rows[0]["progress"] != float64(25) || rows[0]["priority"] != nil {
		t.Errorf("want Sonarr's own row, without SABnzbd's values: %v", rows)
	}
}

func TestQueueWithMediaAppDownShowsDownloadClientRowsUnderTheirOwnName(t *testing.T) {
	sonarrAndSabnzbd(t, false, true)

	out, stderr, code := run(t, "queue")

	if code != 1 {
		t.Errorf("exit code %d, want 1", code)
	}
	if !strings.HasPrefix(stderr, "arr: sonarr: ") {
		t.Errorf("stderr should name sonarr: %q", stderr)
	}
	if !strings.Contains(out, "Severance.S02E07.1080p.WEB-DL-EXAMPLE") {
		t.Errorf("want SABnzbd's row under its NZB name:\n%s", out)
	}
}

func TestQueueOfOneMediaAppInstanceIsNotJoined(t *testing.T) {
	sonarrAndSabnzbd(t, true, true)

	out, stderr, code := run(t, "sonarr", "queue", "--json")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	rows := withDownloadID(queueRecords(t, out), "SABnzbd_nzo_abc123")
	if len(rows) != 1 || rows[0]["progress"] != float64(25) || rows[0]["downloadClientInstance"] != nil {
		t.Errorf("arr sonarr queue should show only what Sonarr says: %v", rows)
	}
}
