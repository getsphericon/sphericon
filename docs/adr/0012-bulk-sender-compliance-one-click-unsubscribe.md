---
status: accepted
---

# Bulk-sender compliance: RFC 8058 one-click unsubscribe & DMARC readiness

> **Amended by [[0121-a-preference-page-per-destination-and-only-the-person-lifts-a-refusal]]:** the
> footer carries one link, to the Preference page, which replaces the confirmation page, the
> `/unsubscribed` page and its `everything` link below. The one-click `POST` is unchanged.

> **Amended by [[0130-deliverability-is-observed-through-feedback-ids-gmails-postmaster-data-and-content-checks]]:**
> a `Feedback-ID` sphericon writes is signed in the same `h=` list; unlike the unsubscribe headers,
> a message without one is still sent.

Gmail/Yahoo/Microsoft enforce bulk-sender requirements (2024+) as the **receiving** side: any
mail landing in those inboxes — regardless of the transport it was sent through (SES, SMTP, …) —
must be DKIM-authenticated, come from a domain with a published DMARC record, and (for senders

> ~5k/day) carry a **one-click unsubscribe** (RFC 8058), with the spam-complaint rate held
> < 0.3%. Non-compliant mail is spam-foldered or rejected. The transport does not do this for us:
> SES ships whatever MIME we hand it and adds no `List-Unsubscribe` headers. So sphericon must build
> the headers itself, in the message, before handing it to any provider.

This ADR covers the two forks still open after ADR 0011 settled the complaint-rate metric:
one-click unsubscribe (RFC 8058) and DMARC posture. Both are table-stakes deliverability that a
self-hoster sending bulk to Gmail needs identically to SaaS, so both are **AGPL core**, no EE
split.

## Decisions

### Rides on the ADR 0010 send-path rework (sequencing)

