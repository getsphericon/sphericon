---
status: accepted
---

# An expired license key falls back to the core instead of stopping the Instance

`licensekey.Parse` treated an expired key like a forged one (`ErrInvalid`), and a key that does not
verify fails boot so that a mistyped key never runs unlicensed. The result for a Self-hoster whose
subscription ended is an Instance that will not start after its next restart: sending stops, and the
tracker loses the events of that window (ADR 0033). A lapsed payment must not cost data or mail.

## Decisions

- **A key that is malformed or whose signature does not verify still fails boot.** That is a
  mistake or a forgery, and failing loudly is the point.
- **An expired key with a valid signature loads as a License with no features.** The Instance runs
  the open-source core, exactly as with no key. Nothing is deleted: the Audit log, retention windows
  and every other Enterprise row stay in the database, unreachable until a valid key is set.
- **The lapse is visible.** A warning at start naming the expiry date, a line in the admin banner of
  the Instance's owner, and a metric (`license_expired`), so the SaaS deployment alerts on it.
- **Expiry is judged at boot only**, as today: a running process keeps the features it started with.

## Consequences

An Enterprise feature that guards data (the Audit log, retention) goes dark on expiry rather than
destroying anything, so renewing restores it intact. The platform's own Instance, whose Operator
console depends on the `staff` feature, must alert on `license_expired` before a restart can take the
console down. A key can no longer be told apart from "no key" by whether the Instance starts; the
warning and the metric are how the Self-hoster learns.
