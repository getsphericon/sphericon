# Coding standards

The rules a change is reviewed against. How the repo is built, run and generated is in
`AGENTS.md`; the vocabulary is in `GLOSSARY.md`.

## Code shape

- **Getting data is separate from using it.** A value is read into a named variable on its own
  line, then used: `after := q.Count(ctx)` then `assert.Equal(t, before, after)`, never an
  assertion, argument or condition that holds a query, a call chain or a computation inside it.
  The same goes for production code: `rows, err := ...` first, then the `if` or the `return` that
  uses them. A test that fails then shows which step produced the wrong value.

## Project checks

Two kinds of check run in `mise run check`, chosen by what the rule needs (`docs/agents/review-lenses.md`).

- **Syntax only: an ast-grep rule** (`ast-grep/rules`, tests in `ast-grep/tests`, run by `mise run check:ast`).
- **Needs Go types: an analyzer of our own** (`tools/analyzers`, its own Go module). It is a
  `go/analysis` analyzer compiled into golangci-lint by `golangci-lint custom` (`.custom-gcl.yml`,
  `mise run build:lint` -> `.bin/custom-gcl`) and enabled as the linter `sphericon` in `.golangci.yml`.
  `check:be`, CI and the heavy-command wrapper all run that build, never a stock `golangci-lint`.
  The `golang.org/x/tools` and `plugin-module-register` versions in `tools/analyzers/go.mod` must be
  the ones the pinned golangci-lint uses, and `.custom-gcl.yml`'s `version` must be the one in
  `mise.lock`; `mise run check:lint-pins` fails otherwise and prints the command that fixes it.
- **Message contract.** A finding begins with the rule number and ends with how to fix it:
  `rule <n>: <what is wrong>; fix: <what to do instead>`. Build it with `rule.Message(n, what, fix)`
  (`tools/analyzers/rule`), never by hand.
- **A check is on at once.** Run it over the repository and fix every finding in the same change; there
  is no baseline of known violations and no ignore list.
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

- **A test asserts what an operation left behind** (rows, outbox events, enqueued jobs, the response),
  never what was called, how often or with which arguments: no call-verification tests. Do not assert
  how many rows a table holds (a new fixture breaks that) or a raw id: count the rows you start from
  and assert business facts (name, order, label). A test that depends on today's date does not read
  the wall clock; it injects the clock.
- **No direct SQL in tests.** Tests read and write through ent (`env.DB`) or river's own API. The
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
- **A test body does not branch.** No `if`, `switch` or `select` in a `Test…` function or its subtest
  closures (`ast-grep/rules/no-branching-in-go-tests.yml`, run by `mise run check:ast`): a branch
  chooses what to check, so a wrong run can take the path that checks nothing. State the expectation
  with `assert`/`require` on a value read beforehand; keep a table's rows as data (split a table
  whose rows need different assertions into two tests, give a row its expected value rather than a
  flag that picks one); filter and find with `lo.Filter`/`lo.Find`; wait on a channel with
  `testhelper.Receive`. Logic that has to branch (a simulated clock, a pager, a conditional skip by
  build tag) goes into a helper function or a file with a build constraint, named for what it does.
- **No `time.Sleep`, anywhere.** A sleep is a guess about how long something takes: it makes a test
  slow where it guesses high and flaky where it guesses low, and in code it hides a missing signal.
  Wait on the condition (a channel, `assert.Eventually`, a returned handle), and write the logic so
  it can be tested without waiting: a decision that needs elapsed time takes a clock, one that
  needs a run of events is a small pure type fed those events. A test that must move time inside a
  `testing/synctest` bubble calls `testhelper.Elapse(d)`, which advances the bubble's fake clock
  and returns at once. `forbidigo` rejects `time.Sleep` everywhere.
- **Tests cover logic only.** A test exercises a decision the code makes and checks what came out.
  There are no tests of how long something takes, how much memory it uses, a timeout firing on the
  wall clock, a runtime limit or a resource guard, or of the machine and the process around the
  code: they pass or fail with the load of the host and prove nothing about the code. A guard whose
  decision is real logic (when does a run of polls count as growth) is a small pure type tested on
  its inputs; the clock, the heap and the timer feeding it are not tested.
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
- **No tests of built-in behaviour.** A test exercises sphericon's own code. One whose only subject is a
  Mantine component, the standard library or another dependency (a copy button showing "Copied", a
  library's defaults or retries) is deleted, and none is added.

## Frontend

- **Never disable a lint rule** (no `off`, no `oxlint-disable`/ignore comments): fix the code;
  configure a rule's own options only when it has them.
- **No custom CSS anywhere in the repo.** Style the frontend exclusively through Mantine — components, style
  props (`p`, `c`, `w`, responsive object syntax), the color system, the theme, and the
  configured breakpoints. Do not add custom `.css`/CSS-module files, inline `style={{…}}`,
  hardcoded colors, or CSS-in-JS (styled-components/emotion). The only CSS imports are the
  library stylesheets in `src/main.tsx`.
- **Theming & responsiveness.** Light and dark color schemes are supported via
  `MantineProvider` (`defaultColorScheme="auto"`) + `ColorSchemeScript` in `index.html`;
  the UI must be responsive using Mantine primitives (responsive props,
  `visibleFrom`/`hiddenFrom`, `AppShell` breakpoints, `SimpleGrid` cols) — never custom CSS.
