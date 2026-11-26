// Package config loads the arr-cli config file, which names the Instances
// arr-cli can reach.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
)

// EnvVar names the environment variable that overrides the config path.
const EnvVar = "ARR_CLI_CONFIG"

// App is one of the *arr applications arr-cli talks to.
type App string

// The Apps arr-cli can talk to.
const (
	Sonarr App = "sonarr"
	Radarr App = "radarr"
)

// Apps lists every App arr-cli can talk to.
var Apps = []App{Sonarr, Radarr}

// Valid reports whether a is an App arr-cli can talk to.
func (a App) Valid() bool { return slices.Contains(Apps, a) }

// ReservedNames are top-level command names. An Instance may not use one,
// because Instance names share the command namespace (ADR 0002). The list
// includes planned commands (queue, calendar, config) so a config that works
// today keeps working when they land.
var ReservedNames = []string{"status", "queue", "calendar", "config", "help", "completion"}

// Config is the parsed config file.
type Config struct {
	Instances []Instance `toml:"instance"`
}

// Instance is one running deployment of an App.
type Instance struct {
	Name      string `toml:"name"`
	App       App    `toml:"app"`
	URL       string `toml:"url"`
	APIKey    string `toml:"api_key"`
	APIKeyCmd string `toml:"api_key_cmd"`
}

// Path returns the config file path: $ARR_CLI_CONFIG if set, otherwise
// ~/.config/arr-cli/config.toml.
func Path() string {
	if p := os.Getenv(EnvVar); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = "~"
	}
	return filepath.Join(home, ".config", "arr-cli", "config.toml")
}

// Load reads and validates the config file at path.
func Load(path string) (*Config, error) {
	var cfg Config
	md, err := toml.DecodeFile(path, &cfg)
	if err != nil {
		var perr toml.ParseError
		if errors.As(err, &perr) {
			return nil, fmt.Errorf("parse %s: %s", path, perr.ErrorWithPosition())
		}
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		return nil, fmt.Errorf("%s: unknown key %q", path, undecoded[0].String())
	}
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &cfg, nil
}

func (c *Config) validate() error {
	seen := map[string]bool{}
	for i, inst := range c.Instances {
		if inst.Name == "" {
			return fmt.Errorf("instance #%d: missing name", i+1)
		}
		if seen[inst.Name] {
			return fmt.Errorf("duplicate instance name %q", inst.Name)
		}
		seen[inst.Name] = true
		if slices.Contains(ReservedNames, inst.Name) {
			return fmt.Errorf("instance %q clashes with the top-level command \"arr %s\"; choose another name", inst.Name, inst.Name)
		}
		if !inst.App.Valid() {
			return fmt.Errorf("instance %q: app %q is not one of %v", inst.Name, inst.App, Apps)
		}
		if inst.URL == "" {
			return fmt.Errorf("instance %q: missing url", inst.Name)
		}
		switch {
		case inst.APIKey == "" && inst.APIKeyCmd == "":
			return fmt.Errorf("instance %q: set api_key or api_key_cmd", inst.Name)
		case inst.APIKey != "" && inst.APIKeyCmd != "":
			return fmt.Errorf("instance %q: set only one of api_key and api_key_cmd", inst.Name)
		}
	}
	return nil
}

// ResolveAPIKey returns the Instance's API key, running api_key_cmd through
// the shell and trimming its stdout when the key is not given literally.
func (i Instance) ResolveAPIKey() (string, error) {
	if i.APIKeyCmd == "" {
		return i.APIKey, nil
	}
	var stdout, stderr bytes.Buffer
	cmd := exec.Command("sh", "-c", i.APIKeyCmd)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("instance %q: api_key_cmd failed: %s", i.Name, msg)
	}
	key := strings.TrimSpace(stdout.String())
	if key == "" {
		return "", fmt.Errorf("instance %q: api_key_cmd printed nothing", i.Name)
	}
	return key, nil
}
