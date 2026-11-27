package cli_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestQueueShowsSonarrDownloadsAsTable(t *testing.T) {
	srv := fakeInstance(t, "sonarr")
	useConfig(t, instanceDef{"sonarr", "sonarr", srv.URL, testAPIKey})

	out, stderr, code := run(t, "sonarr", "queue")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	for _, want := range []string{
		"INSTANCE", "TITLE", "EPISODE", "PROGRESS", "ETA", "STATUS", "CLIENT",
		"Severance", "S02E07 Chikhai Bardo", "25%", "12m 34s", "downloading", "SABnzbd",
		"Shōgun", "S01E10 A Dream of a Dream", "0%", "queued", "qBittorrent",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestQueueShowsRadarrMovieWithoutEpisode(t *testing.T) {
	srv := fakeInstance(t, "radarr")
	useConfig(t, instanceDef{"radarr", "radarr", srv.URL, testAPIKey})

	out, stderr, code := run(t, "radarr", "queue")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	for _, want := range []string{"Good Will Hunting (1997)", "90%", "1d 2h 3m", "warning", "qBittorrent"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "SAMPLEGRP") {
		t.Errorf("table should show the Title, not the release name:\n%s", out)
	}
}

func TestQueueWithoutInstanceCoversEveryInstanceDespiteFailures(t *testing.T) {
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

	out, stderr, code := run(t, "queue")

	if code != 1 {
		t.Errorf("exit code %d, want 1 when an Instance fails", code)
	}
	if n := strings.Count(out, "INSTANCE"); n != 1 {
		t.Errorf("want one combined table, got %d header rows:\n%s", n, out)
	}
	sonarrAt := strings.Index(out, "Severance")
	radarrAt := strings.Index(out, "Good Will Hunting")
	if sonarrAt < 0 || radarrAt < 0 {
		t.Fatalf("want downloads from both working Instances:\n%s", out)
	}
	if sonarrAt > radarrAt {
		t.Errorf("Instances should appear in config order:\n%s", out)
	}
	if !strings.HasPrefix(stderr, "arr: radarr-old: ") || strings.Count(stderr, "\n") != 1 {
		t.Errorf("stderr should name only the failed Instance, got %q", stderr)
	}
}

func TestQueueJSONIsFlatArrayWithInstanceAndApp(t *testing.T) {
	sonarr := fakeInstance(t, "sonarr")
	radarr := fakeInstance(t, "radarr")
	useConfig(t,
		instanceDef{"sonarr", "sonarr", sonarr.URL, testAPIKey},
		instanceDef{"radarr-4k", "radarr", radarr.URL, testAPIKey},
	)

	out, stderr, code := run(t, "queue", "--json")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	want := `[
  {
    "instance": "sonarr",
    "app": "sonarr",
    "title": "Severance",
    "season": 2,
    "episode": 7,
    "episodeTitle": "Chikhai Bardo",
    "release": "Severance.S02E07.1080p.WEB-DL-EXAMPLE",
    "progress": 25,
    "size": 2147483648,
    "sizeLeft": 1610612736,
    "timeLeft": "00:12:34",
    "estimatedCompletionTime": "2026-11-26T14:12:34Z",
    "status": "downloading",
    "downloadClient": "SABnzbd",
    "protocol": "usenet",
    "downloadId": "SABnzbd_nzo_abc123"
  },
  {
    "instance": "sonarr",
    "app": "sonarr",
    "title": "Shōgun",
    "season": 1,
    "episode": 10,
    "episodeTitle": "A Dream of a Dream",
    "release": "Shogun.2024.S01E10.2160p.WEB-DL-EXAMPLE",
    "progress": 0,
    "size": 4294967296,
    "sizeLeft": 4294967296,
    "status": "queued",
    "downloadClient": "qBittorrent",
    "protocol": "torrent",
    "downloadId": "A1B2C3D4E5F6"
  },
  {
    "instance": "radarr-4k",
    "app": "radarr",
    "title": "Good Will Hunting",
    "year": 1997,
    "release": "Good.Will.Hunting.1997.1080p.WEB-DL-SAMPLEGRP",
    "progress": 90,
    "size": 10737418240,
    "sizeLeft": 1073741824,
    "timeLeft": "1.02:03:04",
    "estimatedCompletionTime": "2026-11-27T16:03:04Z",
    "status": "warning",
    "downloadClient": "qBittorrent",
    "protocol": "torrent",
    "downloadId": "F6E5D4C3B2A1"
  }
]
`
	if out != want {
		t.Errorf("output:\n%s\nwant:\n%s", out, want)
	}
}

func TestQueueJSONIsEmptyArrayWhenNothingDownloads(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"page":1,"pageSize":1000,"totalRecords":0,"records":[]}`))
	}))
	t.Cleanup(srv.Close)
	useConfig(t, instanceDef{"sonarr", "sonarr", srv.URL, testAPIKey})

	out, stderr, code := run(t, "sonarr", "queue", "--json")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	if out != "[]\n" {
		t.Errorf("output = %q, want an empty array", out)
	}
}

func TestQueueFollowsPages(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		page := r.URL.Query().Get("page")
		fmt.Fprintf(w, `{"page":%s,"totalRecords":2,"records":[{"title":"release-%s","movie":{"title":"Movie %s","year":2000}}]}`, page, page, page)
	}))
	t.Cleanup(srv.Close)
	useConfig(t, instanceDef{"radarr", "radarr", srv.URL, testAPIKey})

	out, stderr, code := run(t, "radarr", "queue")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	for _, want := range []string{"Movie 1", "Movie 2"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestQueueOffersNoActions(t *testing.T) {
	useConfig(t, instanceDef{"sonarr", "sonarr", "http://127.0.0.1:1", testAPIKey})

	for _, action := range []string{"remove", "blocklist", "retry"} {
		_, _, code := run(t, "sonarr", "queue", action)
		if code == 0 {
			t.Errorf("`arr sonarr queue %s` succeeded; queue should only display", action)
		}
	}
}

func TestQueueShowsWhyAFinishedDownloadIsNotImported(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"totalRecords":1,"records":[{"title":"x","movie":{"title":"Heat","year":1995},
			"size":100,"sizeleft":0,"timeleft":"00:00:00","status":"completed",
			"trackedDownloadStatus":"warning","trackedDownloadState":"importBlocked","downloadClient":"qBittorrent"}]}`))
	}))
	t.Cleanup(srv.Close)
	useConfig(t, instanceDef{"radarr", "radarr", srv.URL, testAPIKey})

	out, stderr, code := run(t, "radarr", "queue")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	if !strings.Contains(out, "importBlocked") {
		t.Errorf("status should say the import is blocked:\n%s", out)
	}
}

func TestQueueNeverShowsUnfinishedDownloadAsComplete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"totalRecords":1,"records":[{"title":"x","movie":{"title":"Heat","year":1995},
			"size":1000,"sizeleft":4,"timeleft":"00:00:01","status":"downloading"}]}`))
	}))
	t.Cleanup(srv.Close)
	useConfig(t, instanceDef{"radarr", "radarr", srv.URL, testAPIKey})

	out, _, _ := run(t, "radarr", "queue")

	if strings.Contains(out, "100%") || !strings.Contains(out, "99%") {
		t.Errorf("a download with bytes left should show 99%%:\n%s", out)
	}
}
