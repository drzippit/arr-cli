package cli

import (
	"cmp"
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/drzippit/arr-cli/internal/arrapi"
	"github.com/drzippit/arr-cli/internal/config"
)

// oldestSupported is the first major version of each App that arr-cli
// supports (ADR 0003).
var oldestSupported = map[config.App]int{
	config.Sonarr: 4,
	config.Radarr: 5,
}

// supportedVersion reports whether version of app is one arr-cli supports. A
// version it cannot read gets the benefit of the doubt.
func supportedVersion(app config.App, version string) bool {
	major, _, _ := strings.Cut(version, ".")
	n, err := strconv.Atoi(major)
	return err != nil || n >= oldestSupported[app]
}

// requireSupportedVersion fails when the Instance runs a version of its App
// that arr-cli does not support. Commands that change an Instance call it
// first.
func requireSupportedVersion(ctx context.Context, inst config.Instance, client *arrapi.Client) error {
	sys, err := client.SystemStatus(ctx)
	if err != nil {
		return err
	}
	if supportedVersion(inst.App, sys.Version) {
		return nil
	}
	name := cmp.Or(sys.AppName, string(inst.App))
	return fmt.Errorf("%s runs %s %s; arr-cli supports %s %d and later",
		inst.Name, name, sys.Version, name, oldestSupported[inst.App])
}
