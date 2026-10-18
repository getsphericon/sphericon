#!/usr/bin/env bash
# Runs a CPU-heavy command (go test, golangci-lint, tsc, vitest, code generation) while
# holding one of a few machine-wide slots, so many checkouts and agents running tasks at
# once queue up instead of all starting together and starving each other.
#
#   mise/scripts/heavy.sh <command> [args...]
#
# A slot is an flock(1) lock on a file in SPHERICON_LOCK_DIR; the kernel drops it when the
# holder ends, however it ends. SPHERICON_HEAVY_SLOTS is how many may run at once (default:
# a quarter of the cores, at least one). A command started inside a held slot (a task that
# calls another heavy task) runs at once, so nesting never deadlocks. CI has a machine of its
# own and skips the queue, and so does a machine without flock (`brew install flock` on
# macOS; util-linux ships it): the command runs at once, with a warning.
set -euo pipefail

if [[ -n "${SPHERICON_HEAVY_HELD:-}" || -n "${CI:-}" ]]; then
  exec "$@"
fi
if ! command -v flock >/dev/null; then
  echo "heavy: flock not found, running without the queue (macOS: brew install flock)" >&2
  exec "$@"
fi

cores="$(getconf _NPROCESSORS_ONLN 2>/dev/null || echo 4)"
slots="${SPHERICON_HEAVY_SLOTS:-$((cores / 4))}"
((slots >= 1)) || slots=1
dir="${SPHERICON_LOCK_DIR:-${TMPDIR:-/tmp}/sphericon-heavy}"
mkdir -p "$dir"

announced=
while true; do
  for ((i = 1; i <= slots; i++)); do
    exec 9>"$dir/slot-$i"
    if flock -n 9; then
      export SPHERICON_HEAVY_HELD=1
      status=0
      # The command does not inherit the lock's descriptor: a daemon it leaves behind
      # (the e2e Mailpit) must not keep the slot after the command ends.
      "$@" 9>&- || status=$?
      exit "$status"
    fi
    exec 9>&-
  done
  if [[ -z "$announced" ]]; then
    echo "heavy: all $slots slots are busy, waiting for one ($dir)" >&2
    announced=1
  fi
  sleep $((1 + RANDOM % 3))
done
