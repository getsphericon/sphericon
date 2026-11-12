---
status: accepted
---

# An Automation sets a field, enrolls elsewhere and notifies Members

> **Amended by [[0133-a-trigger-exit-or-goal-matches-an-event-by-its-action-and-properties]]:**
> a goal or exit on a given Tag is expressible: `contact.tagged` with a filter on `tag`.

> **Amended by [[0129-a-score-is-computed-from-events-and-attributes-never-kept-as-a-counter]]:** no
> increment is added to Set field: a Score is computed from Events and Conditions, never counted.

> **Amended by [[0126-an-automation-starts-on-a-date-and-waits-until-a-local-time]]:** an Enroll,
> Step or Rule Action, cannot target an Automation with a date Trigger.

> **Amended by [[0127-an-automation-may-enroll-a-contact-again-one-enrollment-at-a-time]]:** cycles of
> Enroll steps are no longer bounded by once-ever alone: an entry caused by a chain that passed
> through the target Automation is skipped. "Already enrolled" below means what the target version's
> entry policy refuses: an active Enrollment, `once`, a cooldown or a cycle.

An Automation's Steps send, wait, change Tags, call HTTP, branch and end. A Rule's Actions change
Tags, call HTTP and enroll in an Automation. Three things marketers ask an Automation to do have no
Step: write what it learned onto the Contact ("plan = pro"), hand the Contact to another Automation
at a point of its path, and tell someone on the team ("a lead from the pricing page wants a demo").

Today they are done around the model: a Tag stands in for a value; a Tag step plus a Rule on
`contact.tagged` stands in for the hand-off; an HTTP step to Slack stands in for telling the team,
and covers no team without a chat tool.

## Decision

### Three Steps, and the same three Actions for a Rule

- **Set field, Enroll and Notify are Step kinds.** Set field and Notify become Rule Actions too, so
  a Step and a Rule share one set of actions: apply and remove Tag, Set field, HTTP, Enroll and
  Notify. A Step's values come from the Enrollment's `event`; a Rule's from the Event it fired on.
- **No Step records an Event.** An Event is what a customer or a provider reports; what an
  Automation does is already published as `contact.tagged`, `contact.field_changed` and
  `automation.*`. A goal on one Tag is not expressible, since a goal event is an action with no
  filter on its properties; that gap is queued on its own.
- **No Step adds to a number.** Incrementing a field ("score + 10") belongs to lead scoring, decided
  on its own.

### Set field

- **It gives one Custom field a value or clears it.** The value is either a literal, checked
  against the field's type when the version is saved, or a copy of one property of the Enrollment's
  `event` (`plan` from `event.plan`). Core fields are not written: a name, a locale or a time zone
  comes from the person or from Identify.
- **The field is picked from the catalogue.** A Step never creates one by use; declared-by-use stays
  ingest's (ADR 0006).
- **It is the `system` actor's change**, as a Tag step's is (ADR 0022), and it records
  `contact.field_changed` only when the value changes.
- **A value that does not fit is a skip, not a failure.** When `event` lacks the property, or holds
  it with a type the field does not read, the Step is recorded as skipped with that reason and the
  Enrollment moves on, as a skipped send does. No data arrived; nothing broke.
- **The field is in Definition use** while a kept Automation's live version or draft, or a Rule's,
  names it, so the Removal check refuses to remove it or to retype it, and `contacts` asks
  `automations` as it asks `segments`, `rules` and `broadcasts` (ADR 0102). An older version an
  Enrollment still stands on holds nothing: its Set field on a field that is gone is a skip.

### Enroll

- **It enrolls the Contact in another Automation's live version, and the current Enrollment goes
  on.** An author who means a hand-off follows it with an End step.
- **The new Enrollment begins when the Step runs**, not when the first Automation's triggering Event
  occurred, and carries a copy of the first one's `event`. Beginning at the older moment would let an
  exit or goal Event recorded in between end it as it is made. A Rule's Enroll is unchanged: its
  Enrollment begins when the Event it fired on occurred (ADR 0054), since nothing lies between.
- **An inactive target, or a Contact already enrolled in it, is a skip**, as the Rule Action's is
  (ADR 0054). A version cannot enroll in its own Automation; the save is refused.
