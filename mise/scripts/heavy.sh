#!/usr/bin/env bash
# Runs a CPU-heavy command (go test, golangci-lint, tsc, vitest, code generation) while
# holding one of a few machine-wide slots, so many checkouts and agents running tasks at
# once queue up instead of all starting together and starving each other.
#
#   mise/scripts/heavy.sh <command> [args...]
#
# A slot is a directory in SPHERICON_LOCK_DIR: mkdir is atomic and needs no tool the
# machine may lack (macOS has no flock(1) and ships bash 3.2). The directory holds the pid
# of its owner, so a slot left by a killed run is taken over once that pid is gone.
# SPHERICON_HEAVY_SLOTS is how many may run at once (default: a quarter of the cores, at
# least one). A command started inside a held slot (a task that calls another heavy task)
# runs at once, so nesting never deadlocks. CI has a machine of its own and skips the queue.
set -euo pipefail

if [[ -n "${SPHERICON_HEAVY_HELD:-}" || -n "${CI:-}" ]]; then
  exec "$@"
fi

cores="$(getconf _NPROCESSORS_ONLN 2>/dev/null || echo 4)"
slots="${SPHERICON_HEAVY_SLOTS:-$((cores / 4))}"
((slots >= 1)) || slots=1
dir="${SPHERICON_LOCK_DIR:-${TMPDIR:-/tmp}/sphericon-heavy}"
mkdir -p "$dir"

held=
release() { [[ -z "$held" ]] || rm -rf "$held"; }
trap release EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

# take claims slot $1 and reports whether it is ours now.
take() {
  local slot="$dir/slot-$1" owner
  if mkdir "$slot" 2>/dev/null; then
    echo "$$" >"$slot/pid"
    held="$slot"
    return 0
  fi
  owner="$(cat "$slot/pid" 2>/dev/null || true)"
  # A slot whose owner is gone (killed, or not yet written) is stale; remove it and
  # let the next pass claim it, so two waiters never both think they took it over.
  if [[ -n "$owner" ]] && ! kill -0 "$owner" 2>/dev/null; then
    rm -rf "$slot"
  fi
  return 1
}

announced=
while true; do
  for ((i = 1; i <= slots; i++)); do
    if take "$i"; then
      export SPHERICON_HEAVY_HELD=1
      status=0
      "$@" || status=$?
      exit "$status"
    fi
  done
  if [[ -z "$announced" ]]; then
    echo "heavy: all $slots slots are busy, waiting for one ($dir)" >&2
    announced=1
  fi
  sleep $((1 + RANDOM % 3))
done
