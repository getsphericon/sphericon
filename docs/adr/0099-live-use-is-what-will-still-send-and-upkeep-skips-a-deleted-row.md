---
status: accepted
---

# Live use is what will still send, and upkeep skips a deleted row

ADR 0049 lets live dependencies block a removal and says which readers see discarded rows, but an
audit of the code found the rules read three ways. Deleting an Integration counted a Broadcast that
names none as using the default email Integration whatever its Channel, so a scheduled Telegram
Broadcast neither blocked deleting the Telegram default nor let the email default go. Deleting a
Sending domain matched only the default Integration's From address, while a send uses the From of
the Integration it goes through. The check and the discard ran without a lock, so work could be
scheduled in between. Periodic jobs that keep a configuration working (the quota refresh, `sphericon
doctor`, a queued DKIM notice) still reached deleted rows, and the analytics dashboard dropped
archived Automations from a count the ADR said it keeps.

## Decisions

- **Live use is defined by where work will actually send** (glossary **Live use**): a scheduled or
  sending Broadcast, the live version of an active Automation, any Automation version an unfinished
  Enrollment stands on (an inactive Automation's included), and an active Rule. A Broadcast or a
  step uses the Integration it sends through: the one it names, else its Channel's default, never
  another Channel's. It uses the Sending domain of the From address it sends from: its own From,
  else the From of that Integration. The removal check and the send path compute this with the same
  function. A draft is not Live use.
- **Live use blocks; nothing else does.** A row in Live use answers 409 to Archive and to Delete
  (ADR 0100), naming what uses it.
- **A check and its removal do not race.** Archiving or deleting a Segment, an Integration, a
  Sending domain or a Tag, retyping a Custom field, and the writes that create Live use (scheduling
  a Broadcast, Publish, saving a Segment or a Rule), take one transaction-scoped advisory lock of the
  Workspace: these are rare, and a lock per referenced row is awkward for a Sending domain, which is
  referenced by the domain of an address, not by a key. The hot paths (a Rule firing, an enrollment
  from a Rule) lock the row they act on and check it again under the lock, as an enrollment from a
  Trigger already does.
- **History counts what is archived or deleted; inventory does not.** Figures of what happened
  (messages sent, clicks, conversions, a Broadcast's report, the names on them) include archived and
  deleted rows. Counts of what the Workspace has set up (how many Automations, how many active) and
  what asks for attention (failed Enrollments, failed Broadcasts) leave them out, and so does the
  Instance report's list of providers in use.
- **Upkeep skips a deleted row.** A job that keeps a configuration working (the DKIM and Tracking
  domain re-checks, the Provider limit refresh, `sphericon doctor`, the notice that a domain lost
  its verification) skips deleted rows, and checks again when it runs, not only when it was queued.
  This replaces the one named exception of ADR 0049. A job that processes work or history (sending,
  tracking, provider callbacks, erasure, export, rekey) still sees every row.

## Considered Options

- **Serializable transactions** instead of a lock: every path that reads a dependency would have to
  retry on a serialization failure, and the paths are spread over five modules.
- **No lock**: a send that lost the race holds (`no_integration`, an unverified domain), so nothing
  wrong is sent, but the person who deleted the row never saw the 409 that would have told them.
- **Only an active Automation is Live use**, as ADR 0049 worded it: an Automation deactivated with
  Enrollments still in flight sends through the Integration of the version they stand on, so its
  deletion would hold them.
