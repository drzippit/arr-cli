package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/drzippit/arr-cli/internal/cli"
)

// runWithStdin runs arr with args and stdin as its standard input.
func runWithStdin(t *testing.T, stdin string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = cli.RunWithStdin(args, strings.NewReader(stdin), &out, &errOut)
	return out.String(), errOut.String(), code
}

func TestEditReadsTheSelectionsIDsFromStdinOnePerLine(t *testing.T) {
	srv, edits := fakeMediaAppForEdit(t, "sonarr", nil)
	useConfig(t, instanceDef{"sonarr", "sonarr", srv.URL, testAPIKey})

	// Shōgun and Twin Peaks by TVDB ID, with a blank line.
	_, stderr, code := runWithStdin(t, "392573\n\n70533\n", "sonarr", "edit", "-", "--set-monitored=true", "--yes")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	if len(*edits) != 1 || !sameIDs((*edits)[0].SeriesIDs, 12, 3) {
		t.Errorf("want Shōgun (12) and Twin Peaks (3) edited, got %+v", *edits)
	}
}

// sameIDs reports whether got holds exactly want, in any order.
func sameIDs(got []int, want ...int) bool {
	if len(got) != len(want) {
		return false
	}
	seen := map[int]bool{}
	for _, id := range got {
		seen[id] = true
	}
	for _, id := range want {
		if !seen[id] {
			return false
		}
	}
	return true
}

func TestEditReadsTheRecordsArrListJSONPrints(t *testing.T) {
	srv, edits := fakeMediaAppForEdit(t, "radarr", nil)
	useConfig(t, instanceDef{"radarr", "radarr", srv.URL, testAPIKey})
	listed, _, _ := run(t, "radarr", "list", "--json", "--monitored")

	_, stderr, code := runWithStdin(t, listed, "radarr", "edit", "-", "--set-quality", "Any", "--yes")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	if len(*edits) != 1 || !sameIDs((*edits)[0].MovieIDs, 1, 2) {
		t.Errorf("want The Matrix (2) and Good Will Hunting (1), got %+v", *edits)
	}
}

func TestStdinSelectionIsRefusedBeforeAnythingIsSent(t *testing.T) {
	cases := []struct {
		name, stdin string
		args        []string
		want        string
	}{
		{"another Instance's records", `[{"instance": "radarr-4k", "app": "radarr", "title": "The Matrix", "tmdbId": 603}]`,
			[]string{"--set-quality", "Any", "--yes"}, `from Instance "radarr-4k"`},
		{"an ID not in the Library", "603\n999999\n", []string{"--set-quality", "Any", "--yes"}, "no Title with ID 999999"},
		{"not an ID", "the matrix\n", []string{"--set-quality", "Any", "--yes"}, `"the matrix" is not a TVDB or TMDB ID`},
		{"without --yes or --dry-run", "603\n", []string{"--set-quality", "Any"}, "with -, pass --yes or --dry-run"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, edits := fakeMediaAppForEdit(t, "radarr", nil)
			useConfig(t, instanceDef{"radarr", "radarr", srv.URL, testAPIKey})
			cli.SetConfirm(t, func(string) (bool, error) { t.Error("asked a question with -"); return true, nil })

			_, stderr, code := runWithStdin(t, tc.stdin, append([]string{"radarr", "edit", "-"}, tc.args...)...)

			if code != 1 || !strings.Contains(stderr, tc.want) || len(*edits) != 0 {
				t.Errorf("exit code %d, stderr %q, %d edits; want %q", code, stderr, len(*edits), tc.want)
			}
		})
	}
}

func TestFiltersNarrowTheIDsFromStdin(t *testing.T) {
	srv, cmds := fakeMediaAppForHunt(t, "radarr", nil)
	useConfig(t, instanceDef{"radarr", "radarr", srv.URL, testAPIKey})

	// All three Movies, but only Good Will Hunting is Missing.
	_, stderr, code := runWithStdin(t, "603\n489\n687163\n", "radarr", "hunt", "-", "--missing", "--yes")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	if len(*cmds) != 1 || !sameIDs((*cmds)[0].MovieIDs, 1) {
		t.Errorf("want only Good Will Hunting (1) hunted, got %+v", *cmds)
	}
}

func TestAnIDGivenTwiceOnStdinCountsOnce(t *testing.T) {
	srv, cmds := fakeMediaAppForHunt(t, "sonarr", nil)
	useConfig(t, instanceDef{"sonarr", "sonarr", srv.URL, testAPIKey})

	_, stderr, code := runWithStdin(t, "70533\n392573\n70533\n", "sonarr", "hunt", "-", "--yes")

	if code != 0 || len(*cmds) != 2 {
		t.Errorf("exit code %d, stderr %q, want 2 searches, got %+v", code, stderr, *cmds)
	}
}

func TestStdinJSONRecordsMayLeaveOutTheirInstance(t *testing.T) {
	srv, edits := fakeMediaAppForEdit(t, "radarr", nil)
	useConfig(t, instanceDef{"radarr", "radarr", srv.URL, testAPIKey})

	// As from jq 'map({tmdbId})'.
	_, stderr, code := runWithStdin(t, `[{"tmdbId": 603}]`, "radarr", "edit", "-", "--set-quality", "Any", "--yes")

	if code != 0 || len(*edits) != 1 || !sameIDs((*edits)[0].MovieIDs, 2) {
		t.Errorf("exit code %d, stderr %q, edits %+v", code, stderr, *edits)
	}
}

func TestStdinJSONRecordWithoutAnIDIsRefused(t *testing.T) {
	srv, edits := fakeMediaAppForEdit(t, "radarr", nil)
	useConfig(t, instanceDef{"radarr", "radarr", srv.URL, testAPIKey})

	_, stderr, code := runWithStdin(t, `[{"tmdbId": 603}, {"title": "The Matrix"}]`, "radarr", "edit", "-", "--set-quality", "Any", "--yes")

	if code != 1 || !strings.Contains(stderr, "stdin record 2 has no tvdbId or tmdbId") || len(*edits) != 0 {
		t.Errorf("exit code %d, stderr %q, %d edits", code, stderr, len(*edits))
	}
}
