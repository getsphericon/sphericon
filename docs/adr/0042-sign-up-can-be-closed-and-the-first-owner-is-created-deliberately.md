---
status: accepted
---

# Sign-up can be closed and the first Owner is created deliberately

An Instance answers `signup` to anyone who reaches it, so a Self-hoster who exposes a fresh
Instance hands the first Workspace to whoever arrives first. Four of the self-hosted projects read
(`docs/research/self-hosted-deploy-prior-art.md`) have a deliberate first-admin step or a switch
that closes open registration.

## Decisions

- **`SPHERICON_SIGNUP_OPEN` closes public sign-up.** Only an explicit `false` closes it; the default
  and any other value keep it open, as with `SPHERICON_REQUIRE_EMAIL_VERIFICATION` (ADR 0030). A
  closed `signup` answers `403` with the problem code `signup_closed` before it reads or creates
  anything. Invitation accept is a different path and is untouched.
- **`sphericon user create-owner <email> <name...>` makes the first Owner.** It creates a Verified
  User with its first Workspace (the same transaction as a sign-up with the verification gate off)
  and a generated password it prints once. The bootstrap token was not reused: it mints an API token
  on the oldest Workspace and creates no User, so it cannot make an Owner.
- **The default stays open, for the open-core Instance and the SaaS alike.** A Self-hoster on a
  private network or a laptop should not need a step to try the product, and the SaaS is meant to
  take public sign-up. First-run docs tell a Self-hoster to close it before exposing the host.

## Considered Options

- **Closed by default**: safer for a public host, but every first run needs a CLI step before the UI
  works, against the try-it-in-a-minute path; rejected. The ticket asked to record the default, and
  the open default changes nothing from today, so no migration or upgrade note is needed.
- **A one-time setup page for the first Owner** (as Gitea and Mattermost have): the same race, moved
  to the setup page, unless it is guarded by a token; rejected for the CLI step.
