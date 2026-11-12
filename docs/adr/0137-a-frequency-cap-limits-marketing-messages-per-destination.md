---
status: accepted
---

# A frequency cap limits marketing messages per Destination

Nothing limits how many messages one person gets. The Send cap bounds a Workspace's day and the
Provider limit an Integration's pace; neither looks at the recipient. Florida's telephone
solicitation act, as law firms report it, allows at most three messages on one subject to a person in
24 hours, and Oklahoma's law is listed with the same cap. Beyond the law, a person who gets a Broadcast,
two Automation steps and a series issue in one day unsubscribes.

ADR 0134 put a sending window on marketing SMS in Outbound send, as a Deferral that is checked again
in full when it is retried.

## Decision

- **A Workspace sets a frequency cap per Channel**: at most a number of marketing messages to one
  Destination per rolling period of up to seven days. On a phone Channel it may set caps for
  countries, by the number's country, as for sending windows. The defaults are three per 24 hours on
  SMS, stricter than Florida's rule because it counts every subject, and none on email and Telegram.
  A Workspace may change or remove any cap. The app keeps no table of laws (ADR 0062); the Workspace
  answers for its choice.
- **Existing Workspaces get the SMS default too**, and their settings show it from then on: a
  Workspace already sending marketing SMS is the one the rule is for. Its live Automations may start
  deferring and its Broadcasts skipping a few Recipients; the reports say so.
- **The caps and the sending window are changed in the app by `workspace:update`**, and each change
  is audited (ADR 0022). The external API only reads the Workspace, so no Connection can widen them
  (ADR 0132).
- **"One subject" is not modelled: every marketing message on the Channel counts.** That is a
  Broadcast's, a Broadcast series' issue and an Automation's send. Transactional sends and the messages
  the person just asked for (an opt-in confirmation, a fresh link of the Preference page) neither count
  nor wait. A Test send is outside it.
- **It counts by Destination**, the number, the address or the chat, never by Contact. The law speaks
  of the number, and a Destination stays one when two Contacts of one person are merged (ADR 0128).
  What counts is the marketing messages to it in the period that are sent (accepted by the provider)
  or claimed and not yet given up; a message that failed or was skipped frees its place. An Erasure
  clears the Destination on the Outbound messages (ADR 0021), so the count restarts.
- **Transactional sends count against the Send cap and the Provider limit but not here.** Those two
  protect the platform's and the provider's capacity, which every message uses; the frequency cap
  protects the person from marketing, and a receipt is what they asked for.
- **Over the cap, a Broadcast's Recipient is Skipped and an Automation's send is deferred.** A
  Broadcast or a series issue skips that Recipient, with the reason, since one Broadcast missed is
  better than one sent two days late. An Automation's send step is a Deferral, checked again in full
  when it is retried, as ADR 0134 has it: the order of a sequence matters more than the hour.
- **A deferred Automation send waits one period at most.** If no place has freed for it by then,
  because newer Broadcasts or issues took each one, it is Skipped with the reason and the Enrollment
  moves on, as a skipped send does. There is no priority among messages, so a wait must be bounded
  instead: an Enrollment never stays active for ever, and its Automation can still be archived (the
  reason ADR 0124, ADR 0126 and ADR 0134 bound every wait).
- **No exception.** No Automation or series is outside the cap: on SMS that would be a way around the
  law. When several messages are due together, the first to be claimed counts first.
- **The window is read before the cap.** A message outside its sending window is deferred by the
  window alone; the cap is read when it is retried, so a count that has aged out overnight does not
  skip a Recipient at 8:00. Each Destination is counted on its own, so what releases together at the
  window's opening is capped per Destination, not as a whole.
- **It is exact.** The check and the claim of the Outbound message happen in one transaction that
  holds an advisory lock on the Workspace, the Channel and the Destination, taken after the Workspace
  freeze check and released when the claim commits, before the provider is called. The send path
  takes no Contact lock, so this lock joins no lock order with the Contact's (ADR 0046's fence of an
  in-flight send is unchanged). Two messages to one Destination claimed at once cannot both take the
  last place.
- **It is shown.** The reports of a Broadcast, a series and an Automation count what the cap skipped
  or deferred. It is normal, so Needs attention does not show it.

## Consequences

- Outbound send gains the cap check under an advisory lock per Destination, a Skipped reason, and a
  Deferral for Automation sends bounded by one period; the Workspace gains its caps per Channel and
  country; a migration gives every Workspace the SMS default.
- Reports count the cap's skips and deferrals.

## Considered options

- **Counting per subject**, as Florida does. The app does not know what a message is about; counting
  everything is the stricter reading.
- **Counting per Contact.** Duplicates of one person would each get the full count.
- **Deferring Broadcast Recipients too.** A Broadcast is about its moment; arriving days later it is
  noise.
- **Exempting chosen Automations.** On SMS it undoes the cap where the law needs it.
- **Deferred sends taking a freed place first.** A priority among messages, which this decision
  avoids; a bounded wait keeps the order simple.
- **No default on SMS.** A Workspace would send beyond Florida's limit before learning it exists.
