# An Instance sends an anonymous Telemetry report, opt-out, and checks for releases on a separate channel

A self-hosted Instance sends the project one **Telemetry report** a day, on by default, so the
project can see which versions, deploy shapes and features are in use. The report is anonymous and
exactly documented: its schema is a TypeSpec contract (`typespec/telemetry`, compiled to an OpenAPI
file published in `docs/`, with an ogen-generated client), so the documentation is the contract and a
test compares every built report against it. It carries the app version and edition, OS, arch,
deploy shape, Postgres major, locale, the Instance's age, order-of-magnitude counts (Workspaces,
Contacts, emails sent in 30 days) and "used in the last 30 days" flags. It never carries a name, slug,
domain, address, host, error text, licence key or an exact number. The **Instance id** is a random
value stored in the database, derived from nothing.

The **Release check** is a separate channel: a daily GET of the public GitHub Releases `latest`
(prereleases excluded) that sends nothing about the Instance, feeding an admin-only banner. It has its
own switch, so a self-hoster who refuses telemetry still learns about a security release.

Consent: both are on by default. `SPHERICON_TELEMETRY` and `SPHERICON_RELEASE_CHECK` switch them off
independently; `DO_NOT_TRACK=1` switches the report off and can never switch it on. An explicit
environment value wins over the in-app switch, which then shows as locked. Otherwise the owner of the
Instance's first Workspace decides in the app. That owner sees a notice once, with a link to the
schema and a preview of the report as it would be sent (available even when sending is off), and the
first send waits until the notice has been seen. The SaaS deployment sets `SPHERICON_TELEMETRY=false`
in its deployment (the platform repository); `dev` builds and the test environment never send.

A send is fire-and-forget: a five-second timeout, no retry queue (the next cycle is the retry), a
debug-level log line, no effect on boot, readiness or health. The receiving service lives outside this
repository and stores no source address.

## Considered Options

- **Opt-in.** Rejected: opt-in installs report in single-digit percentages, which makes the data
  useless; the notice, the preview and the published schema carry the honesty instead.
- **One channel and one switch for version check and report (the first ROADMAP draft).** Rejected:
  refusing telemetry would also hide the "a release exists" banner, a security cost the self-hoster should
  not pay for privacy.
- **PostHog SDK in the binary.** Rejected: a third-party SDK and a third party seeing every address for
  a single daily POST; our own endpoint decides what is kept.
- **A "this is the SaaS" flag.** Rejected: the only thing it would do is turn the report off, which
  `SPHERICON_TELEMETRY=false` already does.

## Consequences

A new field in the report is a schema change with its documentation in the same commit; a field that
could identify a Workspace or a person is not added. A database cloned from another Instance (a staging
copy) shares its Instance id; that noise is accepted and there is no way to regenerate the id in the
app. The Release check depends on GitHub's API and rate limits; a static manifest on our own domain
replaces it if that becomes a problem.
