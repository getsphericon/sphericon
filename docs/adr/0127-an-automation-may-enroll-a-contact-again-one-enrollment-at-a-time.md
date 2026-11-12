---
status: accepted
---

# An Automation may enroll a Contact again, one Enrollment at a time

A Contact is enrolled in an Automation at most once ever, enforced by a unique index on the
Automation and the Contact. That fits a welcome series and fails a post-purchase series, an
abandoned-cart reminder or a win-back: each should run again the next time its Event comes. ADR 0126
gave date Triggers a repeat of their own, once per occurrence; Event Triggers still have none.

Once-ever also does a second job: it is what ends a cycle of Automations that enroll each other,
through an Enroll step (ADR 0125) or through an Event one Automation's Step causes and another's
Trigger reads, such as `contact.tagged`. Allowing repeat entry takes that bound away, so it needs a
bound of its own.

Notifuse offers `once` and `every_time`, and `every_time` opens a second, parallel journey while the
first runs.

## Decision

### An entry policy per version

- **An Automation version with an Event Trigger has an entry policy: `once` or `again`.** `once`,
  the default, is today's rule: a Contact with any Enrollment in the Automation, on any version, is
  not enrolled again. `again` enrolls a Contact again once the previous Enrollment has ended. The
  policy of the version an Enrollment would begin on decides. It applies to every way in: the
  Trigger, a Rule's Enroll Action and an Enroll step. A date Trigger keeps its own key, the
  occurrence (ADR 0126), and has no entry policy.
- **Any end allows another Enrollment**: `completed`, `exited`, `goal_reached` and `failed`. An
  unsubscribe from the Automation does not stop it: it is about messages, so the new Enrollment's
  send Steps are skipped while its other Steps run, as an opt-out from `everything` already leaves
  a Contact enrolled in a new Automation.

### One Enrollment at a time

- **A Contact has at most one active Enrollment in an Automation.** A Trigger while one is active
  is skipped and counted. Parallel Enrollments would break what the model stands on: which one an
  exit or goal Event ends, which one a Wait for event wakes, which one an unsubscribe exits. A
  post-purchase series for each of two orders a day apart therefore runs once; a flow per order is a
  short Automation or a Rule.
- **Whether an Enrollment is active is judged at the Event's moment.** A triggering Event that
  occurred before the previous Enrollment ended is skipped as arriving while it was active, even
  when it is handled after the end, so a late or backdated Event does not start a second run of
  what it happened during.
- **A cooldown** sets the least time from the beginning of the previous Enrollment to the next
  ("at most once in 30 days"). It counts from the beginning, so it caps frequency whatever the
  length of the path. **It is at least one hour, which is also its default.** The cycle guard below
  sees only chains inside the app; a round trip outside it, such as an HTTP step whose receiver
  records an Event back, starts a new chain, and without a floor it would loop at machine speed. One
  hour bounds such a loop to 24 Enrollments a day per Contact. An entry within the cooldown is
  skipped and counted.

### Once per Event, and no cycle

- **One Event enrolls a Contact in an Automation at most once.** The Enrollment is unique on the
  Automation and the triggering Event's id; a Rule's Enroll is unique on the Firing, an Enroll step
  on the enrolling Enrollment and the Step id. A redelivery does nothing.
- **An Automation never enrolls a Contact again through a chain that passed through itself.**
  Everything published on behalf of an Enrollment carries the Automations its chain passed through,
  its own included: the Events its Steps cause (`contact.tagged`, `contact.field_changed`), its
  lifecycle Events (`automation.enrolled`, `completed`, `exited`, `goal_reached`) and the
  Enrollments its Enroll steps make. Whatever is published while handling one inherits that list,
  through Rules and other Automations. An entry whose cause lists the Automation it would enroll in
  is skipped and counted. That ends A enrolling A through its own Tag or its own
  `automation.completed`, and A enrolling B enrolling A, whatever the path, while an unrelated Event
  in the same request still enrolls. The list rides in the Event's envelope beside the Rule's
  `CausedBy` (ADR 0054), and a Step resumed after a wait keeps its Enrollment's list.
- **It refuses a deliberate loop too, however long its waits.** A that waits 90 days, enrolls in B,
  and B back in A, is skipped at the second A: the list does not age. A recurring journey is an
  Event the outside world records again, a date Trigger or a cooldown, not a cycle of Automations.
- **What the guard does not see is bounded by the rest**: one active Enrollment at a time and the
  cooldown's floor bound an external round trip and a person opening or clicking the Automation's
  own message.

### Splits and reports

- **A Split draws again for each Enrollment.** Its hash takes the Enrollment's ordinal, the n-th of
  this Contact in this Automation, beside the Contact and the Step (ADR 0124), as a date-triggered
  one takes the year (ADR 0126). A Contact in a holdout is not left in it for good.
- **The report counts Enrollments, not Contacts.** The rate of goal reached is a share of
  Enrollments; beside it stands the number of distinct Contacts. Skipped entries are counted by
  reason, once per Event: an active Enrollment, a cooldown, a cycle, and `once`. An Enrollment made
  already ended, by an exit or goal Event recorded before it, is an Enrollment like any other: it
  counts for the ordinal, the cooldown and `once`.

### Ordering

- **The entry checks run under the Contact's lock**, which making an Enrollment already takes: `once`
  loses its unique index, and two Events handled at once must not both find no Enrollment.

## Consequences

- The unique index on the Automation and the Contact becomes a unique index on the active
  Enrollment of a Contact in an Automation, plus one on the Automation and the triggering Event; a
  `once` version checks for any Enrollment, under the Contact's lock, before it makes one.
- Queued: a flow per key (one Enrollment per `order_id`), which this decision leaves to a short
  Automation or a Rule.
- The version gains the entry policy and the cooldown; the contract, MCP and the editor follow.
- The event envelope gains the list of Automations a chain passed through, set by Steps and kept by
  every handler and job that publishes from one.
- The Automation report gains distinct Contacts and skipped entries by reason.

## Considered options

- **Parallel Enrollments** (Notifuse's `every_time`). They leave exit and goal Events, Waits for
  event and unsubscribes with more than one Enrollment to act on.
- **A key from an Event property** (`order_id`), one Enrollment per key. It allows a flow per order,
  but the same ambiguity returns between Enrollments of different keys, and a Rule or a short
  Automation covers the case.
- **No cooldown by default.** It leaves a round trip through an HTTP receiver unbounded.
- **A cooldown from the end of the previous Enrollment.** It makes the cap depend on how long the
  path took: an Enrollment that waited a month would push the next one out by a month more.
- **An unsubscribe from the Automation blocking entry.** It would stop its Tags and HTTP calls too,
  which the opt-out is not about.
- **Ending cycles by the correlation id alone.** One request that records two Events for a Contact
  shares one correlation id, so a legitimate second entry would read as a cycle; the list of
  Automations names only chains that went through the Automation.
- **The same Split path on every Enrollment.** It keeps a holdout Contact out of every future run.
