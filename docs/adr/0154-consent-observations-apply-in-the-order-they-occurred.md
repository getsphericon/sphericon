---
status: accepted
---

# Consent observations apply in the order they occurred

> Settles the ordering [[0114-mandatory-sms-reactions-run-in-the-publishing-transaction]] left
> separate, and amends [[0141-a-destinations-consent-history-is-read-from-its-evidentiary-events]]
> (an observation that changed nothing is in the history too).

## Context

ADR 0114 makes every accepted consent observation (a STOP or START, a blocked or unblocked bot, an
unsubscribe button, a link) consumed exactly once: a redelivered STOP cannot undo a later START. It
sets no order between distinct observations that arrive late or together, or through two bots, so
today the one handled last wins. A provider may deliver out of order: START at 10:00 and STOP at
10:01, delivered STOP first, leaves the number subscribed although the person's last word was stop.
An opt-out lifted by a return leaves no row (ADR 0141 reads it from its Events), so there is nothing
for a late, older STOP to be compared with.

Providers give time differently. A Telegram bot's `update_id` grows with each update of that bot, and
each message carries a `date` in seconds. A WhatsApp webhook carries the time of each message. An
inbound SMS through Twilio carries none. A click on a link, a member's action, a form and an import
happen when the Instance receives them, or at the time an import states.

Sending to someone who refused costs far more than not sending to someone who agreed: regulators and
providers treat the first as the violation.

## Decisions

- **An observation is ordered by when it occurred, not when it arrived.** Within one Telegram bot by
  `update_id`; otherwise by the provider's time where it gives one (Telegram's `date`, WhatsApp's
  timestamp); otherwise by when the Instance received it (an SMS without a time, a link, a member's
  action, a form). An import that states when consent was given or withdrawn is ordered by that time.
  Clocks are not corrected for skew.
- **A consent state remembers when the observation that set it occurred**, per (Destination, Channel,
  Sending source), a return from an opt-out included: the return keeps that time although the opt-out
  row goes. A new observation changes the state only when it occurred later. One that occurred
  earlier is recorded in the history at its own time and causes no transition, so none of the
  reactions to a transition (ADR 0114) run.
- **A tie goes to the refusal.** Two observations of the same moment, or of an order that cannot be
  told (the same second through two bots), leave the opt-out standing. Observations of one Destination
  on one Channel are handled one at a time, so the comparison and the write are one step.
- **Two bots share one Telegram consent**, as the Destination is the user's id (ADR 0077): between bots
  observations are ordered by `date`, within a bot by `update_id`, and a tie goes to the refusal.
- **Provenance comes before time.** An observation that may not replace the standing state is not
  saved by being later: only the person lifts an Unsubscribe, a Confirmation's stronger provenance
  stands, a tracking `denied` gives way only to the person's own grant (ADR 0120). Time orders only the
  observations allowed to replace each other.
- **One rule for every consent fact of a Destination**: Unsubscribes and returns, Confirmations and
  their withdrawal, Engagement tracking consent. Suppression is not consent but a fact about delivery:
  it is set when observed and lifted only by its owner, outside this rule.

## Consequences

- Each consent state gains the time of the observation that set it, kept for a return as well.
- A consent observation carries its time of occurrence from the adapter that read it.
- The consent history shows an observation that changed nothing, at its own time, marked as such:
  it is recorded for the Contact as the System event `contact.consent_superseded` (#1448), a
  record of the observation, not of a transition.
- Glossary: **Consent observation**.

## Rejected

- **Arrival order**: the provider's delivery queue is not the person's order of acts, and its error
  can turn a refusal into consent.
- **A tie to the latest arrival**: it keeps the same failure for the moments it cannot tell apart.
- **Comparing only with a standing opt-out row**: a return leaves no row, so a late, older STOP would
  always win after a START.
