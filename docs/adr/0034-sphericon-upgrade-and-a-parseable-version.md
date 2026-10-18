---
status: accepted
---

# `sphericon upgrade` swaps the binary and migrates first; `sphericon version` names the build

One command upgrades a bare-binary Instance, and `helm upgrade` upgrades one on Kubernetes. Both
rest on the Upgrade rules of ADR 0033 and on the binary knowing which release it is: without that
neither the command, a diagnosis, nor the guard's message can say what runs where.

## Decisions

- **`sphericon version` prints one line on stdout**, `sphericon v0.4.2 (commit abc1234, built
  2026-10-18T10:00:00Z)`, and `--json` prints the same as an object. The release build sets
  version, commit and date; a build without them (`go run`, `go install`) falls back to the Go
  build info (`vcs.revision`, `vcs.time`) rather than printing `dev`, and the Dockerfile passes
  the same three values.
- **`sphericon upgrade` does four things and nothing else.** It downloads the archive for this OS
  and architecture and `checksums.txt` from the GitHub Release and verifies the sha256 (the
  releases are not signed yet; that is a separate task); unpacks the new binary next to the old
  one; runs the new binary's `migrate` while the old one keeps serving; and swaps the file
  atomically, keeping the old one as `.old`. Before it changes anything it prints the `Upgrade:`
  notes of every release between the two versions and, if there are any, waits for `--yes`. It
  refuses to run inside a container or Kubernetes and says which command to use instead.
- **It reaches the database through the service, not by itself.** Its shell does not carry the
  service's environment, so `upgrade --restart` reads the service's `User` and `EnvironmentFile`
  with `systemctl show` and runs the new binary's `migrate` as a transient unit
  (`systemd-run --wait --collect` with those properties), so there is no second unit file to
  install; then it swaps and runs `systemctl restart sphericon`. A failed migration stops there
  with the old version still serving. Without `--restart` it prints the commands.
- **Across Majors it walks** to the last release of each Major, then to the target; every step is the
  sequence above. `--version vX.Y.Z` pins a release, a downgrade is refused, and `--from <dir>` takes
  an archive and `checksums.txt` already on the machine, so a host without internet access
  upgrades with no network call; it lists the archives a Major crossing still needs before starting.
- **The notes travel in the binary.** `CHANGELOG.md` is embedded, so the new binary prints its own
  `Upgrade:` entries offline. A commit with `!` or `BREAKING CHANGE:` must carry an `Upgrade:`
  line inside its `BREAKING CHANGE:` footer saying what the Self-hoster does (release-please renders
  only that footer's text into the changelog), which `cmd/upgradecheck` checks in CI; the release notes are the only
  place for it, with no per-version table kept by hand.
- **Helm: a chart on `ghcr.io/getsphericon/charts/sphericon`**, published as an OCI artifact at every
  release with the chart version equal to the app version, so `helm upgrade sphericon
  oci://ghcr.io/getsphericon/charts/sphericon --version X --reuse-values` is the whole Upgrade. The
  pre-upgrade hook runs `sphericon migrate`; the Deployment sets `SPHERICON_DB_AUTO_MIGRATE=false`.
  Two replicas, a disruption budget and the readiness probe already make the roll free of downtime.
- **Knowing a release exists is the Release check of ADR 0032**, not a second mechanism: its result
  also goes to a log line at start, to a `sphericon_release_available{latest}` gauge next to
  `sphericon_build_info{version,commit}` on the metrics listener, and to `sphericon upgrade --check`,
  which exits `0` when current, `10` when a release exists and `1` on error, so cron or a
  monitoring check can ask. The app never makes any other outbound request of its own for this.

## Considered Options

- **`sphericon upgrade` runs `migrate` itself.** Rejected: it runs from a shell that has none of
  the service's environment, so it would need the database URL given a second time.
- **A `sphericon-migrate.service` oneshot unit next to the service.** Rejected: a second file to
  install and keep in step with the first; a transient unit built from the service's own
  properties has nothing to install.
- **Restart by default.** Rejected: a command that restarts a service nobody told it about
  surprises; the flag names it.
- **A second update check with its own switch and an email to a configured address.** Rejected:
  ADR 0032 already has the Release check, its switch and its banner for the owner.

## Consequences

Every release needs the notes embedded and the chart published, which the release workflow does.
A signed release is the next step for `upgrade`'s trust, and the single-host restart gap closes with
socket activation if it ever matters.
