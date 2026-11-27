# Branches and PRs

`main` takes changes only through PRs. Every change starts on its own branch, goes up as a PR, and is squash-merged.

## Branches

Name a branch after the kind of change plus a short kebab-case description:

- `feat/<feature>`: new or changed behavior, such as `feat/version-flag`.
- `fix/<fix>`: a bug fix, such as `fix/calendar-month-wrap`.
- `chore/<chore>`: docs, CI, dependencies, or refactors that change no behavior, such as `chore/branch-workflow`.

Nothing else. Leave issue numbers out of the name; link the issue with `Closes #n` in the PR body.

```sh
git switch main && git pull --ff-only
git switch -c feat/<feature>
```

## PRs

- **Title**: an imperative sentence in the style of the existing history, such as "Add arr --version and SemVer Releases", without a `feat:` or `fix:` prefix. It becomes the commit subject on `main`.
- **Body**: written with the `/pr` skill.
- **CI**: `.github/workflows/ci.yml` runs `go vet` and `go test ./...` on the PR. A command that calls an App still needs its `TestIntegration…` case, run with `make integration` (see `docs/integration-tests.md`).
- **Review**: no approval is required. An agent opens PRs but merges one only when the user explicitly asks.

## Merging

Squash is the only merge method. The commit subject is the PR title, its body is the branch's commit messages, and GitHub deletes the branch on merge. Afterwards:

```sh
git switch main && git pull --ff-only
```

## Releases

Tag only commits on `main`. The Release workflow fails a tag that is not on `main`, or that is not SemVer (ADR 0004), and runs the integration tests before publishing.

## How main is kept locked

The repository is private on GitHub Free, which cannot protect a branch, so the lock is local:

- `.githooks/pre-commit` refuses a commit on `main`.
- `.githooks/pre-push` refuses a push to `main`, and a push of a branch whose name breaks the pattern above.
- `make hooks` turns those on for a clone (`git config core.hooksPath .githooks`). Run it once after cloning.
- `.claude/settings.json` runs `.claude/hooks/guard-git.sh` before each Bash call by Claude Code. It blocks skipping the git hooks and any `git push` that names `main`.

The guard reads a Bash command as plain text, so a PR body or commit message given inline that quotes a blocked command (such as a push to main) trips it. Write such text to a file and pass it with `--body-file` or `git commit -F`.

Never skip the git hooks. If one refuses something that should go through, fix the hook in a `chore/` PR.