- **Cycles are bounded by the once-ever Enrollment**: A enrolls in B, B's Step back to A is skipped.
  Repeat Enrollment has to settle cycles again when it is decided.
- **The target is held as a Rule's Enroll holds it today** (ADR 0099, ADR 0100): the live version
  of an active Automation that enrolls in it keeps it from being archived, and the refusal names that
  Automation; an inactive one holds nothing. A Publish that names an archived target is refused. Two
  Automations that enroll in each other are archived by deactivating one first.

### Notify

- **It mails chosen Members of the Workspace about the Contact.** The recipients are Memberships,
  never addresses: a Member who leaves stops getting it, and no Notify reaches someone outside the
  Workspace. Memberships are not in Definition use; removing one always works. Any Member may be
  chosen, since every Role reads Contacts; a Permission to read them, if one is added, would bound
  the choice.
- **It is a Notice, sent through the System sender**, in a fixed frame: which Automation or Rule,
  which Contact, and a link to the Contact. The author adds a short plain text that can name the
  Contact's fields and `event`, never free HTML, so a Step cannot turn the Instance's own sender into
  a sender of the Workspace's mail.
- **What a Contact supplied is rendered as inert text.** A name or an Event property comes from
  ingest, so in the mail it is escaped, never turned into a link and cut to a fixed length; a value
  cannot put a link or markup into mail from the sender that carries verification and reset mail.
- **Notify has a volume cap.** A Rule fires on every matching Event, so a Workspace's Notify
  Notices are capped per hour by an Instance setting. One over the cap is a skip with that reason,
  counted, and the Workspace's Needs attention shows it while it lasts.
- **An Instance with no outbound mail has no System sender**, so a version with a Notify cannot be
  published on it.
- **A Notify whose Members have all left is a skip.** A Notice that fails to deliver does not fail
  the Enrollment: it is sent off the Step.
- **Slack, Teams and other chat tools stay with the HTTP step.**

### Run once

- **Every new Step commits its effect with the Enrollment's move**, as a Tag step does: the field
  write, the new Enrollment and the Notice's job are written in the transaction that moves the
  Enrollment on, so a Step that lost a race does nothing and a restarted worker does nothing twice.
  The Notice's job is keyed by the Enrollment and the Step id, and sends the mail once, after the
  move. None of the three is fenced (ADR 0046): an exit after the move cannot recall a Notice
  already queued, and a field write or an Enrollment is a data change, not a message.
- **A Rule's Actions run once per Firing** (ADR 0054): a Set field writes in the Actions job, and a
  Notify queues its job keyed by the Firing and the Action. Like an HTTP call, a Notify outlives the
  Actions job; unlike one, its delivery is not awaited and never fails the Firing.
- **A Rule now mails Members, never Contacts.** ADR 0054's "no email Action" means no message to a
  Contact; that still holds.

## Consequences

- The version's Step tree and the Rule's Actions gain three and two kinds; the contract, MCP, the
  editor and the Rule editor follow.
- The Notice catalogue gains one kind, whose content is partly the author's.
- The Removal check reads Set field and Enroll as Definition use.
- The Instance gains a setting for the hourly Notify cap, and Needs attention an item for it.
- Queued: a goal or exit on a property of an Event (a given Tag), and incrementing a field with lead
  scoring.

## Considered options

- **A record-Event Step.** It would let one Automation signal another precisely, but it makes the
  app the author of Events that read as the customer's, and the gap it closes is narrower: a goal on a
  given Tag.
- **Set field creating a field by use.** A typo in a Step would create a field and give the Segment
  builder a field no one meant.
- **Failing the Enrollment on a missing property.** A Step without data is the same case as a
  send without a Destination, which moves on.
- **Enroll by a Tag and a Rule on `contact.tagged`.** It works today, but the hand-off is invisible on
  the canvas, and the Rule fires on every Tag and filters in its Condition.
- **The new Enrollment beginning at the first one's triggering Event.** Events between would end it
  at once.
- **Notify to any address, or through the Workspace's own Sending domain.** Any address makes it a
  sender of the Workspace's mail outside Outbound send, with no Contact, no consent and no
  Suppression; the Workspace's domain mails Contacts, and a Member is not one.
- **No Notify, the HTTP step only.** It leaves a team without a chat tool with no way to hear of a
  lead.
