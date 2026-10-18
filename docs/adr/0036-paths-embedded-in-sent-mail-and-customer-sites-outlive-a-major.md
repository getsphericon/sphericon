---
status: accepted
---

# Paths embedded in sent mail and customer sites outlive a Major

An Instance puts its own paths where it cannot update them: the tracker snippet (`/t.js`) and
`/collect` on customer sites, and the signed unsubscribe, confirm and tracking links in mail that is
already delivered. A Major may remove what earlier releases added (ADR 0033), but removing one of these
paths breaks pages and inboxes the Self-hoster does not control, long after the Upgrade.

## Decisions

- **The paths of the tracker (`/t.js`, `/collect`) and of the signed links keep working for at least
  one Major after the release that replaces them.** A Major may add the new path and stop
  advertising the old one; it may not remove the old one. Only the next Major after that does.
- **Signed link tokens stay verifiable across Majors** while their own expiry has not passed.
- **`SPHERICON_APP_URL` is part of every link already sent.** Changing it is supported only with a
  redirect from the old origin to the new one at the reverse proxy, and the upgrade and self-hosting
  guides say so.

## Consequences

A Major's release notes name any path it stops advertising, and the removal is a breaking commit one
Major later. The Instance cannot enforce the `APP_URL` rule: it is documentation, with `sphericon
doctor` warning when the configured origin differs from the one in recent sent mail.
