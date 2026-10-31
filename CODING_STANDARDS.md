# Coding standards

How the repo does things: mechanics, tools and conventions. The rules a change is reviewed against
are in `docs/standards/review-rules.md`: read it too, and do not restate a rule here. How the repo is
built, run and generated is in `AGENTS.md`; the vocabulary is in `GLOSSARY.md`.

## Code shape

- **Getting data is separate from using it.** A value is read into a named variable on its own
  line, then used: `after := q.Count(ctx)` then `assert.Equal(t, before, after)`, never an
  assertion, argument or condition that holds a query, a call chain or a computation inside it.
  The same goes for production code: `rows, err := ...` first, then the `if` or the `return` that
  uses them. A test that fails then shows which step produced the wrong value.
- **No error is dropped silently, and `_ =` is not a way to drop one** (`errcheck` runs with
  `check-blank`). Handle it; for the step after the work is done (closing a connection, shutting
  a server down, removing a temporary file) call `internal/cleanup`, which logs it. The functions
  whose error nobody can act on (terminal output, a response body that was only read, the rollback
  of a committed transaction) are listed with their reason under `errcheck` in `.golangci.yml`.
- **An error is the standard library's: `fmt.Errorf("<operation>: %w", err)`, `errors.Is`,
  `errors.AsType` and `errors.Join`; no error library.** The wrap prefix names the operation, which is
  all a log line needs: a stack taken where the error is wrapped is not where it arose, so it would
  need a wrap at every source (ent, pgx, river) to help. A caller that branches takes a sentinel
  (it only matches a condition) or a typed error (it needs the data), such as `validation.Error`
  for a refused input of a domain module.
- **Narrowing an integer to the contract's `int32` goes through `narrow.Int32`**
  (`internal/narrow`), which caps at the bounds instead of wrapping.
- **A change to a field in an Audit entry's diff is `events.Change(from, to)` or
  `events.Became(to)`**, keyed by the ent field constant (`membership.FieldRole`), not a literal.
- **A linter is off only when it is harmful here, and the reason is in `.golangci.yml`.** An
  exclusion there is scoped to the files it concerns and says why; there is no `//nolint`.

## Project checks

Two kinds of check run in `mise run check`, chosen by what the rule needs (`docs/standards/review-rules.md`).

- **Syntax only: an ast-grep rule** (`ast-grep/rules`, tests in `ast-grep/tests`, run by `mise run check:ast`).
- **Needs Go types: an analyzer of our own** (`tools/analyzers`, its own Go module). It is a
  `go/analysis` analyzer compiled into golangci-lint by `golangci-lint custom` (`.custom-gcl.yml`,
  `mise run build:lint` -> `.bin/custom-gcl`) and enabled as the linter `sphericon` in `.golangci.yml`.
  `check:be`, CI and the heavy-command wrapper all run that build, never a stock `golangci-lint`.
  The `golang.org/x/tools` and `plugin-module-register` versions in `tools/analyzers/go.mod` must be
  the ones the pinned golangci-lint uses (`build:lint` builds the golangci-lint of `mise.lock`);
  `mise run check:lint-pins` fails otherwise and prints the command that fixes it.
- **Message contract.** A finding begins with the rule number and ends with how to fix it:
  `rule <n>: <what is wrong>; fix: <what to do instead>`. Build it with `rule.Message(n, what, fix)`
  (`tools/analyzers/rule`), never by hand.
- **A check is on at once.** Run it over the repository and fix every finding in the same change; there
  is no baseline of known violations and no ignore list.
