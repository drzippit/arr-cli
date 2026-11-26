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
	"unicode"

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

// DisplayName is how the App spells its own name, as in "Sonarr".
func (a App) DisplayName() string {
	if a == "" {
		return ""
	}
	return strings.ToUpper(string(a[:1])) + string(a[1:])
}

// Valid reports whether a is an App arr-cli can talk to.
func (a App) Valid() bool { return slices.Contains(Apps, a) }

// Config is the parsed config file.
type Config struct {
	Instances []Instance `toml:"instance"`
}

// Instance is one running deployment of an App. QualityProfile (a profile
// name), RootFolder (a path), for Radarr MinimumAvailability, and for Sonarr
// Monitor (which episodes of a Series to monitor) are the defaults for adding
// Titles to it.
type Instance struct {
	Name           string `toml:"name"`
	App            App    `toml:"app"`
	URL            string `toml:"url"`
	APIKey         string `toml:"api_key,omitempty"`
	APIKeyCmd      string `toml:"api_key_cmd,omitempty"`
	QualityProfile string `toml:"quality_profile,omitempty"`
	RootFolder     string `toml:"root_folder,omitempty"`

	MinimumAvailability string `toml:"minimum_availability,omitempty"`
	Monitor             string `toml:"monitor,omitempty"`
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

// Load reads and validates the config file at path. An Instance named after
// one of reservedNames (the top-level commands) is rejected, because Instance
// names share the command namespace (ADR 0002).
func Load(path string, reservedNames ...string) (*Config, error) {
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
	if err := cfg.validate(reservedNames); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &cfg, nil
}

// CheckName says why name cannot be given to a new Instance: it is blank,
// has spaces, or an Instance or a top-level command (one of reservedNames)
// already has it.
func (c *Config) CheckName(name string, reservedNames ...string) error {
	switch {
	case strings.TrimSpace(name) == "":
		return errors.New("missing name")
	case strings.ContainsFunc(name, unicode.IsSpace):
		return fmt.Errorf("instance name %q has spaces; it is typed as a command, as in arr <name> status", name)
	case slices.ContainsFunc(c.Instances, func(i Instance) bool { return i.Name == name }):
		return fmt.Errorf("duplicate instance name %q", name)
	case slices.Contains(reservedNames, name):
		return clashError(name)
	}
	return nil
}

// CanAdd says why inst cannot be added to the config, if it cannot.
func (c *Config) CanAdd(inst Instance, reservedNames ...string) error {
	if err := c.CheckName(inst.Name, reservedNames...); err != nil {
		return err
	}
	next := Config{Instances: append(slices.Clone(c.Instances), inst)}
	return next.validate(reservedNames)
}

func clashError(name string) error {
	return fmt.Errorf("instance %q clashes with the top-level command \"arr %s\"; choose another name", name, name)
}

func (c *Config) validate(reservedNames []string) error {
	seen := map[string]bool{}
	for i, inst := range c.Instances {
		if inst.Name == "" {
			return fmt.Errorf("instance #%d: missing name", i+1)
		}
		if seen[inst.Name] {
			return fmt.Errorf("duplicate instance name %q", inst.Name)
		}
		seen[inst.Name] = true
		if slices.Contains(reservedNames, inst.Name) {
			return clashError(inst.Name)
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

// APIKeySource names the config key that gives the Instance's API key:
// api_key or api_key_cmd.
func (i Instance) APIKeySource() string {
	if i.APIKeyCmd != "" {
		return "api_key_cmd"
	}
	return "api_key"
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
