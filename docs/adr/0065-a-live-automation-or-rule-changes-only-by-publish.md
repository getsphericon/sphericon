---
status: accepted
---

# A live Automation or Rule changes only by Publish, and a scheduled Broadcast holds a copy of its Condition

ADR 0016 makes sending a second lock: an API token authors drafts, and only a send scope
(`automations:publish`, `broadcasts:send`, `emails:send`, plus `mcp:send` over MCP) lets it send.
Three edits got past that lock (#748). Saving Steps on an active Automation wrote a new version that
every later Enrollment began on, and its Trigger and Integration were plain columns of the
Automation. Editing an active Rule changed its Condition and Actions at once. A scheduled Broadcast
resolved its Segment at send time, so editing the Segment changed who received it. Each needed only
a write scope. The same gap hurt people on `/site`: every save of an active Automation went live, so
a sequence edited Step by Step sent its half-edited state.

The rule this ADR sets: **without a send scope, nothing can change the definition of an armed
send.** An armed send is an active Automation, an active Rule or a scheduled Broadcast. Its
definition is what decides who receives mail and what the mail says: an Automation's Trigger,
Integration and Steps; a Rule's Trigger events, Condition and Actions; a scheduled Broadcast's
Condition. Data still changes who matches: applying a Tag, editing a Contact or recording an Event
stays authoring or ingest, as before.

## Decisions

- **Every edit writes a draft; Publish makes it live.** An Automation version (ADR 0049) now holds
  the whole definition: Trigger, Integration and Steps. A Rule gets versions of the same shape:
  Trigger events, Condition and Actions. Each object points at its **live version**. An edit writes
  the **draft**, the one version above live, rewritten in place until it is published; a live
  version is never rewritten, even when no Enrollment stands on it yet. The name, the status and
  archiving stay on the object, outside the version.
- **Operations.** **Publish** (send-class, `automations:publish` and over MCP `mcp:send`) makes the
  draft live and the object active; with no draft, it makes the live version active again.
  **Deactivate** (authoring) stops new Enrollments or Firings and keeps the live version. **Revert**
  (authoring) drops the draft. There is no separate activate. The status is `active` or `inactive`;
  "draft" names only the draft version. An object never published is inactive and has no live
  version.
- **In-flight work stays on its version.** An Enrollment begins on the live version and stays on it,
  as ADR 0049 already had for the latest one. A Firing references the Rule version that fired and
  keeps no copy of the Actions.
- **Both surfaces, one model.** `/site` and `/api` read the live version and the draft side by side,
  and `Update` writes only the draft. People publish from the editor; the roles that may activate
  today may publish. A diff of draft against live is a later addition.
- **A scheduled Broadcast holds a copy of its Condition.** Schedule copies the Segment's Condition
  into the Broadcast, as an email Step copies a Template (ADR 0003). Recipients are still resolved
  at send time, from the copy, so a Contact who matches after scheduling still receives it. Unschedule
  drops the copy. The Segment reference stays as provenance only.

## Considered options

- **Refuse an edit of an armed object without the send scope.** Closes the hole with a status check,
  but leaves people editing live sequences and needs a mark on requests from MCP, whose tool list
  cannot refuse an Update that also edits drafts. Rejected for the draft model, which closes the
  hole by construction.
- **Lock a Segment while a scheduled Broadcast uses it.** Couples a shared Segment to one send.
  Rejected.
- **Copy the Condition when the Broadcast is authored.** Matches ADR 0003 more literally, but a draft
  Broadcast would stop following its Segment. Scheduling is the moment a Broadcast is armed, so the
  copy is taken there.
- **Move in-flight Enrollments to the newly published version**, as Iterable and Braze do. Rejected:
  ADR 0049 keeps an Enrollment on the version it began on, so a Step it stands on cannot move.

## Prior art

Draft-beside-live with an explicit publish is the common shape: Iterable ("Edit journey" makes a
draft version, Publish replaces live), Braze (one Canvas draft, applied or discarded), Leanplum
(Publish Changes / Discard Changes) and Apteco Orbit (draft version, Delete Draft Version reverts to
the last published one). Customer.io edits live campaigns in place, which is what sphericon did.

## Consequences

- ADR 0016's send-class list becomes Automation and Rule Publish, Email send, Broadcast schedule and
  unschedule. The send-class scope `automations:activate` is renamed `automations:publish` to match
  the operation it gates; it stays in the send group of the OAuth consent screen and brings `mcp:send`,
  and a migration moves the scope stored on existing API tokens and pending OAuth codes to the new
  name, so they keep the right. ADR 0054's Rule lifecycle (`draft`, `active`) gives way to the one above.
- A scheduled Broadcast no longer keeps its Segment in use. A Tag or Custom field named in a copied
  Condition, or in a live or draft version, still does.
- "Revert" is the verb for dropping a draft because Discard is the soft-delete mechanism (ADR 0049).