- **Rule 1, `handmapper`.** A function that takes an ent entity and returns a type of a `gen/*` package
  must map through a goverter converter (it calls a `...Converter`/`...ConverterImpl` method, directly or
  through a package function that does) or be registered in the converter (`goverter:extend`, or named in
  a `goverter:map ... | helper` line). Add the mapping to the surface's `Converter` instead; test files are not checked.
  The input direction is checked too: a struct literal of a module input (a struct named `...Input` of a package
  under `internal/`) in the `/site` or `/api` handler package is a finding. Add a method to the surface's `Inputs`
  (`resources/inputs.go`) and call it; a field the request does not carry (the actor) is a `goverter:ignore` with
  its reason, assigned by the handler after the call, and a value of the request outside the mapped part (a
  batch's flag on each of its Events) is a `goverter:context` argument.
- **Rule 6, `check:copies` (`cmd/contractcopy`, in `check`).** It reads the generated `openapi/*.openapi.json`
  and fails on a hand-written copy of its values: a Go list, `case` list or TypeScript array or union of
  literals that restates an enum (all of its values, or three or more that are a quarter of a larger one),
  a Go or TypeScript `Max*`/`Min*`/`Default*` constant equal to a declared length, range or default, and a
  fixture scope (`fixtures/*.yml`, which cannot import Go) the contract does not declare. A value that only
  looks like a contract value (an OAuth limit, a secret floor) carries `// contract-copy: <reason>` on its
  line or the line above; a marker without a reason is still a finding, and there is no baseline file. Typed
  Go `const` blocks are domain vocabulary and are not read. Tests use the generated constants
  (`externalapi.ApiTokenScope…`) or the generated zod (`zSiteMembershipRole.options`), not string lists.
- **Rule 6, `one-way-to-record-audit` (ast-grep).** An explicit Audit entry (one no audited write of the
  scoped client produces) is recorded only by `events.RecordAudit(ctx, pub, entry)` in the transaction of
  the change: a call of `RecordAudit` on any other receiver is a finding. Package `events` calls it
  unqualified for the generated wrappers; `ent/` and `gen/` are not read.
- **Rule 7, a state is moved by a conditional write.** A machine column (`status`, `tracking_state`) is written only by its transition table: the ast-grep rule `no-status-write-outside-lifecycle`. A state kept as a moment that a background job moves on its own (`verified_at` and `verification_lost_at` of a Sending domain, `disabled_at` of a Webhook endpoint, `suspended_at` of a Workspace, `verified_at` of a Join domain) is written only by an update with a `.Where(...)` on the state it leaves (analyzer `statemoment`); a lost race surfaces as `ent.IsNotFound` and the caller handles it. To cover a new moment, add its builder method to `moments` in `tools/analyzers/statemoment`.
- **Rule 10, `testwrites`.** A call to an ent `Create` or `CreateBulk` builder (the raw client, a transaction's client
  or a scoped wrapper) in a `_test.go` file, `e2e/` included (the lint run sets the `e2e` tag), needs
  `// test writes: <why the write is the operation under test>` on the line above its statement or trailing on
  its line; an empty reason is still a finding, there is no baseline file, and `*_gen_test.go` mocks are skipped.
  What counts as the operation under test is rule 10; any other test takes its rows from `fixtures/`
  (add a row, `internal/AGENTS.md`). Test-support code outside `_test.go` files
  (`internal/testhelper`) is the fixture tooling and is not read.
- **Rule 5, `entrywrite`.** An entry point does not write through ent: a call that starts a mutation
  (`Create`, `CreateBulk`, `Update`, `UpdateOne`, `UpdateOneID`, `Delete`, `DeleteOne`, `DeleteOneID`) or an
  upsert (`OnConflict`, `OnConflictColumns`, `OnConflictColumnsWhere`) on a type of the `ent` package (the raw
  client, a transaction, the scoped client and its wrappers) is a finding; test files are not read. The handlers
  of the API surfaces (`surfaces`: `internal/api/site`, `external`, `collect` and `staff`) do not read through ent either:
  a `Get`, `GetX`, a `Query…` method or the scoped client's `Workspace` there is a finding, and the read is a
  function of the module that owns the entity; the other entry points may read. Nor does a handler there open a
  transaction (a `Within…` method of the events bus): the module it calls does. The closed list of entry points
  is `entryPoints` in `tools/analyzers/entrywrite`: the site, external, collect and staff handlers, `internal/mcpserver`, `internal/jobs` and `internal/server`. `ee/staff` holds
  the handlers of the Staff console's Enterprise sections and their domain module in one package, so only the methods of its handler types
  (`Handlers`, `ServiceHandlers`, `SecurityHandler`, `ServiceSecurityHandler`, `Surface`, `ServiceSurface`) are
  checked and `*Module` is the domain module; a new handler type there is added to `mixedPackages`. The write
  goes into the domain module that owns the entity (the raw-client packages of `docs/architecture.md`), which the entry
  point calls. The helpers the analyzers share about the generated package are in `tools/analyzers/entx`.
- **Rule 15, `no-common-refusal-in-handlers` (ast-grep).** A handler in `internal/api/site`,
  `internal/api/external` or `internal/api/staff` names no `validation.Error`, `validation.RefusalError`,
  `validation.FieldsError` or `validation.Message` and nothing of `inuse`, and never spells `"workspace not found"`: it returns
  the module's error, and `refusalProblem` in `internal/server` answers a module's sentinel refusal
  by its kind (`validation.NotFound` 404, `validation.Refuse` 422, `validation.Conflict` 409,
  `validation.Forbidden` 403 `permission_required`, `validation.Unauthorized` 401,
  `validation.Denied` 403, `validation.Unverified` 403 `email_verification_required`,
  `validation.LinkInvalid` 400 (a mailed link that does not stand), `validation.Incomplete` 428,
  `validation.UnknownReference` 400 (a reference that names nothing, such as a pending sign-up that
  is gone or a cursor never handed out), the message as `detail` and, for a sentinel that names a
  field (`validation.RefuseField`, `validation.ConflictField`), under its field), a `*validation.Error` with 422 and its field, a
  `*validation.FieldsError` (a form a module refused) with 422, every field and its `Detail` as
  `detail`, an `*inuse.Error` with 409 and `usedBy`, an `*accounts.DeletionRefusedError` with 422
  and the Workspaces the User solely owns, content `emailrender` cannot render with 422, and
  `accounts.ErrWorkspaceNotFound` (an unknown Workspace or one the caller is not a member of) with
  a 404. A sentinel one context answers differently from another is two sentinels of the module,
  one per context, never a mapping in the handler.
- **Rule 15, `no-hand-written-problem-in-site` and `no-hand-written-problem-in-external` (ast-grep).**
  A handler in `internal/api/site` or `internal/api/external` calls neither `problem` nor
  `problemWithErrors`, builds no `siteapi.*ProblemDetails` or `externalapi.*ProblemDetails` literal
  and names no operation's error response (`siteapi.Site…NotFound`, `externalapi.…NotFound` and their
  kin): the module that decides a refusal returns its sentinel. The rules' `ignores` are the files
  that keep an answer no refusal kind gives (the 402 of a missing Enterprise license, the 410 of an
  expired confirmation link), with each surface's `problem.go`, which holds the helpers they call.
  A single answer kept by hand in a covered file carries
  `// ast-grep-ignore: no-hand-written-problem-in-site` under a comment that says why. A batch that
  reports each item inside a 200 words a failed item with `validation.Shown`.
