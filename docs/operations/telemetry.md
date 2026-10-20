# Telemetry and the Release check

An Instance talks to the outside world on two channels, each with its own switch
([ADR 0032](https://github.com/getsphericon/sphericon/blob/main/docs/adr/0032-instance-telemetry-report-and-release-check.md)).
This page says exactly what each one does, how to turn it off and how to see what would be sent.

| Channel          | What it does                                                                | Sends data about your Instance |
| ---------------- | --------------------------------------------------------------------------- | ------------------------------ |
| Telemetry report | Sends the project one anonymous report a day.                               | Yes, the fields listed below.  |
| Release check    | Asks GitHub once a day for the newest stable release, for an update banner. | No.                            |

Both are on by default. Refusing the report does not hide the update banner: a self-hoster who
declines telemetry still learns about a security release.

## The Telemetry report

Once a day the Instance sends one JSON document to `https://telemetry.getsphericon.app`. The
receiver stores no source address. The report carries:

- **Identity of the Instance:** a random `instanceId` stored in your database and derived from
  nothing, and a `schemaVersion`.
- **The build:** `appVersion`, `edition` (`core` or `ee`), `os`, `arch` and the deploy shape
  (`binary`, `container`, `kubernetes` or `unknown`).
- **The environment:** the Postgres major version, the configured locale (`en`) and
  `instanceAgeDays`, the whole days since the first start.
- **Order-of-magnitude counts:** `workspaces`, identified `contacts` (erased ones excluded) and
  `emailsSent30d`. A count is a bucket (`0`, `1-9`, `10-99`, `100-999`, `1000-9999`, `10000-99999`,
  `100000-999999`, `1000000+`), never an exact number.
- **Usage flags:** whether a Broadcast was sent in the last 30 days (`usedBroadcasts30d`), an
  Automation is active (`automationsActive`), the tracker recorded a visit in the last 30 days
  (`usedTracker30d`), an external API token was used in the last 30 days (`usedExternalApi30d`) and
  a webhook endpoint is configured (`webhooksConfigured`).
- **`integrationKinds`:** which sending Integrations are in use (`smtp`, `ses`), each at most once.

The [report schema](./telemetry-schema) is the contract: it lists every field, and every report an
Instance builds is checked against it. A new field is a schema change documented in the same commit.

### When it is sent

The Instance checks every hour whether a report is due and sends at most one a day. Each Instance
reports at its own fixed time of day, 0 to 6 hours after midnight UTC, derived from its Instance
id, so Instances that started together do not all report in the same minute. The first report
waits until an Owner has seen the one-time notice in the app (Instance settings) and then until
that offset has passed. A send has a five-second timeout and no retry queue: a failed send is logged
at debug level and the next hourly check is the retry. It never affects boot, readiness or health.

Nothing is sent from a development or test environment or from a build that
is not a release (a prerelease or a development build).

## The Release check

Once a day, and at start, the Instance does one `GET` of the public GitHub Releases `latest`
(prereleases are excluded). It sends nothing about your Instance: no id, no version, no counts. If
a newer stable release exists, Owners of the oldest Workspace see an update banner and the log
shows a line pointing to `sphericon upgrade --check`. An unreachable GitHub changes nothing. A
build that is not a release never asks.

## The switches

| Variable                  | Effect                                                                                                                                              |
| ------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------- |
| `SPHERICON_TELEMETRY`     | `true` sends the report and locks the in-app switch on, `false` never sends it and locks it off; unset leaves the choice to the Owner (default on). |
| `DO_NOT_TRACK`            | `1` switches the report off and locks the in-app switch; it can never switch the report on.                                                         |
| `SPHERICON_RELEASE_CHECK` | `false` switches the daily Release check off; it is independent of the Telemetry report and `DO_NOT_TRACK`. Default `true`.                         |

These are the same three rows as in the [environment table](../configuration). Precedence for the
Telemetry report, first match wins:

1. `SPHERICON_TELEMETRY=false` turns it off.
2. `DO_NOT_TRACK=1` turns it off.
3. `SPHERICON_TELEMETRY=true` turns it on.
4. Otherwise the in-app switch decides, and it is on until an Owner turns it off.

When the environment decides, the in-app switch is shown locked and says which variable did it.
Otherwise the Owners of the oldest Workspace (every Owner when several share it, following a
transfer of the role) change it under Instance settings.

To send nothing at all, set `SPHERICON_TELEMETRY=false` and `SPHERICON_RELEASE_CHECK=false`.

## What is never sent

No name, slug, domain, email address, host name, error text,
licence key, Contact, message content or exact number. Nothing identifies a Workspace or a person.
The Instance id is random and cannot be tied to you. A database cloned from another Instance (a
staging copy) shares its id.

## Preview the report

Under Instance settings an Owner can open "Preview the report": the body exactly as the Instance
would send it now, built by the code that sends it. It works whether or not sending is on, and
opening it sends nothing.
