---
status: accepted
---

# An Enrollment explains its path, and a Path preview shows one without effects

## Context

An author cannot see why a Contact took the path it took, nor where a Contact would go through a version
before it is published. An Enrollment records the path it took at each Branch, Split and Wait for event
(ADR 0124), each send ends in an Outbound message with its outcome (ADR 0015), and the Enrollment's
progress is recorded as System events, but nothing records why a Branch chose its path, and nothing shows
a path before an Enrollment walks it. A Test inbox (ADR 0058) lets a Workspace send for real without
delivering, and rejected a mode that would put a case of its own in the send path.

Marketing platforms let an author pick a person and preview their path through a journey, with timings
and rendered messages, sending nothing and calling no webhook. An agent driving an email platform is
expected to show what a send will do before it asks a person to approve it.

## Decisions

### Enrollment timeline

- **An Enrollment's timeline is read from what is recorded**: each Step reached and when, the path
  taken at a Branch, a Split or a Wait for event, each wait's due moment, each send's outcome with its
  reason (a Skip names why), an HTTP step's status, and an exit or goal.
- **A Branch records why it chose**: when it decides, the outcome of each path's Condition, true, false
  or unknown, is kept with the path taken. Only the outcomes, never the values tested: those are the
  Contact's data, and an Enrollment is kept for reports after its Contact's data may be gone.

### Path preview

- **A Path preview computes, for one Contact and one version (draft or live), the path it would take
  now**, writing nothing: no Enrollment, no Event, no message, no call. Branches are evaluated against the
  Contact's data now; a Split takes the path its hash gives, which is the one a real Enrollment would
  take; a wait shows its due moment counted from now.
- **What cannot be known now is chosen by the author.** Each Wait for event is previewed as `arrived` or
  `timed_out`, as the author picks. An HTTP step is not called: the preview names its host, and
  `http.<name>` is empty unless the author gives a sample response.
- **A send Step shows its message and its verdict**: the rendered content, and whether it would be sent
  or Skipped and why (Send-eligibility, the Frequency cap, the Sending window), without claiming any send.
- **It is a read**: `automations:read`, on `/api`, `/site` and MCP, so an agent can show it to a person
  before asking for a Publish. Contact data and a sample response in it are marked untrusted (ADR 0132).
- **There is no test mode of an Automation.** Previewing before Publish is the Path preview; a run end to
  end without delivery is an Automation sending through a Test inbox (ADR 0058). A mode that switches
  sending off would be a case of its own in the send path.

## Consequences

- A Branch's recorded path gains the outcomes of its paths' Conditions.
- The Enrollment resource gains its timeline; a Path preview operation is added.
- Glossary: **Enrollment timeline**, **Path preview**.

## Rejected

- **Recording the tested values**: personal data outliving the Contact's in an Enrollment kept for
  reports.
- **A dry-run Enrollment that walks the version over time**: rows, jobs and waits that exist only to be
  thrown away, and a case in every Step for "do not really do it".
