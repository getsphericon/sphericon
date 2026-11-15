---
status: accepted
---

# State written by jobs lives beside the edited row, not in it

> Amends [[0119-an-edit-names-the-revision-it-was-made-on]] (the bookkeeping exemption goes).

## Context

ADR 0119 gives every edited entity a `revision` that each update moves, so an edit made on an old
revision is refused. Background work writes some columns of the same rows: an Integration's provider
limits and quota checks, a webhook endpoint's failure counters, a Workspace's feed position, setup and
abuse notices. A job's write would refuse an editor's save, so `schema.Bookkeeping` lists those columns
and the revision hook leaves a write of only them alone. The list is something the hook has to know and
every new column has to join, and an editor's row carries state no editor shows.

## Decisions

- **State that jobs write lives in a table of its own, one row per owner**: `integration_provider_state`
  (the provider's limits, enforcement and sandbox, when it refused, when its quota was checked or failed),
  `webhook_endpoint_health` (the failure counters and the last error), `workspace_standing` (the feed
  position, setup completion and abuse notices). The edited row keeps only what its editor shows.
- **The exemption goes.** `revision` moves on every write of an edited row that changes something;
  `schema.Bookkeeping`, its case in the revision hook and the generator's check of it are removed.
- **Reads that need the state load it through a one-to-one edge**: the API resources that show a webhook
  endpoint's health, the Integration's limits on its page and the send path's provider state. A list
  loads it eagerly where it shows it.

## Consequences

- Three tables, their edges and a migration that moves the columns; the writers of each column write the
  new table.

## Rejected

- **Keeping the columns with a check that jobs write only listed ones**: the hook would still know the
  list, and the row would still hold state its editor never shows.
