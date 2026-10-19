---
status: accepted
---

# Templates are copied at author time, never referenced (marketing)

> Scope: this decision governs **marketing** sends (Broadcast, Automation step). The later
> [[0005-transactional-send-surface]] binds Templates by _reference_ for transactional sends —
> the deliberate opposite, one model per surface. This ADR's body already anticipated that
> ("the reference model belongs to transactional templating").

A Template is a reusable starting point for email **content** (subject + HTML body). When a
Broadcast or an Automation email step is authored, it takes a _copy_ of the template's
content — there is no foreign key and no live reference back to the Template. Editing or
deleting a Template therefore never changes any already-authored, in-flight, or sent message.

This is the surprising part worth recording: a future reader will expect "edit the template →
all emails using it update". They do not, and that is deliberate. Sent and in-flight content
must be immutable (you cannot retroactively change an email a provider already accepted), and
marketing sends each own their content. The reference/propagation model belongs to
transactional templating (SendGrid/Postmark dynamic templates), not to marketing sends.

Template, Broadcast, and each email Automation step all embody the same **Message content**
value (subject + HTML body); the Template is just its saved, named, reusable instance. The
three are linked by copy, never by reference.

## Considered options

- **Reference a Template by FK from Broadcast/Automation.** Rejected: edits would mutate
  in-flight/sent content, and deleting a template would orphan live sends.
- **Record an informational, nullable `template_id` (provenance only, not a constraint).**
  Open — left to a follow-up decision. It would answer "which template did this broadcast
  start from?" for analytics without affecting content immutability. The default leaning is
  _not_ to add it (keep Template a pure content library).

## Consequences

- The "copy" currently happens client-side (the composer prefills the body); the backend has
  no notion that a template was involved. If provenance is ever wanted, it must be added
  explicitly — it is not recorded today.
- A clean rebuild should model `subject + HTML body` once as a Message-content value and reuse
  it across Template, Broadcast, and Automation step, rather than three independent shapes.
