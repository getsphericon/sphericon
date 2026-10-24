#!/usr/bin/env bash
# Keeps one open issue that lists the mise tools whose newest release is not the one mise.lock
# pins (Dependabot has no mise ecosystem), and closes it once the lock is current. Run weekly by
# .github/workflows/tools.yml with GH_TOKEN set; needs mise, jq and gh on the PATH.
set -euo pipefail

title="Update the mise tools"
label="ready-for-agent"

outdated="$(mise outdated --local --json)"
count="$(jq 'length' <<<"$outdated")"
number="$(gh issue list --state open --label "$label" --search "\"$title\" in:title" \
  --json number,title --jq "map(select(.title == \"$title\")) | .[0].number // empty")"

if [[ "$count" -eq 0 ]]; then
  if [[ -n "$number" ]]; then
    gh issue close "$number" --comment "mise.lock pins the newest release of every tool."
  fi
  exit 0
fi

rows="$(jq -r 'to_entries | sort_by(.key)[] | .value
  | (if .release_url then "[\(.latest)](\(.release_url))" else .latest end) as $newest
  | "| \(.name) | \(.current) | \($newest) |"' <<<"$outdated")"

body="$(
  cat <<EOF
These tools are declared as \`latest\` in \`.mise.toml\`, so \`mise.lock\` is the only thing that
holds them back, and nothing moves it but a commit.

| Tool | Locked | Newest |
|---|---|---|
$rows

Bump each with \`mise upgrade <tool>\` (it rewrites \`mise.lock\`), read the release notes of a tool
that fails \`mise run check\`, and land it with a \`build(deps):\` commit. The newest release may be
withheld by mise's release-age cutoff; if \`mise upgrade\` skips a tool, leave it for the next run.
EOF
)"

if [[ -n "$number" ]]; then
  gh issue edit "$number" --body "$body"
else
  gh issue create --title "$title" --label "$label" --body "$body"
fi
