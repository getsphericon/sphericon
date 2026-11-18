---
status: accepted
build: [1466]
---

# A Workspace's Control group receives no marketing

> Amends [[0001-send-eligibility-model]] (a layer for the Control group). The measure ADR 0152 (no
> holdout in a Broadcast) and ADR 0157 (no send-time optimisation yet) point to.

## Context

The figures a Workspace has credit Orders to the last message clicked (ADR 0064), which says which message
was near a purchase, not what the messages added: someone who would have bought anyway is credited too.
The accepted way to measure that is incrementality: a random group that receives none of the messages
being measured, and Orders compared between that group and everyone else. Marketing platforms offer it
as a control group excluded from every campaign and journey, and an agent driving an email platform is
expected to prove an optimisation with a holdout, never with last-touch credit.

A holdout inside one Broadcast measures nothing here: attribution credits clicks, and a group sent nothing
has none (ADR 0152).

## Decisions

- **A Workspace may turn on a Control group**: a share of its Contacts, 1 to 20%, 5 by default, off until
  turned on. Membership follows a hash of a salt of the Workspace and the Contact's id, so a new Contact
  falls on its side at once and no list is kept.
- **It receives no marketing message**: no Broadcast, series issue or Automation send, on any Channel.
  Transactional messages and those the person asked for (a Confirmation, a Preference page link) still
  go.
- **It is a layer of Send-eligibility**, checked in Outbound send like the others: the message is Skipped
  with the reason `control_group`. An Enrollment goes on past a Skipped send, as it does past any, so its
  Branches and waits still run. ADR 0149's count leaves the group out. No Broadcast may reach it anyway:
  an exception would spoil the measure.
- **Its report compares the groups, not credited revenue**: over the period since the group was set,
  revenue per Contact, the share of Contacts with an Order and Orders per Contact, for the Control group
  and for the rest, with the difference and its confidence interval. Unsubscribes and complaints are not
  compared, since the Control group cannot have them. Credited revenue stays a figure of its own.
- **A period lasts until the group is reset.** A reset draws a new salt and starts a new period; the report
  keeps the results of the earlier ones. The console advises at least 30 days and warns before an earlier
  reset.

## Consequences

- Send-eligibility gains the layer and Skipped the reason `control_group`; the Workspace settings gain the
  switch, the share and the reset; a report compares the groups per period.
- Glossary: **Control group**; Send-eligibility.

## Rejected

- **A holdout per Broadcast or Automation**: the groups change with every send, and a group sent nothing
  has no clicks to be credited, so the comparison needs Orders across all sends anyway.
- **Letting a Broadcast reach the group**: every exception moves the group's Orders toward the rest and
  makes the difference meaningless.
