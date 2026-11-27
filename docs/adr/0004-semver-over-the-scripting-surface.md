# SemVer over the scripting surface

An arr version is tagged `vMAJOR.MINOR.PATCH`, with an optional `-prerelease` suffix, and follows SemVer. What SemVer protects is the surface that scripts use: the command grammar and flags, `--json` output (including `arr --version --json`), the config file format, and exit codes. Removing or renaming a JSON field breaks that surface, and adding one does not. Tables, grids and other human-readable output are not covered, so their layout can change in any Release. Those are for people, and keeping them frozen would block improvements that are the point of a terminal UI.

While arr-cli is on `0.x`, a minor bump may include breaking changes, as SemVer allows. Moving to `1.0.0` is a deliberate promise of stability, not something that happens on its own.

`arr --version` is a flag rather than an `arr version` command, so it reserves no name in the Instance namespace (see ADR 0002). It works even when the config cannot load, and it has no `-v` shorthand, because `-v` already means verbose on `arr status`.
