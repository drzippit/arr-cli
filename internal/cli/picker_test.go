package cli_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/drzippit/arr-cli/internal/cli"
)

// pickAllBut makes the picker untick the options containing any of drop, and
// records what it was shown.
func pickAllBut(t *testing.T, shown *[]string, drop ...string) {
	t.Helper()
	cli.SetPicker(t, func(_ string, options []string) ([]int, error) {
		*shown = options
		var keep []int
		for i, o := range options {
			if !slices.ContainsFunc(drop, func(d string) bool { return strings.Contains(o, d) }) {
				keep = append(keep, i)
			}
		}
		return keep, nil
	})
}

func TestEditInATerminalLetsYouUntickTitlesBeforeAsking(t *testing.T) {
	srv, edits := fakeMediaAppForEdit(t, "sonarr", nil)
	useConfig(t, instanceDef{"sonarr", "sonarr", srv.URL, testAPIKey})
	var shown []string
	pickAllBut(t, &shown, "Twin Peaks")
	var asked string
	cli.SetConfirm(t, func(q string) (bool, error) { asked = q; return true, nil })

	out, stderr, code := run(t, "sonarr", "edit", "--all", "--set-quality", "Any")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	if len(shown) != 3 {
		t.Errorf("want all 3 Series offered, got %v", shown)
	}
	if asked != "Change 2 Series in sonarr: quality Any?" {
		t.Errorf("asked %q", asked)
	}
	if len(*edits) != 1 || !sameIDs((*edits)[0].SeriesIDs, 12, 7) {
		t.Errorf("want Shōgun and Severance only, got %+v", *edits)
	}
	if strings.Contains(out, "Twin Peaks") {
		t.Errorf("an unticked Title is not in the output:\n%s", out)
	}
}

func TestHuntInATerminalLetsYouUntickTitles(t *testing.T) {
	srv, cmds := fakeMediaAppForHunt(t, "radarr", nil)
	useConfig(t, instanceDef{"radarr", "radarr", srv.URL, testAPIKey})
	var shown []string
	pickAllBut(t, &shown, "The Matrix")
	cli.SetConfirm(t, func(string) (bool, error) { return true, nil })

	_, stderr, code := run(t, "radarr", "hunt", "--monitored")

	if code != 0 || len(*cmds) != 1 || !sameIDs((*cmds)[0].MovieIDs, 1) {
		t.Errorf("exit code %d, stderr %q, searches %+v; want only Good Will Hunting", code, stderr, *cmds)
	}
}

func TestPickingNothingOrCancellingSendsNothing(t *testing.T) {
	cases := []struct {
		name string
		pick func(string, []string) ([]int, error)
		code int
		want string
	}{
		{"untick everything", func(string, []string) ([]int, error) { return nil, nil }, 0, "nothing picked"},
		{"cancel", func(string, []string) ([]int, error) { return nil, errors.New("cancelled") }, 1, "cancelled"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, edits := fakeMediaAppForEdit(t, "sonarr", nil)
			useConfig(t, instanceDef{"sonarr", "sonarr", srv.URL, testAPIKey})
			cli.SetPicker(t, tc.pick)
			cli.SetConfirm(t, func(string) (bool, error) { t.Error("asked to confirm"); return true, nil })

			_, stderr, code := run(t, "sonarr", "edit", "--all", "--set-quality", "Any")

			if code != tc.code || !strings.Contains(stderr, tc.want) || len(*edits) != 0 {
				t.Errorf("exit code %d, stderr %q, %d edits", code, stderr, len(*edits))
			}
		})
	}
}

func TestNoPickerWithYesOrDryRun(t *testing.T) {
	for _, flag := range []string{"--yes", "--dry-run"} {
		srv, _ := fakeMediaAppForEdit(t, "sonarr", nil)
		useConfig(t, instanceDef{"sonarr", "sonarr", srv.URL, testAPIKey})
		cli.SetPicker(t, func(string, []string) ([]int, error) { t.Errorf("picker shown with %s", flag); return nil, nil })

		if _, stderr, code := run(t, "sonarr", "edit", "--all", "--set-quality", "Any", flag); code != 0 {
			t.Errorf("%s: exit code %d, stderr %s", flag, code, stderr)
		}
	}
}