- **Rule 15, `access-is-a-permission-go` and `access-is-a-permission-tsx` (ast-grep).** No code compares
  a Role: in Go, `==`/`!=` or a `case` with `RoleOwner`/`RoleAdmin`/`RoleMember` of any package
  (`membership`, `entmembership`, `invitation`), and `RoleEQ`/`RoleNEQ`/`RoleIn`/`RoleNotIn` of a
  Membership or an Invitation in a query; in TypeScript, a `role` or `.role`
  compared with `'owner'`/`'admin'`/`'member'`. Access asks `accounts.Allows`/`Require`/`HoldersOf`
  or `can(workspace, …)`; a Role fact that is not access calls `internal/accounts/roles`. Only that
  package and the Permission table (`internal/accounts/permissions.go`) are not read, with tests and
  generated code. Setting a Role is not a comparison.
- **Adding an analyzer.** (1) Create `tools/analyzers/<name>/` with `Analyzer` (see `noprint`) and report
  through `rule.Message`. (2) Write `<name>_test.go` with `analysistest.Run` and a test data package per
  case under `testdata/src/` (a `bad` package whose violating lines carry `// want` with the message
  regexp, and a `clean` one that must report nothing); watch it fail, then make it pass. (3) Add the
  analyzer to `BuildAnalyzers` in `tools/analyzers/plugin.go`. (4) `mise run check:be` rebuilds the linter,
  runs it over the repository and the analyzers' tests; fix everything it finds. `noprint` (rule 0) is the
  sample that proves the pipeline.

## Database

- **Raw SQL needs its reason (rule 4, analyzer `rawsql`).** A statement run as text (`ExecContext`,
  `QueryContext`, `QueryRowContext`, `Exec`, `Query`, `QueryRow` on `database/sql`) or a raw fragment
  in an ent query (`sql.P`, `ExprFunc`, `Expr`, `ExprP`, `Raw`) carries `// raw sql: <why ent cannot
  express it>` on the line above it or its statement, or trailing. The advisory lock and the Postgres
  version are written once, in `internal/postgres` (`LockTx`, `ServerMajor`); call those.
- **Plain tables only: no database functions, triggers, stored procedures or views.** The schema is
  tables, columns, constraints and indexes. Logic that must run on every write (a change log, a
  counter, a derived row) lives in Go: an ent hook (like `internal/workspacefeed` or
  `internal/trackinghostlog`) or the domain module, inside the same transaction.

## Go tests

