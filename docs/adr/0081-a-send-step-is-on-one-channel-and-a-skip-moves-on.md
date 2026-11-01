---
status: accepted
---

# A send step is on one Channel, and a skip moves on

The Automation step is the one surface where every Channel meets (the map issue,
getsphericon/sphericon#916). Today an Automation version holds one email Integration for all its
steps, its send step is an email, and a Suppression or a hard bounce exits the Enrollment: with one
Channel, a run that kept walking past skipped emails would send nothing.

## Decisions

- **The step's kind fixes its Channel.** There is one send step per Channel (email, SMS, WhatsApp,
  Telegram, web push, mobile push), each with its Channel's Message content (ADR 0078). Every
  product in the multichannel research does the same; a step that picks a Channel per Contact
  needs the branching that is deferred.
- **The Integration moves from the version to the step.** Each send step names its Integration, or
  takes its Channel's default. A hold on an Integration (ADR 0079) stops only the Enrollments waiting
  at a step that sends through it.
- **A skipped send moves on.** When a send step's message is Skipped — no Destination on the
  Channel, an opt-out from the Channel, a Suppression — the Enrollment goes on to the next step and
  the skip is recorded and reported. The Enrollment is `exited` only by an unsubscribe from this
  Automation (scope `automation:<id>`) from any Channel, or by an exit event. A STOP on SMS is an
  opt-out from SMS (ADR 0077), so the sequence's emails continue. The old rule, that a Suppression
  or a hard bounce exits the Enrollment, is dropped: it would end an email-then-SMS sequence for
  every Contact with no phone.
- **Publish checks every send step.** Publish is refused while any send step's route cannot send,
  per its Channel and Integration, or while a WhatsApp step's template is not approved; a later
  pause holds that step (ADR 0079).
- **Channel events trigger as any Event.** `telegram.bound`, `web_push.bound`, `sms.received` and
  the other System events of ADR 0076 serve as a Trigger, an exit event or a goal event with no
  change.

## Consequences

- An Enrollment can walk a sequence whose every message was skipped; the Automation report shows
  the skips instead of the Enrollment ending early.
