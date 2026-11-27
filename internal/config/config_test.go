package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/drzippit/arr-cli/internal/config"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadReadsInstancesInOrder(t *testing.T) {
	path := writeConfig(t, `
[[instance]]
name = "sonarr"
app = "sonarr"
url = "http://sonarr.lan:8989"
api_key = "abc123"

[[instance]]
name = "radarr-4k"
app = "radarr"
url = "http://radarr.lan:7878"
api_key_cmd = "pass show radarr"
`)

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	want := []config.Instance{
		{Name: "sonarr", App: "sonarr", URL: "http://sonarr.lan:8989", APIKey: "abc123"},
		{Name: "radarr-4k", App: "radarr", URL: "http://radarr.lan:7878", APIKeyCmd: "pass show radarr"},
	}
	if len(cfg.Instances) != len(want) {
		t.Fatalf("got %d instances, want %d", len(cfg.Instances), len(want))
	}
	for i := range want {
		if cfg.Instances[i] != want[i] {
			t.Errorf("instance %d = %+v, want %+v", i, cfg.Instances[i], want[i])
		}
	}
}

func TestLoadRejectsInvalidInstances(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		wantErr string
	}{
		{
			name:    "missing name",
			body:    "[[instance]]\napp = \"sonarr\"\nurl = \"http://x\"\napi_key = \"k\"\n",
			wantErr: "name",
		},
		{
			name:    "unknown app",
			body:    "[[instance]]\nname = \"lidarr\"\napp = \"lidarr\"\nurl = \"http://x\"\napi_key = \"k\"\n",
			wantErr: `app "lidarr"`,
		},
		{
			name:    "missing url",
			body:    "[[instance]]\nname = \"sonarr\"\napp = \"sonarr\"\napi_key = \"k\"\n",
			wantErr: "url",
		},
		{
			name:    "no api key",
			body:    "[[instance]]\nname = \"sonarr\"\napp = \"sonarr\"\nurl = \"http://x\"\n",
			wantErr: "api_key",
		},
		{
			name:    "both api key and command",
			body:    "[[instance]]\nname = \"sonarr\"\napp = \"sonarr\"\nurl = \"http://x\"\napi_key = \"k\"\napi_key_cmd = \"echo k\"\n",
			wantErr: "api_key",
		},
		{
			name:    "duplicate name",
			body:    "[[instance]]\nname = \"sonarr\"\napp = \"sonarr\"\nurl = \"http://x\"\napi_key = \"k\"\n[[instance]]\nname = \"sonarr\"\napp = \"sonarr\"\nurl = \"http://y\"\napi_key = \"k\"\n",
			wantErr: "duplicate",
		},
		{
			name:    "quality profile on a download client",
			body:    "[[instance]]\nname = \"sab\"\napp = \"sabnzbd\"\nurl = \"http://x\"\napi_key = \"k\"\nquality_profile = \"HD-1080p\"\n",
			wantErr: `instance "sab": quality_profile is for adding Titles to Sonarr or Radarr`,
		},
		{
			name:    "root folder on a download client",
			body:    "[[instance]]\nname = \"sab\"\napp = \"sabnzbd\"\nurl = \"http://x\"\napi_key = \"k\"\nroot_folder = \"/tv\"\n",
			wantErr: "root_folder",
		},
		{
			name:    "monitor on a download client",
			body:    "[[instance]]\nname = \"sab\"\napp = \"sabnzbd\"\nurl = \"http://x\"\napi_key = \"k\"\nmonitor = \"all\"\n",
			wantErr: "monitor",
		},
		{
			name:    "minimum availability on a download client",
			body:    "[[instance]]\nname = \"sab\"\napp = \"sabnzbd\"\nurl = \"http://x\"\napi_key = \"k\"\nminimum_availability = \"released\"\n",
			wantErr: "minimum_availability",
		},
		{
			name:    "malformed toml",
			body:    "[[instance]\n",
			wantErr: "parse",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := config.Load(writeConfig(t, tc.body))
			if err == nil {
				t.Fatalf("Load succeeded, want error containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not contain %q", err, tc.wantErr)
			}
		})
	}
}

func TestLoadReadsSABnzbdInstanceAsDownloadClient(t *testing.T) {
	path := writeConfig(t, `
[[instance]]
name = "sab"
app = "sabnzbd"
url = "http://sab.lan:8080"
api_key = "abc123"
`)

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	inst := cfg.Instances[0]
	if inst.App != config.Sabnzbd || inst.App.Kind() != config.DownloadClient {
		t.Errorf("App %q of kind %v, want sabnzbd as a Download Client", inst.App, inst.App.Kind())
	}
	if got := inst.App.DisplayName(); got != "SABnzbd" {
		t.Errorf("DisplayName %q, want SABnzbd", got)
	}
}

func TestResolveAPIKeyUsesLiteralKey(t *testing.T) {
	inst := config.Instance{Name: "sonarr", APIKey: "abc123"}
	key, err := inst.ResolveAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	if key != "abc123" {
		t.Errorf("key = %q, want %q", key, "abc123")
	}
}

func TestResolveAPIKeyRunsCommandAndTrimsOutput(t *testing.T) {
	inst := config.Instance{Name: "sonarr", APIKeyCmd: `printf '  s3cret\n\n'`}
	key, err := inst.ResolveAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	if key != "s3cret" {
		t.Errorf("key = %q, want %q", key, "s3cret")
	}
}

func TestResolveAPIKeyReportsFailingCommand(t *testing.T) {
	inst := config.Instance{Name: "sonarr", APIKeyCmd: "echo nope >&2; exit 3"}
	_, err := inst.ResolveAPIKey()
	if err == nil {
		t.Fatal("want error from failing api_key_cmd")
	}
	if !strings.Contains(err.Error(), "sonarr") || !strings.Contains(err.Error(), "nope") {
		t.Errorf("error %q should name the Instance and include stderr", err)
	}
}

func TestResolveAPIKeyRejectsEmptyCommandOutput(t *testing.T) {
	inst := config.Instance{Name: "sonarr", APIKeyCmd: "true"}
	if _, err := inst.ResolveAPIKey(); err == nil {
		t.Fatal("want error when api_key_cmd prints nothing")
	}
}

func TestPathPrefersEnvironmentVariable(t *testing.T) {
	t.Setenv("ARR_CLI_CONFIG", "/somewhere/arr.toml")
	if got := config.Path(); got != "/somewhere/arr.toml" {
		t.Errorf("Path() = %q", got)
	}
}

func TestPathDefaultsToUserConfigDir(t *testing.T) {
	t.Setenv("ARR_CLI_CONFIG", "")
	t.Setenv("HOME", "/home/tester")
	t.Setenv("XDG_CONFIG_HOME", "")
	if got, want := config.Path(), "/home/tester/.config/arr-cli/config.toml"; got != want {
		t.Errorf("Path() = %q, want %q", got, want)
	}
}

func TestLoadRejectsInstanceNamedAfterTopLevelCommand(t *testing.T) {
	reserved := []string{"status", "queue", "calendar", "config"}
	for _, name := range reserved {
		t.Run(name, func(t *testing.T) {
			body := "[[instance]]\nname = \"" + name + "\"\napp = \"sonarr\"\nurl = \"http://x\"\napi_key = \"k\"\n"
			_, err := config.Load(writeConfig(t, body), reserved...)
			if err == nil {
				t.Fatalf("Load accepted an Instance named %q", name)
			}
			want := `instance "` + name + `" clashes with the top-level command "arr ` + name + `"`
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q does not contain %q", err, want)
			}
		})
	}
}
