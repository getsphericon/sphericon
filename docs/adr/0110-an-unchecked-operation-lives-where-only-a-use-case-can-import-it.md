---
status: accepted
---

# An unchecked operation lives where only a use case can import it

> Refines [[0102-the-core-is-layered-and-an-operation-across-modules-is-a-use-case]]: it fenced a
> domain module's unchecked removal by naming it in the `forbid_symbol` list of every other
> component. This removes the cause of most of those names first, and fences the rest by where they
> live.

## Context

ADR 0102 lets a domain module export a write whose check lives in a use case (deleting a Tag, whose
Live use only orchestration can read), and keeps every other caller away by name:

- `x-unchecked-removals` in `easement.yml` names 78 functions. YAML merges maps, not sequences, so
  each variant of the list (`x-core-forbids`, `x-raw-holder-forbids`, `x-runner-forbids`,
  `x-user-deletion-forbids`) is a full copy kept by hand: 624 of the file's 1563 lines.
- Of the 32 rules that take a list, 17 are domain components and 2 are the Enterprise use cases;
  10 are drivers. A rule that drivers call only use cases would remove one copy of four.
- Each new unchecked write is one more name in the definitions and in four copies.

The names fall into three groups, and most of them exist for a reason other than a check above
their owner:

- **The Workspace erase: 40 functions.** Each `Erase*In` is one line,
  `s.X().Delete().Where(x.IDIn(ids...))` (or `Purge`), in a `workspaceerase.go` of its owner. Only
  `workspacedeletion` calls them, and the Enterprise Edition's three reach it through
  `EditionTables` in the wiring. They are exported only because "only the owner writes a table"
  also covers rows of a Workspace that is being erased, which no rule of an owner governs any more.
- **Contact Erasure: 17 steps and `contacts.DeleteIn`.** Several repeat what the foreign keys do
  when the Contact row goes: the `automation_runs`, `rule_firings`, `bindings` and `confirmations`
  rows cascade, and `broadcast_recipients`, `unsubscribes` and `suppressions` set null. The others
  (anonymizing delivery records, hashing numbers, cancelling queued work, the Events, Visitors,
  short links and Captured messages) harm a live Contact when called alone.
- **Checks that bypass: 23 functions.** Some checks need modules their owner may not call (a Tag's
  Live use reads orchestration). Others need nothing their owner lacks, or guard state that sits
  in the wrong module:
  - `sendingdomainverification` reaches one module: `sendingdomains` already does the DKIM and
    DMARC lookups, and the use case only orders them before `RecordDKIM` and `RecordDMARC`.
  - The Second factor's state is columns of `users`. `users` exports four writes without checks,
    and the Second factor check lives in `secondfactorcheck`. All of it is in identity.
  - A refresh token belongs to a Connection (`oauth_refresh_tokens.api_token_id` cascades from
    `api_tokens`), yet `oauthclients` writes it. That is why `connectionissuance` composes the two
    modules.
  - Deleting a Workspace checks only its typed name, which is the Workspace's own rule.
  - `memberships.LeaveDoomedWorkspaceIn` is called by the domain module `users`, because User
    deletion lives there. That alone costs the list its fifth copy.

Two facts about easement 0.5.1 bear on the fix:

- `forbid_symbol` accepts a member after its type (`ent.Client.Query`). A probe forbade
  `tags.Mod.Purge` and failed its call. ADR 0102's "easement matches names, not methods" is wrong.
- A component with `allow_importers` refuses every other importer of the paths it owns. A probe
  failed a driver's import of a removal subpackage and passed a use case's. A `surfaces` entry of
  the domain component does the same, but a component with surfaces and no `public` closes every
  other path, so each domain component would also list its other paths in `public`.

## Decisions

- **The cause goes before the fence.** A write is left unchecked only when its check needs a module
  its owner may not call: one in a higher context, or several modules at once. It still checks its
  own input (ADR 0107); what it leaves to the use case is the rule across modules. Otherwise:
  - the check moves into the owner, and the write becomes an ordinary checked operation of it;
  - when the state is owned by a module other than the one whose rules govern it, the state moves.
- **The Workspace erase removes rows directly.** `workspacedeletion` deletes each scoped entity's rows
  through `*ent.Scoped`, in the order its table keeps (foreign keys first, Owner Memberships last for
  the owner feed, the Tracking domain where its hook records the released host). The test that the
  order covers every entity of `ent.ScopedEntities` stays.
  - `entrywrite` names the erase as the one writer of any Workspace-owned table besides its owner.
    The rows of a Workspace past its window have no rule of an owner left to keep.
  - The 40 `Erase*In` functions and `EditionTables` go. The Enterprise Edition's tables are scoped
    entities like the others.
  - Cascading foreign keys are not used for this: the erase runs in batches, one transaction each,
    and the hooks that record released hosts and advance the owner feed do not run on a cascade.
