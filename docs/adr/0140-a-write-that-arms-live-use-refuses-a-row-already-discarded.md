---
status: accepted
---

# A write that arms Live use refuses a row already discarded

ADR 0099 puts a removal's check of Live use and the writes that create Live use under one advisory
lock of the Workspace, and ADR 0108 makes that lock a type, `*ent.WorkspaceLocked`, which the Removal
check, its readers and the arming writes take. Under it, a Broadcast's Schedule already asks its Send
route whether it can send (`Route.Armable`): a disabled, archived or missing Integration is refused,
and so is an Automation's Publish with an email step (GLOSSARY "Send route").

The race test of a Sending domain's deletion (getsphericon/sphericon#1221) covers one order: a
Broadcast scheduled first, then its domain deleted; the deletion waits for the lock and refuses. The
other order passes unnoticed: the domain is deleted first, then the Broadcast is scheduled, and
Schedule never reads the Sending domain of the From it will send from, so it arms the Broadcast on a
discarded domain and only the send stops it, as a Hold. A Broadcast's Send, which sends it now, runs
no arming check at all.

The lock orders the two writes; it does not make the second one look. A removal asks "who uses this
row?"; a write that arms work must ask the reverse, "is every row this work will use still here?".

## Decision

- **A write that arms Live use reads, under the Workspace lock, the rows Live use is made of for the
  work it arms, and refuses when one is gone.** Those rows are the Integration the work sends through,
  by its Send route, as Schedule reads it today, and the Sending domain of the From that Integration
  sends with, which nothing reads at arming today. A discarded Sending domain is refused, naming it,
  as a removal names the users that block it. An unverified one stays what it is today, a Hold at
  send time and not a refusal: its DNS may still come.
- **The other dependencies keep their own checks.** A Segment that is archived is already refused at
  Schedule; an Automation an Enroll targets is checked at Publish (ADR 0125). The Tags and Custom fields
  a Condition names are Definition use: a Condition that names a missing Custom field fails its own
  validation, and a Tag step creates its Tag on first use, so neither is refused here.
- **The check is one function, the inverse of the reader the removal uses.** The Removal check finds
  the users of an Integration or a Sending domain from each live object's sends (`LiveSends` and its
  kin, ADR 0099); the arming write asks the same sends, for its object, which Integration and Sending
  domain they use, and checks they are present. A dependency added to what a send uses is then
  checked on both sides. It takes `*ent.WorkspaceLocked`, so it cannot run outside the lock.
- **The writes are those ADR 0108 says move Live use, and the Send that was missing**:
  - a Broadcast's Schedule, and its Send, which arms as much as Schedule does and now runs the same
    check;
  - an Automation's, a Rule's and a Broadcast series' Publish (ADR 0135), with or without a draft,
    since publishing again re-arms;
  - making an Integration its Channel's default, or changing its config so that its From changes.
    These move every live send that names no Integration, so they read the Sending domain of the From
    the Integration will send with after the write and refuse a discarded one;
  - a series making an issue, the one write the app makes alone, which skips the occurrence with the
    reason instead of refusing (ADR 0135).
- **An active Broadcast series is Live use**, beside the four kinds the glossary lists: its next issue
  will send through its Integration and its From's Sending domain. ADR 0135's Definition use of its
  Segment stands beside it.
- **Both orders are tested on a real commit** (`testhelper.RealDB`) for each removal that a Live use
  check guards and each kind of arming write: a Sending domain's deletion against a Broadcast's
  Schedule, its Send, an Automation's Publish, a series' Publish and issue, and an Integration's From
  change; an Integration's deletion against the same writes and a default's promotion. Each shows that
  the second write waits for the first and refuses or skips, or that the removal refuses.

## Consequences

- The arming check gains the Sending domain of the effective From, from the same description of a
  send the Removal check reads; Broadcast `Send` calls it, as Schedule, Publish and the Integration's
  default and From changes do.
- The glossary's Live use names an active Broadcast series; the race tests gain the delete-then-arm
  order.

## Considered options

- **Each write checking its own rows.** It is how the bug happened: Schedule checked its route and
  forgot its domain, and Send checked nothing.
- **Leaving it to the send's Hold.** The work holds, but a person was told it was scheduled; a refusal
  at the write is the truth while it can still be fixed.
- **Refusing an unverified Sending domain at arming.** Its record may be on its way; ADR 0010 makes it
  a Hold, which lifts by itself.
- **Locking every row the work will use.** ADR 0099 and ADR 0108 rejected it: a Sending domain is found
  by an address's domain, not by a key.