- **Tests use the ent client or river's own API for data (rule 4)**: `env.DB`. The
  domain-event outbox is modelled in ent too (`OutboxMessage`, `OutboxOffset`: watermill owns the
  tables, `entsql.Skip` keeps them out of migrations, only the client is generated), so
  `env.Outbox*`, `env.OutboxCount` and the cursor helpers sit on the generated client.
  `forbidigo` in `.golangci.yml` rejects `*sql.DB` `Exec`/`Query`/`QueryRow` in `_test.go` files.
  Inject faults with ent hooks/interceptors, never DDL or triggers.
- **Tests never clean up by hand, and never reset state at their start.** A test that needs a real
  commit (the outbox router, concurrent workers, a whole app) takes a database of its own from
  `testhelper.RealDB`, `RealDBURL` or `EmptyDBURL` (pgtestdb), dropped when the test ends; every
  other test rides the transaction `dbcleaner` hands out, which rolls back. A `t.Cleanup` that
  deletes rows, a purge or a reset at the top of a test means a test is committing outside its
  transaction: fix that test.
- **No `time.Sleep`, anywhere.** A sleep is a guess about how long something takes: it makes a test
  slow where it guesses high and flaky where it guesses low, and in code it hides a missing signal.
  Wait on the condition (a channel, `assert.Eventually`, a returned handle), and write the logic so
  it can be tested without waiting: a decision that needs elapsed time takes a clock, one that
  needs a run of events is a small pure type fed those events. A test that must move time inside a
  `testing/synctest` bubble calls `testhelper.Elapse(d)`, which advances the bubble's fake clock
  and returns at once. `forbidigo` rejects `time.Sleep` everywhere.
- **Pure logic gets property tests.** A function or small type that decides something from its
  inputs alone (a delay, a normalization, a counter of polls) is tested with `pgregory.net/rapid`:
  state the property that must hold for every input (bounds, idempotence, monotonicity, agreement
  with a simple model) and let the library search for a counterexample. Examples next to them stay
  for the cases that document a business rule. A counterexample rapid saves under `testdata/rapid`
  is committed as a regression.
- **Code that parses outside input has a fuzz target.** A parser or validator that takes bytes from
  an operator, an agent or the network (a document, a Preset file, a key) gets a `Fuzz…` test next
  to it that states what must hold for any input (no panic, a result or an error and never both, a
  refusal of a known kind), seeded from the real files. `mise run test:fuzz` runs the targets (the
  nightly workflow for minutes each); a crash is saved under `testdata/fuzz` and committed.
- **A package that starts goroutines checks that its tests stop them.** It has a `main_test.go`
  with `goleak.VerifyTestMain`: a test that leaves a server, worker or subscriber running fails the
  package. A new package that starts goroutines gets the same file.
- **Tools for tests without branching (rule 14).** Filter and find with `lo.Filter`/`lo.Find`; wait
  on a channel with `testhelper.Receive`; logic that has to branch (a simulated clock, a pager, a
  conditional skip by build tag) goes into a helper function or a file with a build constraint,
  named for what it does. Split a table whose rows need different assertions into two tests.
- **A guard with real logic is tested as a pure type (rule 12).** When does a run of polls count as
  growth is a decision, tested on its inputs; the clock, the heap and the timer feeding it are not
  tested.

## Frontend

- **Never disable a lint rule** (no `off`, no `oxlint-disable`/ignore comments): fix the code;
  configure a rule's own options only when it has them. The one exception is the workflow
  builder's `<style>`, below.
- **No custom CSS anywhere in the repo.** Style the frontend exclusively through Mantine — components, style
  props (`p`, `c`, `w`, responsive object syntax), the color system, the theme, and the
  configured breakpoints. Do not add custom `.css`/CSS-module files, inline `style={{…}}`,
  hardcoded colors, or CSS-in-JS (styled-components/emotion). The only CSS imports are the
  library stylesheets in `src/main.tsx`, and the workflow builder's, whose global resets must not
  outlive the builder: `AutomationBuilder.tsx` renders it as a `<style>` element, the one file
  `react/forbid-elements` lets one in (an override in `oxlint.config.ts`).
- **A pull request runs the logic tests; the UI tests run nightly (rule 12).** `*.test.ts` files
  are logic and run on every pull request (`mise run test:frontend:logic`). `*.test.tsx` files
  render pages in Chromium: they are written only for a critical path and run, sharded, with the
  coverage floor, in the nightly workflow. `mise run test:frontend` runs both, locally.
- **Theming & responsiveness.** Light and dark color schemes are supported via
  `MantineProvider` (`defaultColorScheme="auto"`) + `ColorSchemeScript` in `index.html`;
  the UI must be responsive using Mantine primitives (responsive props,
  `visibleFrom`/`hiddenFrom`, `AppShell` breakpoints, `SimpleGrid` cols) — never custom CSS.
