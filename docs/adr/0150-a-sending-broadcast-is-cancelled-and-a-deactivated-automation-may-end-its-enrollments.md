---
status: accepted
---

# A sending Broadcast is cancelled, and a deactivated Automation may end its Enrollments

> Amends [[0016-mcp-surface-projection-of-external-api]] (the send-class list),
> [[0046-lifecycles-are-stateless-tables-with-a-compare-and-swap-write]] (the Broadcast
> machine) and [[0065-a-live-automation-or-rule-changes-only-by-publish]] (Deactivate).

## Context

Nothing the Workspace controls stops a send in progress. The Broadcast machine leaves `sending` only
by `finalize` or `fail`, and Unschedule works only from `scheduled`; a Broadcast found wrong while it
goes out keeps going, unless a Staff hold or a suspension pauses the whole Workspace (ADR 0007).
Deactivating an Automation stops new Enrollments, and every Enrollment already in it walks on and
sends its remaining steps, so a wrong step keeps reaching people for as long as its waits last.

A stop has to be as easy to reach as the mistake was to make: a person or an agent who finds a bad
send must be able to end it without first being trusted to send.

## Decisions

### Cancel a sending Broadcast

- **Cancel** moves a Broadcast from `sending` to **`cancelled`**, a final state. It is a state move
  outside an editor, a compare-and-swap on the state alone (ADR 0046, ADR 0119). Unschedule stays the
  way back from `scheduled` to draft.
- **It stops what has not started.** No Recipient without an Outbound message gets one after the
  move; a message already claimed or handed to the provider goes out, since a provider cannot be
  called back. Cancel answers how many Recipients were left unsent.
- **Its figures stay derived.** The Recipients of a cancelled Broadcast that have no Outbound message
  are counted as `cancelled`; it has no `pending`. No row is written for them.
- **A hold does not change it**: a held Broadcast is `sending`, and Cancel ends it like any other.
- **Cancel only narrows reach, so it is authoring, not send-class**: `broadcasts:write`, and over MCP
  it is listed without `mcp:send`. A Connection that cannot send can still stop a send.
- There is **no Pause and Resume**. Resume would arm a send again, a second state would meet holds
  and deferrals, and the need behind it, fixing a mistake and reaching the rest, is a new Broadcast to
  those the cancelled one did not reach: the retry of a Broadcast's unsent Recipients, decided apart.

### Deactivate chooses what happens to the Enrollments in flight

- **Deactivate takes a required choice: `finish` or `end`.** `finish` is what Deactivate does today:
  new Enrollments stop and every active one walks on. `end` also moves every active Enrollment to
  `exited` at that moment, so no waiting step of theirs sends; a message already claimed goes out.
  There is no default: the caller says which.
- **The console says how many Enrollments are active** when it asks, and Deactivate answers how many
  it ended.
- **Both stay authoring** (`automations:write`), as Deactivate is: they only narrow reach.
- An Automation has **no Pause** of its own. Resuming in-flight Enrollments later would release sends
  whose moment has passed, which a Publish of a fixed version does not do.

## Consequences

- The Broadcast status gains `cancelled` in the contract, the console's list and filter, and the report;
  the report's figures gain `cancelled`.
- `Broadcasts.cancel` joins `/api`, `/site` and MCP; Deactivate's body gains `inFlight: finish | end`
  on `/api` and `/site`.
- Glossary: Broadcast's lifecycle, a **Cancel** entry, and the Automation entry's "deactivate ≠ halt"
  now reads as a choice.
