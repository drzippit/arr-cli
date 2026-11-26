package cli_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestStatusWithoutInstanceShowsEveryInstanceInOneTable(t *testing.T) {
	sonarr := fakeInstance(t, "sonarr")
	radarr := fakeInstance(t, "radarr")
	useConfig(t,
		instanceDef{"sonarr", "sonarr", sonarr.URL, testAPIKey},
		instanceDef{"radarr", "radarr", radarr.URL, testAPIKey},
	)

	out, stderr, code := run(t, "status")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	if n := strings.Count(out, "INSTANCE"); n != 1 {
		t.Errorf("want one combined table, got %d header rows:\n%s", n, out)
	}
	sonarrAt := strings.Index(out, "4.0.11.2680")
	radarrAt := strings.Index(out, "5.14.0.9383")
	if sonarrAt < 0 || radarrAt < 0 {
		t.Fatalf("want both Instances' versions:\n%s", out)
	}
	if sonarrAt > radarrAt {
		t.Errorf("Instances should appear in config order:\n%s", out)
	}
}

// barrier blocks each system/status request until n of them have arrived, so
// requests only succeed when every Instance is asked at the same time.
type barrier struct {
	arrived chan struct{}
	release chan struct{}
	once    sync.Once
	n       int
}

func newBarrier(n int) *barrier {
	return &barrier{arrived: make(chan struct{}, n), release: make(chan struct{}), n: n}
}

func (b *barrier) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v3/system/status" {
			b.arrived <- struct{}{}
			if len(b.arrived) == b.n {
				b.once.Do(func() { close(b.release) })
			}
			select {
			case <-b.release:
			case <-time.After(2 * time.Second):
				http.Error(w, "requests were not concurrent", http.StatusServiceUnavailable)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func TestStatusAsksEveryInstanceAtTheSameTime(t *testing.T) {
	b := newBarrier(2)
	sonarr := httptest.NewServer(b.wrap(recordedHandler(t, "sonarr")))
	t.Cleanup(sonarr.Close)
	radarr := httptest.NewServer(b.wrap(recordedHandler(t, "radarr")))
	t.Cleanup(radarr.Close)
	useConfig(t,
		instanceDef{"sonarr", "sonarr", sonarr.URL, testAPIKey},
		instanceDef{"radarr", "radarr", radarr.URL, testAPIKey},
	)

	_, stderr, code := run(t, "status")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
}

func TestStatusStillShowsOtherInstancesWhenOneIsDown(t *testing.T) {
	radarr := fakeInstance(t, "radarr")
	down := fakeInstance(t, "sonarr")
	downURL := down.URL
	down.Close()
	useConfig(t,
		instanceDef{"sonarr", "sonarr", downURL, testAPIKey},
		instanceDef{"radarr", "radarr", radarr.URL, testAPIKey},
	)

	out, stderr, code := run(t, "status")

	if code != 1 {
		t.Errorf("exit code %d, want 1 when an Instance fails", code)
	}
	if !strings.Contains(out, "5.14.0.9383") {
		t.Errorf("radarr's status should still print:\n%s", out)
	}
	if !strings.Contains(out, "sonarr") {
		t.Errorf("the failed Instance should still have a row:\n%s", out)
	}
	if !strings.HasPrefix(stderr, "arr: sonarr: ") || strings.Contains(stderr, "radarr") {
		t.Errorf("stderr should name only the failed Instance, got %q", stderr)
	}
}

func TestStatusJSONIsOneFlatArrayAcrossInstances(t *testing.T) {
	sonarr := fakeInstance(t, "sonarr")
	radarr := fakeInstance(t, "radarr")
	useConfig(t,
		instanceDef{"sonarr", "sonarr", sonarr.URL, testAPIKey},
		instanceDef{"radarr-4k", "radarr", radarr.URL, testAPIKey},
	)

	out, stderr, code := run(t, "status", "--json")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	want := `[
  {
    "instance": "sonarr",
    "app": "sonarr",
    "reachable": true,
    "version": "4.0.11.2680",
    "supported": true,
    "warnings": 2
  },
  {
    "instance": "radarr-4k",
    "app": "radarr",
    "reachable": true,
    "version": "5.14.0.9383",
    "supported": true,
    "warnings": 0
  }
]
`
	if out != want {
		t.Errorf("output:\n%s\nwant:\n%s", out, want)
	}
}
