---
status: accepted
---

# An edit names the revision it was made on

Two people who open the same resource and both save lose the first save without being told. No
write compares what the client loaded with what is stored:

- Every update is a `PUT` whose fields are all optional (absent keeps, `null` clears, ADR 0002), and
  the console sends every field its form holds (`src/resources/resourceFormSchema.ts`). The second
  save rewrites every field of the first.
- An email template update is one blind `UpdateOneID` (`emailtemplates.Module.Update`). A Contact
  update takes the Contact's row lock, which orders two writers and detects neither, and its form
  always sends `tags`, the complete tag set, so a stale form drops a Tag a Rule added meanwhile.
  Segment and Broadcast updates are conditional on a state (not archived, still a draft), not on
  content.
- The compare-and-swap of an Automation and a Rule (ADR 0046, `ent/scoped_versioned.go`) compares
  `updated_at` with the server's own read in the same transaction. It fences the write against a
  concurrent Revert or Deactivate; the client sends nothing to compare, so two Saves under the
  Workspace lock run one after the other and the second wins.
- `updated_at` cannot be the client's token: the contract's `Timestamp` is written with seconds
  (`time.RFC3339`), so two saves in one second read the same.
- `docs/standards/ui.md` states the gap: changes made in another tab or by another person are not
  merged.

The same writes serve the external API and MCP (ADR 0016, ADR 0111), so a script and an agent
overwrite a person, and a person overwrites them, the same way.

## Decision

- **A resource a person edits carries a `revision`**: an integer column of its row, starting at 1,
  that every write of the row moves by one, except a write of its bookkeeping alone (below). It is declared once, by a schema mixin (`RevisionMixin`), and moved by one ent hook that
  every application client carries (`entclient.Revision`, on `UpdateOne` and `Update` of any type
  that has the column), so no writer can forget it. A schema hook would have moved ent's runtime
  into a package every binary must import for its defaults to run. The hook does not see an
  upsert's `ON CONFLICT DO UPDATE`, which ent runs as a create, nor a client built without
  `entclient`: a writer of either kind moves the revision itself. A versioned
  definition (Automation, Rule) keeps it on the definition row, because its name is written there
  and a draft rewritten in place keeps its number and id. It replaces the `updated_at` comparison of
  `scoped_versioned.go`: one token per row.
- **Every write of the row moves it, whoever writes**: the console, the external API, MCP, Identify
  and imports (`contacts` enrichment), a Rule's Tag action, an Automation's Tag step, the Tag
  endpoints of both surfaces, `PUT /broadcasts/{id}/segment`. Each of these changes what an editor of the row shows, so a save made
  over it is a real conflict. A write that changes nothing writes nothing: it moves neither
  `revision` nor `updated_at`.
