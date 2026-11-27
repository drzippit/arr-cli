package release

import (
	"runtime/debug"
	"testing"
)

func buildInfo(version string, settings ...string) *debug.BuildInfo {
	bi := &debug.BuildInfo{Main: debug.Module{Path: "github.com/drzippit/arr-cli", Version: version}}
	for i := 0; i+1 < len(settings); i += 2 {
		bi.Settings = append(bi.Settings, debug.BuildSetting{Key: settings[i], Value: settings[i+1]})
	}
	return bi
}

const rev = "52b07aa9717d79fc46d2358d562bc96eb29a30ea"

func TestStampedTagWinsOverBuildInfo(t *testing.T) {
	got := resolve("v0.2.0", buildInfo("v0.1.0", "vcs.revision", rev, "vcs.modified", "false"))
	if got.Version != "v0.2.0" || got.Commit != "52b07aa" || got.Dirty == nil || *got.Dirty {
		t.Errorf("got %+v", got)
	}
	if got.String() != "v0.2.0" {
		t.Errorf("a Release should print as its tag, got %q", got.String())
	}
}

func TestGoInstallOfATagReportsThatRelease(t *testing.T) {
	got := resolve("", buildInfo("v0.2.0"))
	if got.Version != "v0.2.0" || got.Commit != "" || got.Dirty != nil {
		t.Errorf("got %+v", got)
	}
}

func TestPreReleaseTagFromBuildInfoIsARelease(t *testing.T) {
	if got := resolve("", buildInfo("v0.2.0-rc1")); got.Version != "v0.2.0-rc1" {
		t.Errorf("got %+v", got)
	}
}

func TestLocalBuildIsDevWithCommit(t *testing.T) {
	for _, v := range []string{
		"v0.1.1-0.20261126224139-52b07aa9717d+dirty",
		"v0.1.1-0.20261126224139-52b07aa9717d",
		"v0.2.0-rc1.0.20261126224139-52b07aa9717d",
		"v0.2.0+dirty",
		"(devel)",
		"",
	} {
		got := resolve("", buildInfo(v, "vcs.revision", rev, "vcs.modified", "true"))
		if got.Version != "dev" || got.Commit != "52b07aa" || got.Dirty == nil || !*got.Dirty {
			t.Errorf("module version %q: got %+v", v, got)
		}
		if got.String() != "dev (52b07aa-dirty)" {
			t.Errorf("module version %q: got %q", v, got.String())
		}
	}
}

func TestDevWithoutVCSInfoIsPlainDev(t *testing.T) {
	got := resolve("", buildInfo("(devel)"))
	if got.String() != "dev" || got.Commit != "" || got.Dirty != nil {
		t.Errorf("got %+v, %q", got, got.String())
	}
	if got := resolve("", nil); got.String() != "dev" {
		t.Errorf("no build info: got %q", got.String())
	}
}

func TestCleanDevBuildHasNoDirtySuffix(t *testing.T) {
	got := resolve("", buildInfo("(devel)", "vcs.revision", rev, "vcs.modified", "false"))
	if got.String() != "dev (52b07aa)" {
		t.Errorf("got %q", got.String())
	}
}
