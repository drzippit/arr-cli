# Integration tests

The integration tests run arr against real Sonarr, Radarr and SABnzbd Instances in Docker. The ordinary tests replay recorded responses, so they cannot notice when an App's API changes; these can. The Release workflow runs them before publishing, and a failure stops the Release. Run them yourself when a change touches how arr talks to an App.

## Running them

```
make integration
```

This starts Sonarr, Radarr and SABnzbd from `integration/compose.yaml`, waits until all three answer, runs every test named `TestIntegration…` (in files tagged `//go:build integration`), then stops and removes the containers, whether the tests passed or not. A first run downloads the images. `go test ./...` without `-tags integration` never compiles these tests.

It needs Docker with the Compose plugin, and a user that can run `docker` without sudo (on most Linux systems, a member of the `docker` group).

Pass extra `go test` flags through `TESTFLAGS`, as in `make integration TESTFLAGS=-v` or `TESTFLAGS='-run TestIntegrationStatusOfRadarr'`; a second `-run` replaces the default one. The ports the Instances listen on, on 127.0.0.1, are set in the `Makefile`; override `ARR_IT_SONARR_PORT`, `ARR_IT_RADARR_PORT` or `ARR_IT_SABNZBD_PORT` there or on the command line if they are taken.

Ctrl-C stops the tests and still removes the containers, and each run first removes any containers an earlier run left behind. The containers have a fixed Compose project name, so run one `make integration` at a time on a machine.

## The Instances

- Each image is pinned to an exact release in `integration/compose.yaml`. To test against a newer release, change the tag, run `make integration`, and commit both together. Tests check the major and minor version (" 4.0.", " 7.0.", "5.1."), so a patch release needs no test change, but a new minor or major release means updating those checks too.
- The API keys are fixed in the `Makefile` and handed to the Apps through their environment, so the tests know them without reading the Apps' config files. SABnzbd reads its key only from its config file, so its service writes a new `sabnzbd.ini` holding the key before SABnzbd starts.
- Every run starts from new Instances: Sonarr and Radarr with an empty Library, SABnzbd with an empty Queue. All tests in one run share the same Instances, so a test that changes one (adding a Title, say) must not depend on running before or after another. Keep the integration tests in `internal/cli`: tests in another package would run at the same time as these, against the same Instances.
- A new Instance has the App's default quality profiles but no root folder. The Sonarr container has an empty `/tv` folder and the Radarr one an empty `/movies`; a test that adds a Title first makes that folder a root folder by POSTing `{"path": "/tv"}` (or `"/movies"`) to the Instance's `/api/v3/rootfolder`. The App refuses a folder that does not exist in its container.

## Adding a case

Add a test to `internal/cli/integration_test.go` named `TestIntegration<Command>…`. `realInstanceFor(t, "sonarr")`, `realInstanceFor(t, "radarr")` or `realInstanceFor(t, "sabnzbd")` gives that Instance, named after its App, for `useConfig`; then run arr with `run(t, …)` as the other CLI tests do:

```go
func TestIntegrationQueueOfSonarr(t *testing.T) {
	useConfig(t, realInstanceFor(t, "sonarr"))

	out, stderr, code := run(t, "sonarr", "queue")

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s\n%s", code, stderr, out)
	}
}
```

Check what a real Instance must give, not what this particular run happens to hold: a new Instance has nothing downloading, so a queue test can only expect an empty queue unless it adds something first. Another test may already have added a Title, so a read-only test checks the shape of the output, not that it is empty.

Helpers in `integration_test.go`:

- `records(t, args...)` runs arr with `--json` among `args` and returns its records, failing unless the output is a JSON list whose records each name their Instance and App.
- `useBothInstances(t)` puts both real Instances in the config, for commands that run across every Instance.
- `ensureRootFolder(t, inst, "/movies")` makes a folder a root folder of the Instance unless it already is; `add` needs one.

A test that changes an Instance keeps every step that depends on that change in one test, in order: `TestIntegrationAddThenListMovie` adds The Matrix, lists it, and checks a second add is refused. The `search` and `add` tests go through the App to its metadata service, so they need the internet and fail while that service is down; re-run the Release once it is back.
