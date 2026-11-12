---
status: accepted
---

# A Wait for event keeps the Event that ended it for later Steps

A Wait for event (ADR 0124) moves an Enrollment on when an Event arrives, "a purchase within three
days", but nothing after it can use that Event: a thank-you message cannot name the product bought.
An Enrollment keeps one copy of an Event, the one that began it, with its properties bounded at
32 KiB as a Rule sees them; every send step, on every Channel, and every HTTP step binds it as
`event` (`event.action`, `event.properties.<key>`). A Branch reads the triggering Event's properties
folded in memory (ADR 0122), and Publish does not check the names a template uses: a template naming
a deleted Score renders nothing (ADR 0129).

## Decision

- **A Wait for event that takes `arrived` on an Event of an action it names keeps a copy of that
  Event**, its action and its properties bounded as `event`'s are, on the Enrollment under the Step's
  id: taken once, never changed. That includes an arrival the timeout finds already recorded (ADR
  0124). A Wait for event on an open or click of the Enrollment's own message keeps none: that Event is
  the app's, and the message is already the Enrollment's.
- **An Event an agent wrote without the send lock satisfies no Wait for event.** An Event whose cause
  marks it as written through a Connection without `mcp:send` (ADR 0132) does not take a Wait for event
  to `arrived`, whose path may send, and so is never copied into what a message prints.
- **A Wait for event has a name**, a key fixed when the Step is created, like a Custom field's or a
  Score's: lower-case letters, digits and underscores, starting with a letter, unique in the
  Automation. Templates read `awaited.<name>`, the same shape as `event`:
  `{{ awaited.purchase.properties.total }}`. As for any merge tag, Publish does not check the names a
  template uses; a name with no copy renders nothing.
- **Where there is no copy, it is empty.** After `timed_out` or unknown, or on a path that never
  passed the Step, `awaited.purchase` renders nothing, as a Custom field the Contact lacks does. The
  editor warns when a Step uses a name not every path before it passes, and the Content check of a
  merge tag that renders empty (ADR 0130) shows it.
- **Which Steps read it.** Every send step and HTTP step binds `awaited` beside `event`. A Set field
  takes a literal, a property of `event` or a property of `awaited.<name>`. A Notify's text may name
  it. An Enroll step carries only `event` into the new Enrollment (ADR 0125): the new Automation
  awaits nothing of the first one.
- **A Branch reads it too**: a leaf `awaited:<name>.<key>`, where the name runs to the first dot and
  the key is the rest, folded in memory as a triggering Event's property is. It is two-valued: no copy,
  or no such property, fails the test. It may name only a Wait for event that can come before the
  Branch, by the rule ADR 0122 sets for testing an earlier message, and a Condition naming none is
  refused. It is valid in a Branch only: a Segment, a Rule, a Wait for event's Condition and an Event
  matcher's filter (ADR 0133) refuse it, as they refuse the tests of an Enrollment's own messages.
- **The copy lives as long as the Enrollment runs.** It is cleared when the Enrollment ends: nothing
  reads it after, and an awaited `email.replied` or `sms.received` holds the person's own words, which
  an Enrollment row, kept for reports, must not outlive Retention with. Its properties are untrusted
  (ADR 0132), and the Enrollment resource returns them so marked. Each Enrollment of a Contact
  enrolled again has its own (ADR 0127), a Merge moves it with its Enrollment (ADR 0128), and an
  Erasure deletes it with the Enrollment.

## Consequences

- A Wait for event gains a name; a running Enrollment keeps the arrived Event of each, by Step id, and
  clears them when it ends.
- Send, HTTP, Set field and Notify steps bind `awaited`; the Branch's Condition gains the
  `awaited:<name>.<key>` leaf.

## Considered options

- **Naming it by Step id.** Ids are opaque to an author and unreadable in a template.
- **Overwriting `event` with the awaited Event.** `event` would mean different things at different
  Steps, and the triggering Event would be lost.
- **Reading the Contact's latest Event of that action when rendering.** It could be a later Event than
  the one that moved the Enrollment on.
- **A name the author can rename.** Templates are not checked at Publish, so a rename would silently
  empty them; a fixed key cannot.
