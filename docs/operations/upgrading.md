# Upgrading

An Upgrade is one command for each way of running an Instance, and none of them stops a running
one. The new version migrates the database while the old one keeps serving, and the old one is
replaced only afterwards, so a failed or interrupted Upgrade leaves your Instance running on a
schema it can use. Customer sites keep sending events across the swap: a restart of a single host
takes seconds, and the tracker and `/collect` cover it (see
[Retrying from a server client](../guide/tracking#retrying-from-a-server-client)).

Migrations are embedded in the binary and **forward-only**: there is no down migration, so the
way back is [a restore](#roll-back), which is why every Upgrade starts with a backup.

## Before you start

1. Take a database dump and confirm it finished. Keep `SPHERICON_ENCRYPTION_KEY` and
   `SPHERICON_JWT_SECRET` unchanged; see [Backup and restore](./backup).

   ```sh
   pg_dump --format=custom --file="sphericon-pre-upgrade.dump" \
     "postgres://backup_user@db.example.com:5432/sphericon?sslmode=require"
   ```

2. Note the version you run, so you can go back. `sphericon version` prints one line on stdout,
   `sphericon v0.4.2 (commit abc1234, built 2026-10-18T10:00:00Z)`, and `sphericon version --json`
   prints the same as an object, for a script or monitoring. For the image use
   `docker run --rm ghcr.io/getsphericon/sphericon:<tag> version`.
3. Ask whether a newer release exists ([`upgrade --check`](#check)).
4. Read the [Upgrade notes](#upgrade-notes) of every release between yours and the target.
   `sphericon upgrade` prints them for you; with Helm or Docker read the
   [releases page](https://github.com/getsphericon/sphericon/releases).

## Majors {#majors}

Releases between two **Majors** only add: a new table, a nullable column, an index. A removal or a
rename waits for a Major. Until 1.0 every `0.MINOR` is a Major (`v0.4.1` to `v0.4.7` stays inside
one, `v0.4.x` to `v0.5.0` crosses one). You may skip any releases inside a Major. Across Majors an
Upgrade goes through the **last release of each Major it crosses**: that release is the oldest one
that still runs on the next Major's schema.

The database records its Major and that oldest release (the floor), and a binary refuses to start
only when it cannot run on them; see [When it refuses](#refusals).

## Bare binary: `sphericon upgrade` {#binary}

One command downloads the release, verifies it, migrates, swaps the binary and, with `--restart`,
restarts the service:

```sh
sudo /usr/local/bin/sphericon upgrade --restart
```

In order, it:

1. downloads `sphericon_<version>_<os>_<arch>.tar.gz` and `checksums.txt` from the
   [GitHub Release](https://github.com/getsphericon/sphericon/releases) for this OS and
   architecture and checks the archive's sha256 against `checksums.txt`, so a damaged download is
   never installed (`HTTP_PROXY`, `HTTPS_PROXY` and `NO_PROXY` are honoured);
2. unpacks the new binary next to the installed one and prints the [Upgrade notes](#upgrade-notes)
   of every release between the two versions. If there are any it stops, having changed nothing,
   until you rerun with `--yes`;
3. runs the new binary's `migrate` while the old one keeps serving. A failed migration stops here
   with nothing swapped;
4. swaps the file atomically and keeps the previous binary beside it as `sphericon.old`;
5. restarts the service (`--restart`), or prints the command that is left to you.

| Flag               | Effect                                                                                                                                           |
| ------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------ |
| `--version vX.Y.Z` | Install this release instead of the latest stable one. A release older than the running one is refused: the command never moves you backwards.   |
| `--from <dir>`     | Use an archive and `checksums.txt` already in this directory instead of downloading; no network call is made. See [Offline](#offline).           |
| `--yes`            | Go past the Upgrade notes. Without it a release that has notes stops after printing them, so a script cannot walk past them.                     |
| `--restart`        | systemd: migrate in a transient unit, swap, restart the service, and exit non-zero if it is not active afterwards. See [With systemd](#restart). |
| `--service <name>` | The systemd unit `--restart` reads and restarts (default `sphericon`).                                                                           |
| `--check`          | Change nothing; only ask whether a newer release exists. Takes no other flag. See [Ask first](#check).                                           |

Without `--restart` the command ends by printing the one step it leaves to you,
`systemctl restart sphericon`, and its `migrate` runs in your shell, so the shell needs the
service's environment (at least `DATABASE_URL` and `SPHERICON_JWT_SECRET`, and `SPHERICON_ENV=production`).
On systemd use `--restart` and the environment comes from the unit.

The command replaces the file it runs from, so it needs write access to that directory. Only a
release build can use it: a build from source (`go run`, version `dev`) is not a release and the
command asks you to install a release binary first.

### With systemd: `--restart` {#restart}

`--restart` needs no second unit file. It reads the service's `User` and `EnvironmentFile` with
`systemctl show`, runs the new binary's `migrate` as a transient unit with those properties
(`systemd-run --wait --collect`), swaps the binary, runs `systemctl restart <service>` and then
checks `systemctl is-active`. If the service is not active it exits non-zero, naming the previous
binary and `journalctl -u <service>`. Run it as a user that may use `systemctl` and write the
binary's directory, normally with `sudo`. The unit in [Self-hosting](../self-hosting#systemd) is
the one it expects.

### Ask first: `--check` {#check}

```sh
sphericon upgrade --check
```

It makes one request to GitHub and prints `Up to date: <version> is the latest release.` or
`A newer release is available: <version> (running <current>). Run `sphericon upgrade`.`. The exit
status is `0` when you are current, `10` when a newer stable release exists and `1` on an error,
so cron or a monitoring check can ask:

```sh
sphericon upgrade --check; [ $? -ne 10 ] || echo "a sphericon release is available"
```

### Across Majors {#majors-walk}

When the target lies beyond the next Major, `upgrade` stages the target, asks it which Majors lie
between (`migrate --status --json`, which reads your database) and installs the last release of
each Major in turn, then the target. Before the first migration or swap it prints the Upgrade
notes of every release it will install (the target's changelog covers them all) and stops
without `--yes`; `--yes` covers the whole walk. Each step is then a full Upgrade (migrate, swap,
restart).

```text
Crossing 2 Majors: installing 0.4.9, 0.5.6, 0.6.0 in turn.
```

You run one command, and it needs nothing beyond the target's own archive to plan the walk.

### Offline: `--from <dir>` {#offline}

On a host without internet access, fetch the files on another machine from the
[releases page](https://github.com/getsphericon/sphericon/releases), copy them over and point
`upgrade` at the directory:

```sh
ls /srv/sphericon-release
# checksums.txt  sphericon_0.5.0_linux_amd64.tar.gz
sudo /usr/local/bin/sphericon upgrade --from /srv/sphericon-release --restart
```

The directory holds `checksums.txt` and the archive for this host's OS and architecture, named
`sphericon_<version>_<os>_<arch>.tar.gz`; the sha256 is still verified. Without `--version` the
newest archive in the directory is installed. When the Upgrade crosses a Major the directory needs
the last release of every Major in between as well. `upgrade --from` checks every archive of the
walk **before it starts** and, if one is missing, changes nothing and lists the file names it
needs:

```text
this Upgrade needs 1 archive(s) the source does not have (nothing was changed): sphericon_0.4.9_linux_amd64.tar.gz
```

## Helm {#helm}

The chart is published as an OCI artifact at every release, with the chart version equal to the
app version, so no clone of the repository is needed and one command is the whole Upgrade:

```sh
helm upgrade sphericon oci://ghcr.io/getsphericon/charts/sphericon \
  --version 0.5.0 --reuse-values
```

The chart runs `sphericon migrate` in a pre-upgrade hook Job, and the Deployment keeps
`SPHERICON_DB_AUTO_MIGRATE=false`. A failed migration therefore fails the release **before any pod
is replaced**: the old pods keep serving. Once the hook succeeds the Deployment rolls its replicas
behind the readiness probe (`GET /readyz`) and the disruption budget. To see why a hook failed,
read the logs of the release's `-migrate` Job.

`--reuse-values` keeps your values, including `existingSecret` (the Secret with `DATABASE_URL`,
`SPHERICON_JWT_SECRET` and `SPHERICON_ENCRYPTION_KEY`). A pinned `image.tag` is kept too, so either
leave it unset (it follows the chart's app version) or set it in the same command
(`--set image.tag=0.5.0`). Across a Major, run the command once for each release a
[refusal](#refusals) names, oldest first. Offline, mirror the image and the chart into your own
registry and use that path. `sphericon upgrade` refuses to run on Kubernetes and names this command.

## Docker {#docker}

`sphericon upgrade` never replaces a binary inside an image: it refuses and names this command.
Pull the new tag, migrate with it while the current container keeps serving, then recreate:

```sh
docker pull ghcr.io/getsphericon/sphericon:0.5.0

docker run --rm \
  -e SPHERICON_ENV=production \
  -e DATABASE_URL="postgres://app@db.example.com:5432/sphericon?sslmode=require" \
  -e SPHERICON_JWT_SECRET="$SPHERICON_JWT_SECRET" \
  ghcr.io/getsphericon/sphericon:0.5.0 migrate

# then recreate the container (or `docker compose up -d`) on the new tag
```

Use an explicit version tag in production, never `latest`, so a restart does not upgrade you by
accident. With several replicas leave `SPHERICON_DB_AUTO_MIGRATE` off, so a heavy migration never
runs under an orchestrator's liveness probe, and roll the replicas one at a time, waiting for
`GET /readyz` to return `200` on each. Across a Major, migrate and recreate on each release a
[refusal](#refusals) names, oldest first. Offline, move the image with `docker save` and
`docker load`.

After an upgrade or a change of configuration, [`sphericon doctor`](./doctor) checks the whole Instance.

To look before you act, `migrate --status` prints the schema's current Major and release, the
newest the binary knows and the migrations that would run, and changes nothing (`./sphericon
migrate --status` for a binary, or the same `docker run ... migrate --status`):

```text
schema:  42
binary:  45
state:   Major 1, oldest release that runs: v0.0.0 (this binary: Major 1)
pending: 3
  ...
```

## When it refuses {#refusals}

A binary refuses to start, and `upgrade` stops, only when it cannot proceed safely. A refusal stops
the new binary, never the one that serves. The three guard messages each name the version, the
commit and a link to this page:

- **"is older than the database allows ... needs vX.Y.Z or newer."** This binary is older than the
  database's floor, usually a rollback past a removal. Start the release it names or a newer one. Do
  not try to migrate down: [restore the backup](#roll-back) if you must go back.
- **"skipped a Major ... install vX.Y.Z first, then upgrade again."** The database is two or more
  Majors behind this binary. Install the release it names (the last of your database's Major), start
  it, then upgrade again. `sphericon upgrade` makes this walk itself; an image tag or a manual
  install skipped it.
- **"is one Major ahead of the database ...: run `sphericon migrate`, or set
  SPHERICON_DB_AUTO_MIGRATE=true."** A newer binary on a database that has not been migrated. Run
  `sphericon migrate` (or `sphericon upgrade`), or turn on `SPHERICON_DB_AUTO_MIGRATE` to migrate on
  start (a start waits for the lock for at most `SPHERICON_DB_MIGRATE_TIMEOUT`).

A binary of the database's own Major starts, and so does the last release of the previous Major
(the floor of the next), which is why an interrupted walk leaves the Instance running. A build
with no release number (`dev`) is never refused as too old.

`sphericon upgrade` itself stops with:

- **"the release has Upgrade notes: read them above and rerun with --yes."** Do what the notes say,
  then rerun with `--yes`.
- **"refusing to downgrade."** `--version` named a release older than the running one. `upgrade`
  only moves forward; to go back, [restore a backup](#roll-back) and start the older release.
- **"this Instance runs in a container"** or **"runs on Kubernetes".** Use [Docker](#docker) or
  [Helm](#helm); the message names the command.
- **"this build is ... not a release."** Built from source; install a release binary first.
- **"checksum mismatch"** or **"has no entry in checksums.txt".** The archive is damaged or is not the
  one `checksums.txt` lists. Download it again; nothing was installed.
- **"migrate with X (nothing was swapped)".** The migration failed and your Instance is exactly as
  it was. Fix the cause the error names (database reachable, privileges, disk) and run the command
  again: `migrate` takes a database lock, so repeating it is safe.
- **"this Upgrade needs N archive(s) the source does not have."** With `--from`, a Major crossing
  needs more archives than the directory holds. Add the files it lists and run again.
- **`<service> is <state> after the restart.`** The service did not come up on the new binary. Read
  `journalctl -u <service>`; the previous binary is kept as `sphericon.old`.

## Roll back {#roll-back}

Migrations only go forward, so a rollback means returning to the pre-upgrade state:

1. Stop every replica.
2. Restore the pre-upgrade dump into an empty database (see the
   [restore drill](./backup#restore-drill)) and point `DATABASE_URL` at it, or drop and
   recreate the database you are restoring over.
3. Start the **previous** binary or image. Do not point it at the migrated database: after a Major
   it refuses, as [above](#refusals).

Anything written after the dump is lost, and the points in
[After a restore](./backup#after-a-restore) apply. If the Upgrade is only minutes old, weigh
whether fixing forward is cheaper than losing that window.

The kept `sphericon.old` shows what you replaced. It also covers one narrow case: the new release
would not start and the Upgrade stayed inside one Major, where the old binary still runs on the
migrated schema. Anything else is a restore.

## Upgrade notes {#upgrade-notes}

sphericon is versioned with release-please from Conventional Commits. The changes of each release
are on the [GitHub releases page](https://github.com/getsphericon/sphericon/releases). A commit that
breaks something carries an `Upgrade:` line saying what a Self-hoster must do, and those lines are
where a new required setting, a removed one or a changed default is recorded; there is no
per-version table to keep in step.

The binary carries its own changelog, so the notes of the release you are about to install are
there offline and match it exactly. `sphericon upgrade` prints them before it changes anything. To
read them on their own, with the new binary:

```sh
sphericon upgrade-notes --since v0.4.2   # the Upgrade: entries of every release after v0.4.2
```

It prints nothing when there are none.

## PostgreSQL major upgrades {#postgres}

A PostgreSQL major upgrade (for example 16 to 17) is a database administration task,
independent of sphericon releases. Do not combine it with an application upgrade: change one
thing at a time, so a failure has one suspect.

1. Check that the target major version is supported (see [Self-hosting](../self-hosting) for
   the supported range).
2. Take a fresh dump and rehearse the whole procedure on a scratch copy first.
3. Upgrade in place with `pg_upgrade`, or dump from the old server and restore into a new
   server on the new major version (`pg_dump` then `pg_restore`). Managed services offer
   their own major-version upgrade.
4. Stop sphericon during the switch, repoint `DATABASE_URL` if the server changed, start it, and
   check `GET /readyz`.
5. Run `ANALYZE` on the new cluster: planner statistics are not always carried over.

A physical backup or WAL archive from the old major version cannot be restored onto the new
one. Take a fresh base backup right after the upgrade, and keep the pre-upgrade `pg_dump` until
you are confident in the new cluster.
