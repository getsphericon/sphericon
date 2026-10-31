---
status: accepted
---

# The Send cap is the platform's limit, the Provider limit is the customer's

ADR 0023 gave each Integration a ceiling that mirrors the customer's own provider account, and the
app showed it as a "Send limit" the customer edits. It read as a limit the platform sets, which it
is not: it protects the customer's provider account. The SaaS needs a limit of its own that the
customer cannot change, because tenants who bring their own SES or SMTP still put the platform's
reputation at risk. The links of every message sent without a serving Tracking domain are built on
the Instance's host, and a blocklist such as Spamhaus DBL lists a domain found in message bodies
together with all its subdomains ([send limits research](../research/send-limits-prior-art.md)).
So there are two limits with two owners, and each gets its own name.

## Decisions

- **Provider limit** (GLOSSARY) is the new name of ADR 0023's Send rate limit, in code, contracts
  and UI. It is the customer's self-protection and exists on every Instance. The app says so: "the
  limits of your provider account, so a large send does not trip them".
- **Send cap** (GLOSSARY): the most messages a Workspace may send per rolling 24 hours, across all
  its Integrations. A Staff member sets it in the Staff console, and the Workspace cannot change it.
  - **`saas` only.** A self-hosted Instance does not limit what its Workspaces send: its operator
    and its senders are usually the same organization. The Mode decides it (ADR 0060), not the
    License key: on `selfhosted`, even with Enterprise features, there is no Send cap in the
    console, the app or the send path.
  - **Per Workspace, per day only.** Per Integration, a second Integration would get round it. The
    platform's risk is volume, not speed: the speed belongs to the Provider limit.
  - **It joins ADR 0023's minimum.** The effective limit is the least of the manual and provider
    values, the warmup and the Send cap. A spent Send cap is a **Deferral**, not a Hold: sending
    slows, it does not stop. A Broadcast shows its ETA and that the platform's Send cap is the
    cause, and the Workspace's settings show the cap read-only. A Broadcast larger than the cap is
    not refused: it is warned about before it starts and goes out over several days, faster as
    soon as Staff raises the cap.
  - **No exceptions.** Transactional sends count against it and, as in ADR 0023, are never delayed
    by it. The Test inbox counts like any Integration, so a test shows what real sending would do.
  - **Set by hand, no expiry.** A Staff member sets a default for new Workspaces and a value per
    Workspace; every change goes to the Audit log with its author. The console warns while no
    default is set. The abuse detector (ADR 0026) does not change it. Unlike a Staff hold, it has
    no expiry: the operator sets it directly, and no outside service has to renew it.
- **The Send cap is not an allowance.** ADR 0009 says the core keeps no allowance of its own: what a
  Workspace's commercial terms allow is managed outside and acts through Staff holds. The Send cap is
  reputation protection, like Workspace suspension (ADR 0007), and holds no commercial meaning; its
  name and copy carry none.
- **What the provider says about the account counts too.** Where the provider's limit cannot be
  read, the customer gives it:
  - An SES Integration with its own endpoint (an SES-compatible service such as Yandex Cloud
    Postbox, which has no quota call and starts new accounts at 1 per second and 200 a day) needs
    both Provider limit values, on every surface, or the write is a 422.
  - SMTP stays optional, as many SMTP servers have no limit.
  - For SES, the hourly quota read also reads `EnforcementStatus` and production access:
    - `SHUTDOWN` is a **Hold** (the provider has stopped the customer's account) that lifts when the
      status recovers.
    - `PROBATION` and the sandbox are warnings on the Integration.
  - A failed read still never blocks sending.

## Considered options

- **No Send cap, only Staff hold and suspension**: a stop is all or nothing, so a new tenant sends at
  its provider's full rate or not at all; rejected.
- **A Send cap set by the external service, with an expiry like a Staff hold**: on the SaaS only the
  platform's own Staff run the console, so there is nothing to mediate, and an expiring cap fails
  open; rejected.
- **The core derives the cap from the Tracking domain's state**: policy of the hosted offering
  written into the AGPL core; rejected.
- **A Send cap per Integration**: spread over as many Integrations as the Workspace adds; rejected.
- **A Send cap on every Instance with the Enterprise features**: the self-hosted operator does not
  need to limit its own sending, and an Instance default it could not override per Workspace
  without the key would be a trap; rejected.
- **A built-in default per known provider endpoint** (Postbox → 1/200): a vendor list in the core
  that goes stale; rejected in favour of requiring the values.
- **A default Send cap from an environment variable that `saas` requires at start**: it is a value
  the operator changes as it runs, not one only the deployment can give; rejected.
- **Exempting the Test inbox**: a test would no longer show what real sending does; rejected.

## Consequences

- ADR 0023's term, its contract fields and its package take the Provider limit name.
- A third, per-Workspace bucket joins the two per-Integration ones, and Transactional decrements
  it without waiting, as it does the others.
- Fairness between Workspaces in the shared broadcast queue is not addressed here: a Send cap
  bounds a Workspace's volume, not its share of the workers.
