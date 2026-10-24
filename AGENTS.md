# AGENTS.md

Guidance for coding agents working in this repository.

## Outside contributors: read this first

If you work from a fork and are not a maintainer of this repository, this block overrides the
maintainer rules further down (worktrees, no branches, pushing to `main`, closing tickets).

- Fork the repository, work on a branch of your fork and open a pull request against `main`. A
  change that is more than a typo or a doc line starts from an issue a maintainer has accepted
  (label `ready-for-agent` or `ready-for-human`); name that issue in the pull request.
- A human opens the pull request, after reading the whole diff, and answers for every line of it:
  an agent does not open it on its own. The description says what changed and why, in the human's
  own words. The "AI assistance" section of the pull request template says how an AI tool was
  used: check exactly one option, or the `pr-body` check fails. A `contract-copy:` marker you add
  needs a `Contract-Copy: <file>: <reason>` line in the body.
- A commit written with an agent may end with `Co-Authored-By:` or `Assisted-by:` naming the tool;
  both are accepted. The CLA and any `Signed-off-by` are the human's, never the agent's.
- Before you push: `mise run generate` after a TypeSpec or `ent/schema` change, then
  `mise run check` and `mise run test`. The pull request title is a Conventional Commit.
- A generated file is never edited by hand (the list is under "Codegen pipeline").

## What this is

sphericon is an **open-core marketing automation platform** (with a planned SaaS offering).
Go backend + React/Vite frontend in a single repo. The data model is workspace-scoped
(multi-tenant): contacts, events, api tokens, and tracking entities all belong to a
`workspace`.

**The project is greenfield.** There are no production users and no legacy contracts to
preserve: no backward compatibility, no compat shims, deprecation paths or fallbacks.
Rewrite whatever the cleanest design needs: the TypeSpec contract, ent schema, tests and
every caller (regenerate afterwards), rather than layering the new design over the old.

## Codegen pipeline (read this first)

API contracts are **one-directional**: TypeSpec → OpenAPI → generated Go + TS. Never
hand-edit anything under `openapi/`, `gen/`, `ent/` (except `ent/schema/`),
`src/generated/` / `packages/analytics/src/generated/` / `packages/node/src/generated/`, `src/routeTree.gen.ts`, or the `*_gen.go` files in the
`internal/api/{site,external}/resources` packages, `ee/staff/resources`, `ee/staff/service_scopes_gen.go` and `internal/fixtures/catalog_gen.go`, or any `*_gen_test.go` mock, or `internal/emaildoc/assets/` (the document renderer bundle and schema listing) — regenerate instead.

Pipeline diagram, goverter and what is generated: `docs/agents/codegen.md`.

## Common commands

```sh
mise run setup          # deps, db:up, migrate, seed, worktrees:gc
mise run db:up          # ensure the one shared Postgres runs and this checkout's databases exist
mise run dev            # db:migrate (db:up first), then mise daemons start caddy — full dev stack (https://sphericon.localhost; a linked worktree has its own URL, see docs/agents/dev-environment.md)
mise run dev:down       # stop this checkout's dev stack (never the shared Postgres)
mise run dev:restart    # dev:down, then dev
mise run test           # creates the test DB, then `go test ./...` (packages in parallel)
mise run check          # tsc + oxlint + oxfmt --check + knip + golangci-lint with our analyzers (tools/analyzers, see docs/standards/coding-standards.md; incl. gofmt) + gitleaks + jactionlint + zizmor + links in the Markdown (lychee --offline)
mise run test:cover     # Go suite with coverage, fails under the total in .testcoverage.yml (nightly in CI)
mise run check:vulns    # govulncheck (nightly in CI, not part of `check`)
mise run fix            # i18n extract + oxlint --fix + tsp format + oxfmt + go fmt
mise run generate       # typespec -> openapi -> backend -> frontend -> i18n types -> format
```

Run Go tests only through `mise run test`: it sets `SPHERICON_ENV=test` and the test DB, and a bare
`go test` opens the dev database and fails.

Run a single Go test (arguments after `--` go to `go test`; the default is `./...`):

```sh
mise run test -- ./internal/api/site -run TestSiteContactsRequireAuth
```

Frontend tests: `mise run test:watch`.

Rules for writing tests (what to assert, no direct SQL, no manual cleanup) are in `docs/standards/coding-standards.md`.

## Where the rules live

The rules of the code are in `docs/`, read before you write or review:

- `docs/architecture.md`, before you touch code that queries the database, builds a `*ent.Scoped` or adds
  a package that needs the ent client (section "Workspace scoping", the two closed lists), and for the
  API surfaces, sign-in, the Enterprise Edition, async work and email.
- `docs/standards/review-rules.md`: the numbered rules every change is checked against; a check's message names
  its number.
- `docs/standards/coding-standards.md`: tests, plain-table schema, no custom CSS, theming, lint.

The rules of a directory are in its own `AGENTS.md`, loaded when you work there: `ent/schema/`
(migrations, Squawk, schemalens), `src/` (generated client, routing, i18n, lint), `e2e/`, and
`internal/` (the Go test harness).

How to run things is in `docs/agents/`: `codegen.md` (the pipeline diagram, goverter, read it
before you change TypeSpec or `ent/schema`) and `dev-environment.md` (the dev stack, Postgres, ports,
the heavy-task queue, the toolchain).

The costliest of these rules, in short:

