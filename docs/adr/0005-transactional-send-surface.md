---
status: accepted
---

# Transactional is a first-class send surface, binding templates by reference

sphericon supports **transactional email** as a third send surface alongside Broadcast and
Automation, so a customer can run marketing _and_ transactional sends through one service
rather than bolting on a SendGrid/Postmark. A **Transactional send** is a single-recipient
email triggered by the customer's own application through the `/api` surface (password reset,
receipt, OTP): the app supplies a Destination, a referenced Template id, and per-call
variables, and the content is rendered at send time.

It composes with the existing send-eligibility model ([[0001-send-eligibility-model]]) rather
than bypassing it: a Transactional send carries **no Sending source**, so it skips Unsubscribe
(layers 2–3 — you cannot opt out of your own password reset), but it **still respects
Suppression**. A hard-bounced or complained Destination is never sent to, transactional or
not — Suppression remains the global hard floor on every send. This is why the eligibility
layers were written with a transactional carve-out from the start; this ADR makes the surface
that exercises it real.

**Transactional binds its Template by reference, not by copy.** This is the surprising part,
and it is the deliberate inverse of [[0003-templates-copied-not-referenced]]. Marketing sends
copy content at author time (sent content is immutable history). A Transactional send instead
references a Template by id and renders its _current_ content with per-call variables, so
fixing a typo in a receipt template instantly corrects every future receipt — without the
customer redeploying their app. ADR 0003 already foreshadowed this ("the reference/propagation
model belongs to transactional templating"); the two binding models now coexist, one per
surface, because the surfaces mean different things: marketing says "send _this_ text",
transactional says "send the _live_ template X with these data".

## Considered options

- **Don't support transactional; stay marketing-only.** Rejected: forces customers to run a
  second vendor for the highest-volume, most deliverability-sensitive mail, and splits their
  sending reputation and suppression state across two services. "One service for everything"
  is the product bet.
- **Transactional copies content like marketing.** Rejected: the whole value of a
  transactional template is that the customer's app sends `template_id + variables` and never
  has to redeploy to change wording; copy-at-configure-time defeats that.
- **Transactional gets its own opt-out / unsubscribe scope.** Rejected: transactional mail is
  by definition not marketing and not subject to marketing consent; modelling it as a Sending
  source would invite "unsubscribe from password resets". It carries no source and skips
  Unsubscribe by construction — but never skips Suppression.

## Consequences

- A new send path renders a referenced Template + variables; it needs a variable/templating
  layer over the HTML body (e.g. `{{name}}`), an implementation detail not fixed here.
- Suppression is now checked by three surfaces (Broadcast, Automation, Transactional);
  Unsubscribe by two (the marketing surfaces only).
- Deliverability facts unify: bounces/complaints from transactional sends feed the same
  Suppression registry, so reputation and do-not-send state are shared across all surfaces.
- Template gains a meaningful reference-by-id use (transactional), even though marketing still
  records no `template_id` — the provenance question left open in 0003 is unaffected.

## Amendment (2026-06-30): a per-send record exists

The original cut deferred any per-send record; it is now built. Each Transactional send writes
a `TransactionalEmail` row — the transactional counterpart of `BroadcastRecipient`: a durable,
queryable send trace (destination, referenced `template_id`, resolved `contact_id`, outcome)
surfaced in the UI, and the publisher of `email.sent` so transactional sends are segmentable
through the Event log like the other surfaces. It is also the synchronous claim behind an
optional `Idempotency-Key` header: the row is inserted on a unique `(workspace, key)` index
_before_ the provider call, so a retried or concurrent same-key request replays the recorded
outcome (or gets 409 while in flight) instead of sending twice — the Event-log DedupKey only
dedupes the event row, never the send. Consistent with the by-reference rule, the record stores
only provenance and outcome, never the rendered content.

## Amendment: the per-send record is the Outbound message

The `TransactionalEmail` record and its `(workspace, key)` claim generalise to every surface as
the **Outbound message** ([[0015-outbound-send-single-chokepoint]]). A Transactional send stays
synchronous and keeps its optional `Idempotency-Key`; the claim, the Suppression check, the
Sending-domain gate and the freeze check now run inside the shared Outbound send module rather
than in the HTTP handler. A reversible hold on the Workspace or its Sending domain is returned to
the client as a 4xx, not recorded as a failed send.
