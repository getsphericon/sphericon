#!/usr/bin/env bash
# Lands the work of this worktree on main, the way AGENTS.md describes it: rebase onto a freshly
# fetched origin/main, the checks and the tests, `git push origin HEAD:main`, then the ticket check.
#
#   mise run land -- [--skip-test] [<issue>...]
#
# A worktree lands from a detached HEAD, never a branch, with a clean tree. --skip-test is for a
# change no Go or frontend test reads (a doc, a workflow). Each <issue> is the ticket the work was
# for: once the push is done it is closed with a comment unless GitHub closed it from the commit's
# `Closes #<n>` footer. The worktree itself is removed by the caller, which is standing in it.
set -euo pipefail

skip_test=
issues=()
for arg in "$@"; do
  case "$arg" in
    --skip-test) skip_test=1 ;;
    *) issues+=("$arg") ;;
  esac
done

branch="$(git branch --show-current)"
if [[ -n "$branch" ]]; then
  echo "land: HEAD is on the branch '$branch'; work lands from a detached HEAD, never a branch" >&2
  exit 1
fi
if [[ -n "$(git status --porcelain)" ]]; then
  echo "land: the working tree is not clean; commit or discard the changes first" >&2
  exit 1
fi

git fetch -q origin
git rebase origin/main
if [[ "$(git rev-parse HEAD)" == "$(git rev-parse origin/main)" ]]; then
  echo "land: nothing to land, HEAD is origin/main" >&2
  exit 1
fi

mise run check
if [[ -z "$skip_test" ]]; then
  mise run test
fi

git push origin HEAD:main
sha="$(git rev-parse --short HEAD)"

for n in ${issues[@]+"${issues[@]}"}; do
  state="$(gh issue view "$n" --json state --jq .state)"
  if [[ "$state" != "CLOSED" ]]; then
    gh issue close "$n" --comment "Implemented on main in $sha."
  fi
  echo "issue #$n: $(gh issue view "$n" --json state --jq .state)"
done

echo "landed $sha. Remove this worktree, then update the checkout you work from:"
echo "  git fetch origin && git rebase origin/main"