The current send stack (`nikoksr/notify`) exposes no custom-header support and cannot DKIM-sign;
`go-mail` is not a dependency (the backlog's "already a dep" note is wrong). ADR 0010's native
DKIM signing **already** forces replacing this with a raw-MIME-building + signing stack. The
`List-Unsubscribe` headers ride on that same rework — they physically cannot exist until the
send path builds raw MIME. The _design_ below is independent of the stack choice; only its
implementation sequences with/after ADR 0010.

**Critically, the send-path rework must include both `List-Unsubscribe` and
`List-Unsubscribe-Post` in the DKIM-signed header list (`h=`), not merely emit them.** RFC 8058
§5 requires the one-click headers to be covered by the DKIM signature (so a forwarder/attacker
cannot inject a forged unsubscribe URL); Gmail/Yahoo honor one-click **only** when the headers
are within the signed scope. Headers added _outside_ `h=` produce mail that looks correct, emits
no error, and silently fails one-click — the exact trap this note exists to prevent.

### Headers on Broadcast + Automation only, never Transactional

`List-Unsubscribe` + `List-Unsubscribe-Post` go on the two **marketing** surfaces —
**Broadcast** and **Automation** — and **never** on **Transactional**. This mirrors the existing
model exactly: Broadcast and Automation already carry the footer unsubscribe link
(`UnsubscribeFooter`) and run the full Send-eligibility stack including Unsubscribe;
Transactional carries no Sending source, skips Unsubscribe (eligibility layers 2–3), and respects
only Suppression. You cannot (and per the RFC must not) offer to unsubscribe from a password
reset or an OTP.

### Header reuses the footer's source-scoped token, never `everything`

The header's HTTPS URI carries the **identical source-scoped `UnsubTarget` token** the footer
link already builds — `broadcasts` for a broadcast, `automation:<id>` for an automation. One-click
and the visible footer link become semantically identical, differing only in transport (MIME
header vs HTML link). The reserved **`everything`** scope stays reachable only by deliberate human
escalation — a mailbox provider's one-click button must never silently detach the whole contact.

### Endpoint: `GET` = confirm (no state change), `POST` = perform

The unsubscribe endpoint is redesigned to method-differentiate, replacing today's
GET-records-immediately behavior:

- `GET /e/u/{token}` → renders the SPA **confirmation page**, records **nothing**.
- `POST /e/u/{token}` → **performs** the opt-out (writes the Unsubscribe row, exits an automation
  enrollment, fires the engagement Event — the existing `recordUnsubscribe` transaction).

The RFC 8058 header points at this URL; a compliant mailbox provider POSTs
`List-Unsubscribe=One-Click` and the opt-out happens immediately with no landing page. The human
footer link is a GET → confirm page → its "Confirm" button POSTs the same URL.

This **fixes a latent bug**: recording a state change on GET violates HTTP safe-method semantics,
and email link scanners / security proxies (Mimecast, Barracuda, Gmail Safe Links, corporate
gateways) routinely GET every link in a message — silently unsubscribing legitimate subscribers.
After the change, a scanner's GET is a no-op; only an explicit POST opts out. RFC 8058's
POST requirement exists for exactly this reason. The cost is one extra click on the human footer
path (confirm-first instead of instant-with-undo); the mailbox one-click path — what most users
actually use — skips it via POST.

### HTTPS URI only; `mailto:` deferred

`List-Unsubscribe` carries only the HTTPS URI plus `List-Unsubscribe-Post: List-Unsubscribe=One-Click`.
That combination **is** RFC 8058-compliant and satisfies the Gmail/Yahoo one-click mandate; the
`mailto:` variant is not required for it. Acting on a `mailto:` unsubscribe needs an inbound-mail
parsing pipeline sphericon does not have (the only inbound path is the SNS bounce/complaint webhook).
Shipping a `mailto:` we cannot act on is worse than omitting it, so it is deferred until an
inbound-mail pipeline exists.

### DMARC = warn-not-block bulk-readiness signal; send-gate stays DKIM-only

The send-gate remains **DKIM-only**, unchanged from ADR 0010 — a valid published DKIM record is
what lets a domain send. DMARC becomes a **distinct bulk-readiness signal** on the Sending domain:
Gmail hard-requires a published DMARC record (at least `p=none`) for bulk senders, and a domain
with valid aligned DKIM but **no** `_dmarc` record still fails that policy — the mail is
**rejected (a 5xx → bounce, inflating the ADR 0011 bounce rate) or spam-foldered** (killing
deliverability and engagement), and a bounce spike can trip auto-suspension. So a missing DMARC
record raises a **prominent warning before the blast** but does **not** block sending. sphericon
generates the suggested `_dmarc` TXT (`v=DMARC1; p=none; …`), checks its presence, and guides
toward `quarantine`/`reject` over time — but never mandates the stricter policies, which live on
the organizational domain and can break the sender's unrelated mail streams.

## Considered options

- **Rely on SES/provider list-management to add List-Unsubscribe** (rejected): couples the header
  to one provider, breaks for raw SMTP, and fragments the model the moment a second transport
  lands (same reasoning as ADR 0010's native-signing choice). sphericon owns the MIME.
- **A separate POST-only one-click endpoint, leaving the footer GET as-is** (rejected): avoids the
  UX change but leaves the footer's scanner-unsubscribe bug unfixed; once the POST-performs path
  exists for the header, keeping a second scanner-unsafe GET-records path is strictly worse.
- **Include a `mailto:` unsubscribe** (deferred): needs an inbound-mail pipeline we lack; a dead
  unsub mailbox hurts more than its absence.
- **Hard-gate sending on DMARC** (rejected): contradicts ADR 0010's deliberate DKIM-only gate and
  risks halting a live domain over a policy that affects the org's unrelated mail.
- **A new `everything`-scoped one-click** (rejected): a mailbox provider's automated button must
  never detach the whole contact; `everything` stays a deliberate human escalation.

## Consequences

- Blocked on the ADR 0010 send-path rework: headers require raw-MIME building.
- The unsubscribe UX changes from instant-with-undo to confirm-first on the human footer path; the
  SPA gains a confirmation page, and `recordUnsubscribe` moves from the GET handler to a POST
  handler. `GET /e/u/{token}` becomes a safe, idempotent page render.
- Broadcast/Automation MIME gains two headers; Transactional is untouched.
- The Sending domain surfaces a bulk-readiness state (DMARC published) alongside `verified`
  (DKIM) — two independent signals, one gating, one advisory-with-teeth.
- All three Gmail/Yahoo bulk pillars are now specified across ADRs: authentication (ADR 0010
  DKIM), one-click unsubscribe + DMARC (this ADR), and complaint-rate < 0.3% (ADR 0011).

## Amendment: owned by Outbound send, and fail-closed

Building the footer link, both RFC 8058 headers and the `h=` coverage is part of the single
Outbound send module ([[0015-outbound-send-single-chokepoint]]), identical for Broadcast and
Automation. If a marketing message cannot get its unsubscribe artefacts it is a failed send,
never sent without them; the opt-in confirmation mail is not a marketing send and must not carry a
placeholder target. Transactional is still untouched. Cited prior art: Listmonk ships one-click
headers on opt-in mails pointing at an all-zero placeholder campaign, so a provider's one-click
"succeeds" and changes nothing.
