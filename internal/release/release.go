// Package release says which arr version is running (ADR 0004).
package release

import (
	"regexp"
	"runtime/debug"
	"sync"
)

// tag is the arr version's tag, stamped in by `make dist` with
// -ldflags '-X github.com/drzippit/arr-cli/internal/release.tag=v0.2.0'.
var tag string

// Info is the running build of arr-cli.
type Info struct {
	// Version is the arr version's tag, such as v0.2.0, or dev for any other build.
	Version string `json:"version"`
	// Commit and Dirty come from Go's VCS stamp and are unset without one.
	Commit string `json:"commit,omitempty"`
	Dirty  *bool  `json:"dirty,omitempty"`
}

// String gives the arr version's tag, or dev with the commit for any other build,
// as in "dev (52b07aa-dirty)".
func (i Info) String() string {
	if i.Version != "dev" || i.Commit == "" {
		return i.Version
	}
	s := "dev (" + i.Commit
	if i.Dirty != nil && *i.Dirty {
		s += "-dirty"
	}
	return s + ")"
}

// Current returns the running build.
var Current = sync.OnceValue(func() Info {
	bi, _ := debug.ReadBuildInfo()
	return resolve(tag, bi)
})

// releaseTag matches a SemVer tag. Build metadata such as +dirty is left out
// on purpose, so a modified build never passes for an arr version.
var releaseTag = regexp.MustCompile(`^v\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$`)

// pseudoVersion matches the timestamp-and-commit tail Go gives a build that is
// not exactly at a tag, as in v0.1.1-0.20261126224139-52b07aa9717d.
var pseudoVersion = regexp.MustCompile(`\d{14}-[0-9a-f]{12}$`)

// resolve takes the Version from the stamped tag, else from a module version
// that is an arr version's tag (as with go install …@v0.2.0), else dev.
func resolve(tag string, bi *debug.BuildInfo) Info {
	info := Info{Version: "dev"}
	if bi != nil {
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				info.Commit = s.Value[:min(7, len(s.Value))]
			case "vcs.modified":
				dirty := s.Value == "true"
				info.Dirty = &dirty
			}
		}
	}
	switch {
	case tag != "":
		info.Version = tag
	case bi != nil && releaseTag.MatchString(bi.Main.Version) && !pseudoVersion.MatchString(bi.Main.Version):
		info.Version = bi.Main.Version
	}
	return info
}
