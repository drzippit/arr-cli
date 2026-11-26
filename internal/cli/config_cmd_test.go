package cli_test

import (
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/drzippit/arr-cli/internal/cli"
	"github.com/drzippit/arr-cli/internal/config"
)

// fillForm makes the CLI act as if it runs in a terminal where the person
// fills the new-Instance form with inst. It returns the name check the form
// was given, to try other names with.
func fillForm(t *testing.T, inst config.Instance) *func(string) error {
	t.Helper()
	var check func(string) error
	cli.SetInstanceForm(t, func(checkName func(string) error) (config.Instance, error) {
		check = checkName
		return inst, nil
	})
	return &check
}

// useMissingConfig points arr at a config file that does not exist yet, in
// a folder that does not either, and returns its path.
func useMissingConfig(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "arr-cli", "config.toml")
	t.Setenv(config.EnvVar, path)
	return path
}

func TestConfigAddSavesReachableInstanceToNewConfig(t *testing.T) {
	srv := fakeInstance(t, "sonarr")
	path := useMissingConfig(t)
	fillForm(t, config.Instance{Name: "sonarr", App: config.Sonarr, URL: srv.URL, APIKey: testAPIKey})

	out, stderr, code := run(t, "config", "add")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("config after add: %v", err)
	}
	want := config.Instance{Name: "sonarr", App: config.Sonarr, URL: srv.URL, APIKey: testAPIKey}
	if len(cfg.Instances) != 1 || cfg.Instances[0] != want {
		t.Errorf("Instances %+v, want %+v", cfg.Instances, want)
	}
	if want := "Added sonarr (Sonarr 4.0.11.2680) to " + path; !strings.Contains(out, want) {
		t.Errorf("output missing %q:\n%s", want, out)
	}
}

func TestConfigAddWithWrongAPIKeySavesNothing(t *testing.T) {
	srv := fakeInstance(t, "radarr")
	path := useMissingConfig(t)
	fillForm(t, config.Instance{Name: "radarr", App: config.Radarr, URL: srv.URL, APIKey: "wrong"})

	_, stderr, code := run(t, "config", "add")

	if code == 0 {
		t.Fatal("config add should fail when the Instance rejects the key")
	}
	for _, want := range []string{"cannot reach radarr at " + srv.URL, "401", "check the API key", "nothing saved"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr missing %q:\n%s", want, stderr)
		}
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("config file should not exist, got err %v", err)
	}
}

func TestConfigAddToUnreachableURLKeepsConfigAsItWas(t *testing.T) {
	srv := fakeInstance(t, "sonarr")
	existing := fmt.Sprintf("# mine\n[[instance]]\nname = \"sonarr\"\napp = \"sonarr\"\nurl = %q\napi_key = %q\n", srv.URL, testAPIKey)
	useConfigTOML(t, existing)
	fillForm(t, config.Instance{Name: "radarr", App: config.Radarr, URL: "http://127.0.0.1:1", APIKey: testAPIKey})

	_, stderr, code := run(t, "config", "add")

	if code == 0 || !strings.Contains(stderr, "nothing saved") {
		t.Errorf("want failure saying nothing was saved, got exit %d, stderr %q", code, stderr)
	}
	body, _ := os.ReadFile(os.Getenv(config.EnvVar))
	if string(body) != existing {
		t.Errorf("config changed:\n%s\nwant:\n%s", body, existing)
	}
}

func TestConfigAddRejectsNamesOfCommandsAndExistingInstances(t *testing.T) {
	srv := fakeInstance(t, "sonarr")
	existing := fmt.Sprintf("[[instance]]\nname = \"sonarr\"\napp = \"sonarr\"\nurl = %q\napi_key = %q\n", srv.URL, testAPIKey)
	useConfigTOML(t, existing)
	check := fillForm(t, config.Instance{Name: "sonarr", App: config.Sonarr, URL: srv.URL, APIKey: testAPIKey})

	_, stderr, code := run(t, "config", "add")

	if code == 0 || !strings.Contains(stderr, `duplicate instance name "sonarr"`) {
		t.Errorf("want failure naming the duplicate, got exit %d, stderr %q", code, stderr)
	}
	body, _ := os.ReadFile(os.Getenv(config.EnvVar))
	if string(body) != existing {
		t.Errorf("config changed:\n%s", body)
	}
	for _, name := range []string{"sonarr", "status", "config", "search", ""} {
		if (*check)(name) == nil {
			t.Errorf("the form's name check accepted %q", name)
		}
	}
	if err := (*check)("sonarr-4k"); err != nil {
		t.Errorf("the form's name check rejected sonarr-4k: %v", err)
	}
}

