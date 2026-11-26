# arr-cli

## Agent skills

### Issue tracker

Issues are tracked in GitHub Issues for drzippit/arr-cli, via the `gh` CLI. See `docs/agents/issue-tracker.md`.

### Triage labels

Uses the five default triage labels: `needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`. See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: one `GLOSSARY.md` and `docs/adr/` at the repo root. See `docs/agents/domain.md`.

### Integration tests

A command that calls an App gets a `TestIntegration…` case when it is added or changed; `make integration` runs them against real Instances in Docker. See `docs/integration-tests.md`.