- **The checks that need nothing above the owner move into it:**
  - `sendingdomains` verifies and rechecks a Sending domain itself. `RecordDKIM` and `RecordDMARC`
    become unexported, and the use case `sendingdomainverification` goes.
  - A Second factor is owned by a domain module `secondfactors` in identity, with a table of its own
    for the factor and the Recovery codes. The Second factor check is that module's. It sits above
    `users`, since it reads the User's session epoch and password hash and bumps the epoch. The
    readers of the factor (login, sign-in, notices, re-encryption) read it there, and the
    Second factor deadline of a Workspace that requires one, with its reminders and its part of
    admission, moves from `users` into it, so `users` reads nothing of the factor. A Second factor
    reset also removes Passkeys, so it stays a use case.
  - `apitokens` owns a Connection's refresh tokens. It exchanges a code and refreshes a Connection
    itself, and `oauthclients` claims a code only after its own check of the client, the redirect
    URI and the PKCE verifier. `connectionissuance` goes. A code or a refresh token is looked up
    before any Workspace scope exists, so `apitokens` joins the closed list of raw-client holders.
  - `workspaces` checks the typed name when it deletes a Workspace. Restoring one needs a Membership
    that holds the delete Permission, so it stays a use case. (Amended by #1218: that use case is
    the Workspace view, which answers the restore with the Owner's view of the Workspace, not the
    deletion cycle; see [[0111-every-entry-point-is-a-driver-that-calls-one-operation-of-the-core]].)
  - User deletion becomes a use case. `LeaveDoomedWorkspaceIn` becomes an unchecked operation like
    the others, and `x-user-deletion-forbids` goes.
  - The Erasure steps that the foreign keys already do go: `automations.EraseRuns`,
    `rules.EraseFirings`, `broadcasts.DetachRecipients`, `unsubscribes.DetachContact`, and the parts
    of `bindings.EraseSubject`, `confirmations.EraseContact` and `suppressions.EraseContact` that
    touch the Contact's own rows.
- **What stays unchecked lives in `<module>unchecked`**, a subpackage of its owner
  (`internal/domain/contacts/tags/tagsunchecked`). The name keeps the importing use case free of
  aliases: one use case imports a dozen of them.
  - One easement component owns the core's subpackages, with
    `allow_importers: [usecase, usecase-raw]`, and one the Enterprise Edition's, with
    `allow_importers: [ee-usecase, ee-usecase-raw]`. An Enterprise use case reaches none of the
    core's, as today. No `forbid_symbol` list names an unchecked operation.
  - The four Live use writes keep the lock type of ADR 0108 in their signatures.
  - The subpackage may import its owner. The owner does not import it.
  - `entrywrite` takes a subpackage as its owner's, by its prefix.
  - Today that is about 21 functions: the four Live use writes (`tags.DeleteIn`,
    `integrations.DeleteIn`, `sendingdomains.DeleteIn`, `contacts.UpdateCustomFieldIn`), the
    Erasure steps the foreign keys cannot do with `contacts.DeleteIn`, `workspaces.RestoreIn` and
    `TombstoneIn`, `workspaces.CreateIn` with `memberships.AddCreatorIn`,
    `memberships.LeaveDoomedWorkspaceIn`, and the removal of the factor and of the Passkeys in a
    Second factor reset.
- **What only the wiring may use keeps a short `forbid_symbol`**, written once with its own
  `because`: `ent.Client`, the ent hooks and guards, the upgrade boundaries and river's workers.
  Its reason is who builds the program, not a check.
- **A fence for any other caller has the same form.** A subpackage, and one component whose
  `allow_importers` names who may import it. A list of names in `forbid_symbol` is not used to keep
  callers away from a module's function.

## Considered options

- **Keep the names and drop the copies.** A top-level `forbid_symbol` applies to every checked file,
  and the schema has no exemption, so the use cases would be forbidden too. Per-component lists stay
  copies.
- **A type per module whose methods are the unchecked writes**, forbidden by one name per module.
  Members can be forbidden, so it works. But it is still a list of about 30 names in every rule
  except the use cases'.
- **`surfaces` on each domain component.** It refuses the same imports, but each domain component
  would then list all its other paths in `public`, a list to keep for every new module.
- **Drivers call only use cases** (ADR 0111). It removes the runner's copy and leaves the 19 rules of
  the domain components and the Enterprise use cases as they are.
- **Cascading foreign keys for the Workspace erase.** One delete of a large Workspace would be one
  transaction, and the hooks would not run.

## Consequences

- Each move above is a change of its own, with its tests moving to the module or use case that now
  holds the check (`internal/AGENTS.md`, "Use cases").
- `easement.yml` loses the 78 names, their four copies and their comments. The component of the
  unchecked subpackages and the short wiring list replace them.
- `docs/architecture.md`, review rule 5 and the comments that say "easement forbids every other
  package to call X" change with the code. So do the closed list of raw-client holders (`apitokens`
  joins it) and the rule that the Audit table's rows are removed only through `auditentries`, which
  gains the Workspace erase.
- A `forbid_symbol` list that fences one module's functions (the readers of a Removal check, if it
  lands) becomes a subpackage with its own component.

## Amendment: one component per context, and two removals the list missed

- **The unchecked subpackages are fenced per context.** One component for the core's subpackages
  sat in `layers` above every context, so an identity subpackage could import an instance module
  and a contacts one an orchestration module, though its owner may import neither. Each context
  has a component of its own instead, `domain-<context>-unchecked`
  (`internal/domain/<context>/**/*unchecked`), with `allow_importers: [usecase, usecase-raw]`,
  placed in `layers` right above its context: a subpackage imports its owner and what its owner's
  context may, and no more. The bus's Erasure step (`eventsunchecked`) has `infra-unchecked` above
  the infrastructure, and the Enterprise Edition `ee-domain-<context>-unchecked`. A context with no
  subpackage today keeps an optional component, so its first one is fenced. Only
  `domain-identity-unchecked` may use `ent.Client`: its subpackages join their use case's raw
  transaction.
- **The list of what stays unchecked also holds** `usersunchecked.DeleteIn`, the User row of User
  deletion, whose check needs `memberships` and `secondfactors` beside `users`. (An authorization
  code was removed by `oauthclientsunchecked.DeleteCodesIn` while it was no scoped row; it is
  Workspace-owned now, ADR 0017, so the erase removes it as it removes every Workspace-owned row.)
