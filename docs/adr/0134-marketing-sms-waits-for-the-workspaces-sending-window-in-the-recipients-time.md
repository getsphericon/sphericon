---
status: accepted
---

# Marketing SMS waits for the Workspace's sending window in the recipient's time

> **Amended by [[0137-a-frequency-cap-limits-marketing-messages-per-destination]]:**
> the cap on messages per person it queued is decided as a frequency cap per Destination.

Marketing text messages are restricted by the hour in several places, by the recipient's local
time. As law firms and vendors report it: in the United States the TCPA's federal window is 8:00 to
21:00, Florida and Oklahoma narrow it to 20:00, and whether it applies to messages sent with consent
is contested in court; in France no statute sets hours, and the AF2M industry charter in force since
March 2026 allows 8:00 to 21:30 and tolerates Sundays and public holidays. The rules differ by place
and change, and this decision relies on none of them being exact.

A Contact's time zone is optional, and it may be a browser's guess. The phone number library the app
uses maps a number's prefix to the time zones it may be in, and for some prefixes it knows none. A
send that cannot go now but may go later is a **Deferral**: the message is not consumed and is
retried, and a Broadcast stays sending (GLOSSARY "Deferral"; the Send cap and the Provider limit
defer this way).

ADR 0126 left quiet hours to Outbound send, as a guard no author can forget.

## Decision

### What it covers

- **A sending window guards every Broadcast and Automation send on SMS**, and on WhatsApp when it is
  built: marketing by surface, as ADR 0012 has it. It sits in Outbound send, the one place every
  send passes (ADR 0015).
- **Transactional sends are outside it**, and so are the messages the app sends because the person
  just asked: an SMS confirmation of an opt-in, a fresh link of the Preference page (ADR 0121). A
  one-time code is needed now. An Automation answering a person's own message is a marketing send
  and waits like any other.
- **Email and Telegram are outside it.** No law sets hours for them; a Workspace that wants a time of
  day for them uses Wait until (ADR 0126).
- **A cap on how many messages a person gets** (Florida's three on one subject in 24 hours) is not
  part of it and is queued on its own.

### The Workspace sets the window

- **A window is hours of the day and days of the week, in the recipient's local time.** A Workspace
  has one, on by default: 8:00 to 20:00, every day. That sits inside the US federal rule, Florida's
  and Oklahoma's, and the AF2M charter's hours; "every day" includes the Sundays the charter only
  tolerates, and public holidays are not modelled. The Workspace may narrow or widen it and add
  windows for countries, by the number's country.
- **A window that never opens in a week is refused**, a country's included, so no message waits for
  ever (the reason ADR 0124 and ADR 0126 bound every wait).
- **The app keeps no table of laws by country or state.** It would be incomplete and out of date, and
  the app ships no legal text (ADR 0062). The setting says the Workspace answers for its compliance.

### The recipient's local time

- **The window must be open in every zone that may be the recipient's**: the Contact's time zone when
  it has a valid one, and the zones the number's prefix may be in, as the phone number library maps
  them; for a prefix it cannot map, every zone of the number's country. A guessed time zone narrows
  the window but never opens it where the number says it is closed, and a person travelling is safe
  either way.
- **When no moment of the week is open in all of them** (a narrow window across a country as wide as
  Russia, or a Contact's zone far from its number's), the message is Skipped, with the reason that no
  local time could be found, and counted. It is never sent at night, and it does not wait for ever.
- **Never the Workspace time zone**: it says nothing about where the recipient is.

### Outside the window a message waits

- **A message outside the window is a Deferral until the window opens**, per recipient: it is neither
  failed nor skipped. A Broadcast sends to the Recipients inside their window on its schedule, stays
  sending while the rest wait, possibly into the next day, and sends to them as their windows open;
  an Automation's send step waits where it stands, and the Enrollment moves on once it sends.
- **Everything is read again when the window opens.** A deferred message goes through Outbound send
  again: Send eligibility (an opt-out or a STOP received overnight), Suppression, a Hold of the
  Workspace or the Integration, an unscheduled Broadcast and an Enrollment ended by an exit or goal
  Event all apply at release, not when the message was first tried.
- **It is normal, not trouble.** The Broadcast's and the Automation's reports show how many wait for
  the window; Needs attention does not. Before Schedule, the editor says what share of the audience
  would be outside its window at the scheduled time and when it is reached.
- **Nothing gets around it**: the window is in Outbound send, not in a Step, so a Wait until or a
  Branch cannot skip it. A Test send, which goes to the number its author types, is outside it.

## Consequences

- Outbound send gains the window check for SMS sends of Broadcasts and Automations, a per-recipient
  Deferral with its release time, and a Skipped reason for a recipient with no local time; the
  Broadcast planner holds the Recipients outside their window.
- The Workspace gains the window and its country windows; the reports gain the waiting count and the
  sends judged by a single zone; the Broadcast editor gains the share outside the window.
- Queued: a cap on messages per person per period.

## Considered options

- **A table of each country's and state's rules.** It is legal advice the app cannot keep current.
- **A window on each Broadcast or Step.** An author could forget it; a Workspace's rule belongs where
  every send passes.
- **Skipping a message outside the window.** The person would miss a message they consented to for
  being asleep.
- **The Workspace time zone when the Contact has none.** It is the sender's clock, not the
  recipient's.
- **Sending at the first listed zone when the zones disagree.** It sends at night to everyone in the
  others.
- **The Contact's time zone alone when it has one.** A browser's guess would open the window where
  the number's own zones say it is night.
- **Quiet hours for email.** No law asks for them, and mail waits in the inbox.
