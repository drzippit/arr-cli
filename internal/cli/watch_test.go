package cli_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/drzippit/arr-cli/internal/cli"
)

const (
	altScreenOn  = "\x1b[?1049h"
	altScreenOff = "\x1b[?1049l"
)

func TestQueueWatchRedrawsOnTheAlternateScreenWithADownloadClientFooter(t *testing.T) {
	today(t)
	cli.SetTerminal(t, 200)
	cli.SetWatchFrames(t, 2)
	sonarrAndSabnzbd(t, true, true)

	out, stderr, code := run(t, "queue", "--watch")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	if !strings.HasPrefix(out, altScreenOn) || !strings.HasSuffix(out, altScreenOff) {
		t.Errorf("want the frames on the alternate screen, left again at the end: %q", out)
	}
	if n := strings.Count(out, "INSTANCE"); n != 2 {
		t.Errorf("want 2 frames, got %d:\n%s", n, out)
	}
	for _, want := range []string{"arr queue, every 2s", "Ctrl-C", "Severance", "sab: downloading at 2.0 MiB/s, 53m 45s left"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestQueueWatchShowsAnUnreachableInstanceInsteadOfStopping(t *testing.T) {
	today(t)
	cli.SetTerminal(t, 200)
	cli.SetWatchFrames(t, 2)
	sonarrAndSabnzbd(t, true, false)

	out, stderr, code := run(t, "sonarr", "queue", "-w")
	if code != 0 || !strings.Contains(out, "Severance") || !strings.Contains(out, "arr sonarr queue, every 2s") {
		t.Fatalf("arr sonarr queue -w: exit code %d, stderr %s\n%s", code, stderr, out)
	}

	out, stderr, code = run(t, "queue", "-w")

	if code != 0 {
		t.Errorf("exit code %d, want 0: Ctrl-C ends a watch, not a failure; stderr: %s", code, stderr)
	}
	if stderr != "" {
		t.Errorf("failures belong in the frame, not on stderr: %q", stderr)
	}
	if n := strings.Count(out, "sab: "); n != 2 {
		t.Errorf("want sab's failure in each of 2 frames, got %d:\n%s", n, out)
	}
}

func TestQueueWatchInterval(t *testing.T) {
	today(t)
	cli.SetTerminal(t, 200)
	cli.SetWatchFrames(t, 1)
	sonarrAndSabnzbd(t, true, true)

	out, stderr, code := run(t, "queue", "--watch", "--interval", "5s")

	if code != 0 || !strings.Contains(out, "every 5s") {
		t.Errorf("exit code %d, stderr %q:\n%s", code, stderr, out)
	}
}

func TestQueueWatchRefusesWhatItCannotDo(t *testing.T) {
	sonarrAndSabnzbd(t, true, true)
	cases := []struct {
		name     string
		terminal bool
		args     []string
		want     string
	}{
		{"no terminal", false, []string{"queue", "--watch"}, "--watch needs a terminal"},
		{"json", true, []string{"queue", "--watch", "--json"}, "--watch and --json cannot be used together"},
		{"short interval", true, []string{"queue", "--watch", "--interval", "500ms"}, "--interval must be 1s or more"},
		{"bad interval", true, []string{"queue", "--watch", "--interval", "soon"}, "--interval"},
		{"interval without watch", true, []string{"queue", "--interval", "5s"}, "--interval needs --watch"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.terminal {
				cli.SetTerminal(t, 200)
			}
			out, stderr, code := run(t, tc.args...)
			if code != 1 || !strings.Contains(stderr, tc.want) {
				t.Errorf("exit code %d, stderr %q, want %q", code, stderr, tc.want)
			}
			if strings.Contains(out, altScreenOn) {
				t.Errorf("drew before refusing: %q", out)
			}
		})
	}
}

func TestQueueOfPausedSabnzbdHasNoTimeLeft(t *testing.T) {
	today(t)
	cli.SetTerminal(t, 200)
	cli.SetWatchFrames(t, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("mode") != "queue" {
			sabnzbdHandler(t).ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"queue": {"version": "5.1.3", "status": "Paused", "kbpersec": "0.00", "timeleft": "0:00:00",
			"diskspace1": "10", "diskspace2": "10", "slots": [{"index": 0, "nzo_id": "SABnzbd_nzo_x", "filename": "Paused.Thing",
			"cat": "*", "priority": "Normal", "mb": "100", "mbleft": "100", "percentage": "0", "status": "Paused", "timeleft": "0:00:00"}]}}`)
	}))
	t.Cleanup(srv.Close)
	useConfig(t, instanceDef{"sab", "sabnzbd", srv.URL, testAPIKey})

	out, stderr, code := run(t, "sab", "queue", "--json")
	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	if strings.Contains(out, `"timeLeft"`) {
		t.Errorf("0:00:00 means SABnzbd doesn't know, so there should be no timeLeft:\n%s", out)
	}

	out, _, _ = run(t, "sab", "queue", "--watch")
	if !strings.Contains(out, "sab: paused at 0 B/s\n") {
		t.Errorf("want the footer without a time left:\n%s", out)
	}
}
