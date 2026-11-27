#!/bin/sh
# PreToolUse hook for Bash: blocks skipping the git hooks in .githooks
# (--no-verify, or -n on git commit) and any git push that names main, so
# main takes changes only through PRs (docs/agents/branching.md).
cmd=$(jq -r '.tool_input.command // ""')
block() {
	echo "$1 See docs/agents/branching.md." >&2
	exit 2
}
echo "$cmd" | grep -Eq 'git[^|;&]*\b(commit|push)\b[^|;&]*--no-verify' &&
	block "Blocked: --no-verify would skip the git hooks that keep main locked."
echo "$cmd" | grep -Eq 'git[^|;&]*\bcommit\b[^|;&"'\'']*[[:space:]]-[a-zA-Z]*n[a-zA-Z]*([[:space:]]|$)' &&
	block "Blocked: git commit -n would skip the git hooks that keep main locked."
echo "$cmd" | grep -Eq 'git[^|;&]*\bpush\b[^|;&]*([[:space:]:+])main([[:space:]]|$)' &&
	block "Blocked: main takes changes only through PRs; push a feat/, fix/ or chore/ branch and open one."
exit 0