func TestConfigAddAppendsToExistingConfig(t *testing.T) {
	sonarr := fakeInstance(t, "sonarr")
	radarr := fakeInstance(t, "radarr")
	existing := fmt.Sprintf("# mine\n[[instance]]\nname = \"sonarr\"\napp = \"sonarr\"\nurl = %q\napi_key = %q\n", sonarr.URL, testAPIKey)
	useConfigTOML(t, existing)
	fillForm(t, config.Instance{Name: "radarr", App: config.Radarr, URL: radarr.URL, APIKeyCmd: "echo " + testAPIKey})

	_, stderr, code := run(t, "config", "add")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	body, _ := os.ReadFile(os.Getenv(config.EnvVar))
	if !strings.HasPrefix(string(body), existing) {
		t.Errorf("config should start with what was there:\n%s", body)
	}
	cfg, err := config.Load(os.Getenv(config.EnvVar))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Instances) != 2 || cfg.Instances[1].Name != "radarr" || cfg.Instances[1].APIKeyCmd != "echo "+testAPIKey || cfg.Instances[1].APIKey != "" {
		t.Errorf("Instances %+v, want sonarr then radarr with its api_key_cmd", cfg.Instances)
	}
}

func TestConfigAddWithoutTerminalFailsPointingAtConfigFile(t *testing.T) {
	path := useMissingConfig(t)

	_, stderr, code := run(t, "config", "add")

	if code == 0 || !strings.Contains(stderr, "terminal") || !strings.Contains(stderr, path) {
		t.Errorf("want failure saying it needs a terminal and naming %s, got exit %d, stderr %q", path, code, stderr)
	}
}

func TestConfigListShowsInstancesWithKeysHidden(t *testing.T) {
	useConfigTOML(t, `[[instance]]
name = "sonarr"
app = "sonarr"
url = "http://sonarr.lan:8989"
api_key = "s3cr3t-sonarr-key"

[[instance]]
name = "radarr-4k"
app = "radarr"
url = "http://radarr.lan:7878"
api_key_cmd = "echo s3cr3t-radarr-key"
`)

	out, stderr, code := run(t, "config", "list")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	assertRow(t, out, "sonarr", "sonarr", "http://sonarr.lan:8989", "api_key (hidden)")
	assertRow(t, out, "radarr-4k", "radarr", "http://radarr.lan:7878", "api_key_cmd (hidden)")
	if strings.Contains(out, "s3cr3t") {
		t.Errorf("output shows an API key or the command giving one:\n%s", out)
	}
}

func TestConfigListJSON(t *testing.T) {
	useConfigTOML(t, `[[instance]]
name = "sonarr"
app = "sonarr"
url = "http://sonarr.lan:8989"
api_key = "s3cr3t-sonarr-key"
`)

	out, stderr, code := run(t, "config", "list", "--json")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	want := `[
  {
    "name": "sonarr",
    "app": "sonarr",
    "url": "http://sonarr.lan:8989",
    "apiKeyFrom": "api_key"
  }
]
`
	if out != want {
		t.Errorf("JSON output:\n%s\nwant:\n%s", out, want)
	}
}

func TestConfigListWithoutConfigPointsToConfigAdd(t *testing.T) {
	path := useMissingConfig(t)

	_, stderr, code := run(t, "config", "list")

	if code == 0 || !strings.Contains(stderr, "no config found at "+path) || !strings.Contains(stderr, "arr config add") {
		t.Errorf("want failure naming the missing config and arr config add, got exit %d, stderr %q", code, stderr)
	}
}

func TestConfigAddRefusesURLRunningAnotherApp(t *testing.T) {
	srv := fakeInstance(t, "radarr")
	path := useMissingConfig(t)
	fillForm(t, config.Instance{Name: "sonarr", App: config.Sonarr, URL: srv.URL, APIKey: testAPIKey})

	_, stderr, code := run(t, "config", "add")

	if code == 0 || !strings.Contains(stderr, srv.URL+" runs Radarr, not Sonarr") || !strings.Contains(stderr, "nothing saved") {
		t.Errorf("want failure naming the App found, got exit %d, stderr %q", code, stderr)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("config file should not exist, got err %v", err)
	}
}

func TestConfigAddRefusesUnsupportedVersion(t *testing.T) {
	srv := httptest.NewServer(runningVersion("Sonarr", "3.0.10.1567", recordedHandler(t, "sonarr")))
	t.Cleanup(srv.Close)
	path := useMissingConfig(t)
	fillForm(t, config.Instance{Name: "sonarr", App: config.Sonarr, URL: srv.URL, APIKey: testAPIKey})

	_, stderr, code := run(t, "config", "add")

	if code == 0 || !strings.Contains(stderr, "sonarr runs Sonarr 3.0.10.1567; arr-cli supports Sonarr 4 and later") || !strings.Contains(stderr, "nothing saved") {
		t.Errorf("want failure naming the unsupported version, got exit %d, stderr %q", code, stderr)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("config file should not exist, got err %v", err)
	}
}

func TestConfigAddNameCheckRejectsBlankAndSpacedNames(t *testing.T) {
	srv := fakeInstance(t, "sonarr")
	useConfigTOML(t, fmt.Sprintf("[[instance]]\nname = \"sonarr\"\napp = \"sonarr\"\nurl = %q\napi_key = %q\n", srv.URL, testAPIKey))
	check := fillForm(t, config.Instance{Name: "sonarr-4k", App: config.Sonarr, URL: srv.URL, APIKey: testAPIKey})

	if _, stderr, code := run(t, "config", "add"); code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}

	for _, name := range []string{"   ", "my sonarr"} {
		if (*check)(name) == nil {
			t.Errorf("the form's name check accepted %q", name)
		}
	}
}
