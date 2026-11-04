---
status: accepted
---

# A deleted Integration or Sending domain is never restored

> Amended by [[0098-a-rule-is-deleted-not-archived]]: a Rule is deleted the same way, read by id
> with `deletedAt`, and has no archived view.

> Amended by [[0100-archive-is-not-discard]]: the field is `discardedAt`, and a deleted Integration or
> Sending domain is read-only.

ADR 0049 discarded an Integration and a Sending domain and named the state `archived` in the
contract, with an `archived` list filter and an `unarchive` operation, and ADR 0066 then gave the app
a view of the archived ones to restore them, as for a Broadcast or an Automation. Unlike those, a
provider or a domain is not work a person comes back to: an archived one is a credential or a DNS
setup nobody sends with. The view added a place to look and a second state to reason about, and
restoring a domain had its own refusal (the same domain added again meanwhile).

## Decisions

- **Deletion is final.** On `/site` and `/api` an Integration and a Sending domain have a `DELETE`
  operation (204), refused with 409 while live work uses them, as archiving was. There is no
  `unarchive`, no `archived` list filter and no view of the deleted ones in the app. The app confirms
  with the typed name: the deletion cannot be undone.
- **The row is still discarded, not removed** (ADR 0049). History needs it: the Outbound messages,
  reports and the reputation figures it took part in, the Automation version that names it, and the
  provider callbacks its hook still reads. A by-id `get` still answers for a deleted row, with
  `deletedAt` set, so what names it can show its name.
- **The field is `deletedAt`.** The contract's word for these two is delete; `discarded_at` stays the
  column and the mechanism.
- Broadcasts, Automations, Segments and Rules keep their archived view and restore (ADR 0049).

## Consequences

ADR 0049's "Segment, Integration and Sending domain take the mixin too, with `archived` in the
contract" holds for the Segment alone; ADR 0066's gap "restoring an archived Integration or Sending
domain" is closed by removing the capability, not by a screen. The Audit log still records the
deletion as `integration.discard` and `sending_domain.discard`.