- Handlers and domain modules take a `*ent.Scoped`, hold no `*ent.Client` and never call
  `client.Scoped(<int64>)`.
- A migration is written by `mise run db:generate name=<desc>` and is never edited once it is on `main`.
- A resource DTO comes from a goverter `Converter` method, not from a struct literal.
- The frontend calls the API through the generated client in `src/generated/`, never a hand-written `fetch`.

## Code search

`ast-grep` is installed (pinned in `.mise.toml`). For any search that depends on syntax or code
structure (calls, declarations, a method on a type, JSX props), default to
`ast-grep --lang <go|tsx|typescript> -p '<pattern>'` and adjust `--lang` per file type. Use `rg`/grep
only for plain-text searches (strings, comments, config values). Skip generated files (`gen/`, `ent/`
outside `schema/`, `src/generated/`) unless that is the point of the search.

## Maintainers only

The rest of this file is for maintainers and the agents they run: how work lands on `main`.

## Conventions

- Commit messages follow **Conventional Commits** (`feat:`, `fix:`, `chore:`, `docs:`,
  `refactor:`, `ci:` …) — release-please uses them for versioning/changelog.
  commitlint runs on every commit; the header fits 120 characters and each body line 100.
  **Never create a merge commit** (`chore: merge origin/main` and the like): history stays linear.
  Bring `main` in with `git fetch origin && git rebase origin/main`, never `git merge`.
- A new environment variable is documented in the configuration reference of the documentation site
  repository (`getsphericon/getsphericon.github.io`, served at
  https://getsphericon.com/docs/configuration); no automated check enforces it, and the README
  links to the page. All public documentation lives in that repository, not here.
- **Agent guidance lives in this file, not in personal agent memory.** The repo has several
  developers: a rule an agent should follow goes here (or in `docs/`), never into one person's
  memory.
- **Land non-trivial work from its worktree, then clean up.** A change made in a worktree reaches
  `main` with `git push origin HEAD:main` once the checks pass and the work is rebased onto a freshly fetched `origin/main` (a
  detached `HEAD`, never a branch). Then remove that worktree (`git worktree remove <path>`) right
  away, without asking. Only the worktree made for that change.
  **After the work is on `main`, update the checkout you work from:** `git fetch origin && git rebase
  origin/main` (never `git merge`), so the next piece of work starts from the merged state.
  **Landing also closes the ticket.** When the user says to land or finish ("заливай", "завершай",
  "land it"), that covers the issue the work was for: put `Closes #<n>` in the footer of the
  commit that reaches `main` (one per ticket), then confirm with `gh issue view <n> --json state`
  that it is closed after the push. If GitHub did not close it (the push lacked the keyword, or the work came through a PR), run `gh issue close <n> --comment
  "Implemented on main in <sha>."`. A ticket that stays open after its work is on `main` is
  taken as unfinished by the next agent, who then redoes it. The push prints
  `Required status check "license/cla" is expected` and still succeeds: that is how the
  repository is set up, so it is expected. Do not mention it, flag it or call it a bypass.
- **The open app stays neutral about the hosted offering.** No plans, prices, trials, payment
  providers, billing or the words that describe them in code, docs, UI strings or contracts: the
  closed services live in `getsphericon/platform`. A Staff hold carries a free-form reason code, a
  message and an optional action URL, and the core attaches no meaning to the code.
- ADR numbers are unique (`mise run check:adr`). Take the next free number when you add an ADR,
  and re-check after merging `main`: parallel branches pick the same one.
- After changing TypeSpec or `ent/schema`, run `mise run generate` and commit the generated output.
- Coding standards (tests, plain-table schema, no custom CSS, theming, lint) are in
  `docs/standards/coding-standards.md`: read it before writing code. The numbered review rules every change is
  checked against are in `docs/standards/review-rules.md`.
- A commit made with an agent ends with the `Co-Authored-By:` trailer naming it: the agent wrote and
  reasoned about the change, and the trailer says so. Outside contributors may use `Co-Authored-By:` or
  `Assisted-by:`; the person who opens the pull request answers for the code either way.

## Agent skills

### Issue tracker

Issues live in GitHub Issues (`getsphericon/sphericon`), via the `gh` CLI. See `docs/agents/issue-tracker.md`.

### Triage labels

Default five-label vocabulary (`needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`). See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: one `GLOSSARY.md` at the repo root plus `docs/adr/`. See `docs/agents/domain.md`.

### Worktrees

A trivial change is committed straight to `main` in the primary checkout. Anything else is done in a worktree, created in Claude's default `.claude/worktrees/<name>`, never in a sibling directory. Never create a branch, in the primary checkout or in a worktree. See `docs/agents/worktrees.md`.

### Advisor before /to-spec

Before running the `to-spec` skill, call the `advisor` tool first and take its advice into account. The skill is vendored, so this rule lives here rather than in the skill.

### Rebase onto main before marking ready

In `implement-spec`, before marking the draft PR ready, rebase the integration branch onto a freshly
fetched `origin/main`, push it with `--force-with-lease`, and merge the PR at once: `main` moves while the integration runs, and a
conflicting PR runs no CI. The skill is vendored, so this rule lives here rather than in the skill.

### Pull request body

The `pr` skill's template is extended by `.github/pull_request_template.md`: a body also carries its
"Issue" and "AI assistance" sections, with the option that says how an AI tool was used checked (the
maintainer confirms it when they read the diff), or the `pr-body` check fails. The skill is vendored,
so this rule lives here rather than in the skill.