- **Bookkeeping does not move it.** A column the system keeps for itself, which no editor shows or
  sends, is marked `schema.Bookkeeping`: a webhook endpoint's failure record (its consecutive
  failures, when they began, the last error), a provider's reading of an Integration, a notice's
  moment. An update that writes such columns alone leaves the revision, so a job that writes them on
  every delivery or every check does not refuse the save of a person who has the resource open; an
  update that also writes any other column moves it, and one that writes no column (a touch of
  `updated_at`, as a Contact's Tag change does) moves it too. A column a person also writes, or that
  an editor shows and could save over, is not bookkeeping: a delivery worker that turns an endpoint
  off moves the revision, since the editor's form sends whether it is on. The hook reads the
  generated set (`ent.Bookkeeping`), so a writer cannot opt out at a call site.
- **The resource returns it, and its update takes it back.** The resource's `revision` is the one
  of the state it shows; an update's body names the `revision` the edit was made on, and the write
  is conditional on it (`UPDATE … WHERE id = ? AND revision = ?`, or the comparison inside the
  transaction that already reads the row). The answer to a successful update carries the new
  `revision`.
- **The `revision` is required on every surface**: `/site`, `/api` and so MCP. An update without
  one is the lost update itself, whoever sends it, and there is no client to keep: a script or an
  agent reads the resource before it writes, as the console does. A contract check fails an update
  operation of a resource with a `revision` whose body does not require it, and the editor
  session's operations below likewise.
- **A stale revision is a 409 with the code `revision_changed`** (`ProblemCode`, ADR 0103). It is
  "lost a race" in the sense of ADR 0109, and the client branches on the code, not on the status.
  The body carries nothing more: the client reads the resource again.
- **The editor session's operations name it too.** Publish, Revert, Send now and Schedule
  act on stored content the person is looking at, so each takes the `revision` of what the editor
  shows and is refused the same way. A state move outside an editor (archive or discard from a
  list, a worker's move) stays a compare-and-swap on the state alone (ADR 0046).
- **The editor's base revision moves only with what the editor shows**: when an acknowledgement is
  folded in, or when the session replaces the content (Revert, a reload). A query refetch that
  brings a newer `revision` does not move it, or a background refetch would make a stale form look
  fresh, with one exception: when what the editor would open from the refetched resource is exactly
  what it last saved, the write that moved the revision changed nothing it shows or sends (another
  form of the same row, such as a Workspace's General and Time zone sections, or a column no form
  sends), so the base moves to it and nothing is said. This holds only for an editor whose content
  is made from the resource alone (a schema form, not a canvas whose graph is its own), only while
  no write of its own is pending, and never clears a notice already given.
- **The console tells the person and merges nothing.** On `revision_changed`, or when a refetch
  shows a newer `revision` than the base, the editor keeps the person's content as it is, still
  unsaved, and says that the resource changed since it was opened, with two actions: load the
  saved version (dropping the unsaved edits, asked in the leave dialog's words) or save these edits
  over it (the same submission with the newer `revision`).

## Consequences

- A migration adds `revision bigint NOT NULL` to each table of the list below: an add-only change
  (ADR 0033). It is added with `DEFAULT 1`, which fills the rows already there, and the default is
  dropped in the same migration: that a resource starts at revision 1 is the application's rule,
  which ent sets on every insert (a Go default, `DefaultFunc`, not copied into the column), so a
  row written past ent, a fixture included, names its own.
- Every update test of these resources sends a `revision`, and each resource gets a test that a
  stale one is refused and leaves the row as it was. Every caller of an update, the e2e steps and
  the MCP tools' descriptions included, reads the resource first.
- The editor session gains the base revision and the "changed since it was opened" state;
  `docs/standards/ui.md` says how the console handles it instead of "not merged".
- Machine writers that rewrite unchanged values (Identify's enrichment does an update even when no
  field changes) must skip the write, or an active Contact refuses every console save.
- The resources: Automation, Rule, Email template, Contact, Segment, Broadcast; then every other
  resource the console edits with a form (Brand kit, Custom field, Webhook, Integration, Workspace
  settings).

## Considered options

- **`ETag` and `If-Match`, a 412 on a mismatch.** The HTTP way, and the one to keep in mind for the
  external API. It was not chosen because the token has to reach every client of the write: MCP
  returns a tool caller only the body of an answer (`internal/mcpserver`), so an agent would never
  see the `ETag`; the console's resource editor reads only the body of an update's answer; and a
  response header becomes a wrapper type of every get and update the generated server returns. A
  field of the body reaches all of them, a list included.
- **`updated_at` as the token, sent with microseconds.** It needs no column, but it is a time, not a
  counter: it is set by the process clock, it moves on writes that are not edits, and nothing stops
  two writes from reading the same value.
- **`revision` optional on `/api` and MCP**, an update without it written unconditionally, for
  scripts that write blind. It keeps the lost update for exactly the writers a person cannot see,
  and the project has no external client whose calls it would keep working.
- **Merging concurrent edits.** Field-level merge breaks on the fields that are replaced whole
  (`steps`, `condition`, `tags`, a body), which are the ones people edit. A refusal the person
  resolves is honest; a merge that is right most of the time is not.
