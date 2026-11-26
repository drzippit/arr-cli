package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/drzippit/arr-cli/internal/config"
)

func TestAppendKeepsExistingEntriesAndComments(t *testing.T) {
	path := writeConfig(t, `# my arr stack
[[instance]]
name = "sonarr"
app = "sonarr"
url = "http://sonarr.lan:8989"
api_key = "abc123"
quality_profile = "HD-1080p" # the usual one`)

	err := config.Append(path, config.Instance{Name: "radarr", App: config.Radarr, URL: "http://radarr.lan:7878", APIKeyCmd: `pass show "radarr"`})
	if err != nil {
		t.Fatalf("Append: %v", err)
	}

	body, _ := os.ReadFile(path)
	for _, want := range []string{"# my arr stack", `quality_profile = "HD-1080p" # the usual one`} {
		if !strings.Contains(string(body), want) {
			t.Errorf("config lost %q:\n%s", want, body)
		}
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load after Append: %v\n%s", err, body)
	}
	want := []config.Instance{
		{Name: "sonarr", App: config.Sonarr, URL: "http://sonarr.lan:8989", APIKey: "abc123", QualityProfile: "HD-1080p"},
		{Name: "radarr", App: config.Radarr, URL: "http://radarr.lan:7878", APIKeyCmd: `pass show "radarr"`},
	}
	if len(cfg.Instances) != 2 || cfg.Instances[0] != want[0] || cfg.Instances[1] != want[1] {
		t.Errorf("Instances after Append:\n%+v\nwant:\n%+v", cfg.Instances, want)
	}
}

func TestAppendCreatesMissingFileAndFolderReadableOnlyByOwner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "arr-cli", "config.toml")

	err := config.Append(path, config.Instance{Name: "sonarr", App: config.Sonarr, URL: "http://sonarr.lan:8989", APIKey: "abc123"})
	if err != nil {
		t.Fatalf("Append: %v", err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load after Append: %v", err)
	}
	if len(cfg.Instances) != 1 || cfg.Instances[0].Name != "sonarr" || cfg.Instances[0].APIKey != "abc123" {
		t.Errorf("Instances after Append: %+v", cfg.Instances)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("config file mode %o, want 600 since it holds API keys", perm)
	}
}

func TestAppendWritesThroughSymlinkedConfig(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "dotfiles", "arr.toml")
	if err := os.MkdirAll(filepath.Dir(real), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(real, []byte("# kept in dotfiles\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "config.toml")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}

	if err := config.Append(link, config.Instance{Name: "sonarr", App: config.Sonarr, URL: "http://sonarr.lan:8989", APIKey: "abc123"}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("config should still be a symlink, got %v, %v", info, err)
	}
	body, _ := os.ReadFile(real)
	if !strings.Contains(string(body), `name = "sonarr"`) {
		t.Errorf("the linked file should hold the new Instance:\n%s", body)
	}
}
