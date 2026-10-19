# Coding standards

The rules a change is reviewed against. How the repo is built, run and generated is in
`AGENTS.md`; the vocabulary is in `GLOSSARY.md`.

## Database

- **Plain tables only: no database functions, triggers, stored procedures or views.** The schema is
  tables, columns, constraints and indexes. Logic that must run on every write (a change log, a
  counter, a derived row) lives in Go: an ent hook (like `internal/workspacefeed` or
  `internal/trackinghostlog`) or the domain module, inside the same transaction.

## Go tests

- **A test asserts what an operation left behind** (rows, outbox events, enqueued jobs, the response),
  never what was called, how often or with which arguments: no call-verification tests. Do not assert
  how many rows a table holds (a new fixture breaks that) or a raw id: count the rows you start from
  and assert business facts (slug, order, label). A test that depends on today's date does not read
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
  other test rides the transaction `pgcleaner` hands out, which rolls back. A `t.Cleanup` that
  deletes rows, a purge or a reset at the top of a test means a test is committing outside its
  transaction: fix that test.
- **No `time.Sleep`, anywhere.** A sleep is a guess about how long something takes: it makes a test
  slow where it guesses high and flaky where it guesses low, and in code it hides a missing signal.
  Wait on the condition (a channel, `assert.Eventually`, a returned handle), and write the logic so
  it can be tested without waiting: a decision that needs elapsed time takes a clock, one that
  needs a run of events is a small pure type fed those events. A test that must move time inside a
  `testing/synctest` bubble calls `testhelper.Elapse(d)`, which advances the bubble's fake clock
  and returns at once. `forbidigo` rejects `time.Sleep` everywhere.
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
