# arr-cli

[![Release](https://github.com/drzippit/arr-cli/actions/workflows/release.yml/badge.svg)](https://github.com/drzippit/arr-cli/actions/workflows/release.yml)
[![Latest release](https://img.shields.io/github/v/release/drzippit/arr-cli?sort=semver&label=release)](https://github.com/drzippit/arr-cli/releases/latest)
[![Go](https://img.shields.io/badge/go-1.27-00ADD8?logo=go&logoColor=white)](go.mod)
[![Sonarr](https://img.shields.io/badge/Sonarr-4%2B-35c5f4)](docs/adr/0003-current-major-app-versions-only.md)
[![Radarr](https://img.shields.io/badge/Radarr-5%2B-ffc230)](docs/adr/0003-current-major-app-versions-only.md)
[![SABnzbd](https://img.shields.io/badge/SABnzbd-4%2B-f5c518)](docs/adr/0003-current-major-app-versions-only.md)
[![Platforms](https://img.shields.io/badge/platforms-linux%20%7C%20macOS%20%7C%20windows-lightgrey)](#install)

**Run your self-hosted \*arr media stack from the terminal.**

`arr` talks to every Sonarr, Radarr and SABnzbd you run at once. Check that they're all healthy, see what's downloading and what's due this month, search your Libraries, and add new Series and Movies, without opening a single web UI. Every command prints a readable table in a terminal and clean JSON for scripts.

```
$ arr status
╭──────────┬─────────┬───────────┬─────────────┬─────────────┬───────────┬───────────┬──────────╮
│ INSTANCE │ APP     │ REACHABLE │ VERSION     │ STATE       │ SPEED     │ FREE      │ WARNINGS │
├──────────┼─────────┼───────────┼─────────────┼─────────────┼───────────┼───────────┼──────────┤
│ sonarr   │ sonarr  │ yes       │ 4.0.20.3014 │ -           │ -         │ -         │ 0        │
│ radarr   │ radarr  │ yes       │ 7.0.3.10918 │ -           │ -         │ -         │ 1        │
│ sab      │ sabnzbd │ yes       │ 5.1.3       │ downloading │ 2.0 MiB/s │ 812.5 GiB │ 0        │
╰──────────┴─────────┴───────────┴─────────────┴─────────────┴───────────┴───────────┴──────────╯
```

## Features

- **Every Instance at once.** `arr status`, `arr queue`, `arr calendar` and `arr list` ask all your Instances in parallel and show the answers together, including a second Sonarr for 4K or anime.
- **One Instance by name.** `arr sonarr-4k queue` works on just that one, so a command that changes something always names what it changes.
- **Add Series and Movies.** Search, pick a Candidate, and add it with your usual quality profile and root folder, interactively or fully from a script.
- **A month at a glance.** `arr calendar` draws the episodes and Movie releases due this month as a grid in your terminal.
- **Made for scripts too.** `--json` on every command, `--plain` lists, no prompts when there's no terminal, and a non-zero exit code when an Instance can't be reached.
- **Shell completion** for fish, zsh and bash, including the names of your Instances.

## Install

### From a release

Each [release](https://github.com/drzippit/arr-cli/releases) has an archive per platform (Linux and macOS on amd64/arm64, Windows on amd64) and a `SHA256SUMS`. Pick your platform; `v` becomes the latest release (or set it yourself, as in `v=v0.3.0`):

```sh
platform=linux_amd64
v=$(curl -fsSLI -o /dev/null -w '%{url_effective}' https://github.com/drzippit/arr-cli/releases/latest); v=${v##*/}
curl -fsSLO "https://github.com/drzippit/arr-cli/releases/download/$v/arr_${v}_${platform}.tar.gz"
curl -fsSLO "https://github.com/drzippit/arr-cli/releases/download/$v/SHA256SUMS"
sha256sum --check --ignore-missing SHA256SUMS   # on macOS: shasum -a 256 --check --ignore-missing SHA256SUMS
tar -xzf "arr_${v}_${platform}.tar.gz" arr
sudo install arr /usr/local/bin/
arr --version
```

### With Go

With Go 1.27 or later, this installs the latest Release, which `arr --version` then reports:

```sh
go install github.com/drzippit/arr-cli/cmd/arr@latest
```

### From source

```sh
git clone https://github.com/drzippit/arr-cli.git
cd arr-cli
go build -o arr ./cmd/arr
```

## Quick start

1. **Add an Instance.** `arr config add` asks for its name, App, URL and API key (Settings › General in Sonarr or Radarr, Config › General in SABnzbd), checks that it can reach it, and saves it:

   ```sh
   arr config add
   ```

   Repeat for each Sonarr, Radarr and SABnzbd you run.

2. **Check on everything:**

   ```sh
   arr status
   ```

3. **Explore:** `arr queue`, `arr calendar`, `arr list`, and `arr <instance> --help` for what one Instance can do.

## Configuration

The config lives at `~/.config/arr-cli/config.toml`; set `ARR_CLI_CONFIG` to use another file. `arr config add` writes it for you, keeping anything already there, and `arr config list` shows what's configured with the API keys hidden. You can also edit it by hand:

```toml
[[instance]]
name = "sonarr"
app = "sonarr"
url = "http://sonarr.lan:8989"
api_key = "0123456789abcdef0123456789abcdef"
quality_profile = "HD-1080p"     # defaults for arr sonarr add
root_folder = "/tv"
monitor = "all"

[[instance]]
name = "radarr-4k"
app = "radarr"
url = "http://radarr.lan:7878"
api_key_cmd = "pass show radarr-4k"   # a command that prints the key, instead of api_key
quality_profile = "Ultra-HD"     # defaults for arr radarr-4k add
root_folder = "/movies-4k"
minimum_availability = "released"

[[instance]]
name = "sab"
app = "sabnzbd"
url = "http://sab.lan:8080"
api_key_cmd = "pass show sabnzbd"
```

| Key | Required | Meaning |
| --- | --- | --- |
| `name` | yes | What you call the Instance in commands, as in `arr radarr-4k status`. It can't be the name of a top-level command such as `status` or `config`. |
| `app` | yes | `sonarr`, `radarr` or `sabnzbd`. |
| `url` | yes | Where the Instance answers, including any URL base. |
| `api_key` / `api_key_cmd` | one of them | The API key, or a shell command that prints it (for a password manager). |
| `quality_profile` | no | Quality profile name `add` uses when you don't pass `--quality`. This key and the three below are for adding Titles, so a `sabnzbd` Instance can't have them. |
| `root_folder` | no | Root folder `add` uses when you don't pass `--root`. |
| `monitor` | no | Sonarr only: which episodes `add` monitors when you don't pass `--monitor`. |
| `minimum_availability` | no | Radarr only: `announced`, `inCinemas` or `released` (the default). |

## Usage

Commands take the form `arr <instance> <command>`. Leave out the Instance and the read commands run across every Instance:

```sh
arr status              # every Instance
arr radarr-4k status    # just radarr-4k
```

### Commands

| Command | What it does |
| --- | --- |
| `arr [<instance>] status` | Whether each Instance is reachable, its version, and how many Warnings it reports. For SABnzbd also whether it is downloading or paused, its speed, and the free space in its download folders. `-v` lists each Warning. |
| `arr [<instance>] queue` | Downloads in progress, with progress and time left. `arr queue` shows each Download once: one that Sonarr or Radarr grabbed and SABnzbd is fetching shows the Title and episode, with SABnzbd's live progress, priority and category, and one you added to SABnzbd by hand shows under its own name. `arr <instance> queue` shows just what that Instance reports, and for SABnzbd that includes Downloads it is still verifying, repairing or unpacking. `-w` / `--watch` keeps it on screen and refreshes it every 2s (`--interval 5s` for another pace) until Ctrl-C, with SABnzbd's speed and time left underneath. |
| `arr [<instance>] calendar` | Episodes and Movie releases due this month, as a grid in a wide enough terminal. `--month 2026-12`, `--month next` or `prev` picks another month; `--list` gives a list by date; `--unmonitored` includes unmonitored ones. |
| `arr [<instance>] list [query]` | The Titles in each Library, optionally only those matching `query`. `--plain` prints one Title per line. |
| `arr [<instance>] history` | Downloads SABnzbd has finished, newest first, with why a failed one failed. `--failed` shows only failed ones; `--limit 50` shows more than the 20 it shows by default. |
| `arr <instance> search <query>` | Series or Movies that could be added to that Instance, marking those already in its Library. |
| `arr <sonarr> add [query]` | Add a Series. See [Adding Titles](#adding-titles). |
| `arr <radarr> add [query]` | Add a Movie. See [Adding Titles](#adding-titles). |
| `arr config add` | Add an Instance to the config, after checking `arr` can reach it. |
| `arr config list` | The Instances in the config, with API keys hidden. |
| `arr completion <shell>` | A completion script for fish, zsh or bash. |

`calendar`, `list`, `search` and `add` are about Titles, so they leave SABnzbd out, and a SABnzbd Instance has only `status`, `queue` and `history`. `history` covers SABnzbd only.

Every command takes `--json` to print JSON instead of a table, and `--help` for its flags.

### Adding Titles

```sh
arr sonarr add "severance"          # pick from the Candidates, then the quality profile, root folder and monitoring
arr sonarr add --tvdb 371980 --monitor future --quality HD-1080p --root /tv
arr radarr add "project hail mary"
arr radarr add --tmdb 603 --no-search    # The Matrix, without searching yet
```

`add` looks your query up, lets you pick a Candidate, and adds it monitored, then starts a search for it (`--no-search` skips the search). The quality profile and root folder come from `--quality` / `--root`, then the Instance's config, then a picker. For a Series, `--monitor` takes Sonarr's options: `all`, `future`, `missing`, `existing`, `firstSeason`, `latestSeason` or `none`. For a Movie, `--availability` sets when it counts as available.

In a terminal, `arr` asks for whatever it's missing. From a script it never asks: it adds the only Candidate that matches, or fails and lists them so you can pass `--tvdb` / `--tmdb`, and it fails naming the flag and config key for any other setting it lacks.

### In scripts

- `--json` prints the same records as the table, for `jq` and friends.
- `arr list --plain` prints one Title per line.
- Pickers and prompts only appear when both stdin and stdout are a terminal.
- A command across several Instances still shows what the reachable ones said, reports each one that failed on stderr, and exits non-zero. Warnings an Instance reports about itself don't change the exit code.

```sh
arr status --json | jq -r '.[] | select(.warnings > 0) | .instance'
```

### Shell completion

```sh
arr completion fish > ~/.config/fish/completions/arr.fish
arr completion zsh > "${fpath[1]}/_arr"
arr completion bash > ~/.local/share/bash-completion/completions/arr
```

Completion offers the top-level commands and your Instance names (`arr ra<TAB>`), then each Instance's commands (`arr radarr <TAB>`).

## Supported App versions

`arr` supports **Sonarr 4 and later** and **Radarr 5 and later**, the current major versions, and **SABnzbd 4 and later**: `arr` only reads from SABnzbd, and versions 4 and 5 answer it alike. `arr status` marks an older Instance as unsupported, and commands that change an Instance, such as `add`, refuse to run against one. See [ADR 0003](docs/adr/0003-current-major-app-versions-only.md).

## Versioning

`arr --version` prints the Release you are running (`arr v0.2.0`), or `dev` and the commit for a build that is not one (`arr dev (52b07aa-dirty)`). Add `--json` for `{"version", "commit", "dirty"}`. It works even when the config does not load.

Releases follow [SemVer](https://semver.org) over what scripts use: commands and flags, `--json` output, the config file and exit codes. Removing or renaming a JSON field is a breaking change; adding one is not. Tables and other output meant for people can change in any Release. While `arr` is on `0.x`, a minor Release may break things. See [ADR 0004](docs/adr/0004-semver-over-the-scripting-surface.md).

## Development

```sh
go test ./...          # the unit tests, against recorded API responses
make integration       # against real Sonarr, Radarr and SABnzbd in Docker
make dist VERSION=dev  # the release archives, into dist/
```

- `main` takes changes only through PRs from `feat/`, `fix/` or `chore/` branches. Run `make hooks` once after cloning to turn on the git hooks that enforce it; see [docs/agents/branching.md](docs/agents/branching.md).
- `make integration` needs Docker with Compose; see [docs/integration-tests.md](docs/integration-tests.md).
- Pushing a `v*` tag on `main` (for example `v0.2.0`) builds and publishes a release through [the Release workflow](.github/workflows/release.yml). A tag with a hyphen (`v0.2.0-rc1`) becomes a pre-release, and a tag that is not SemVer fails the workflow.
- `arr` is released under the [MIT License](LICENSE).
- The domain language (App, Instance, Title, Library, Candidate, Warning…) is defined in [GLOSSARY.md](GLOSSARY.md), and design decisions are recorded in [docs/adr](docs/adr).
