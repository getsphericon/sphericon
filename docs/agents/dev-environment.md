# Dev environment

Running the stack, Postgres, ports, the heavy-task queue and the toolchain.

Every task runs natively with the toolchain pinned in `.mise.toml` / `mise.lock` (Go and Node
are pinned to the exact versions the `Dockerfile` builds with, Go also to the `go` line of
`go.mod`: bump them together); CI installs the same toolchain with `jdx/mise-action` and runs
the same tasks.
There are no dev containers and no Makefile: tasks live in `mise/tasks.toml` (included
from `.mise.toml`); `mise tasks` lists them. The dev stack (Postgres, Mailpit, backend, Vite,
Caddy) is a set of **mise daemons** (`[daemons]` in `.mise.toml`, experimental). Tasks that need
Postgres depend on `db:up` (below) instead of a `db` daemon of their own.

**One Postgres for every checkout.** The `db` daemon (mise `postgres` preset, fixed port 15432)
is declared in `.mise.toml` but only ever runs in the **primary checkout** (the first
`git worktree list` entry: a stable project root that `mise daemons prune` never sees as
deleted); `mise run db:up` starts it from any checkout and nothing else starts a `db` daemon.
A postmaster per worktree takes one of macOS's 32 SysV shared-memory segments
(`kern.sysv.shmmni`), so ~30 worktrees exhaust them and Postgres fails with `shmget ... No space
left on device`. `db:up` is idempotent: a server already answering at `PGHOST:PGPORT` is a no-op
(so two checkouts calling it at once are safe: at most one start, the other waits for the port),
otherwise it runs `mise -C <primary> daemons start db`, then creates this checkout's databases
if missing under an advisory lock. Every task that needs Postgres (`test`, `test:e2e`, `db:*`)
depends on `db:up`, and so does `dev`; `dev:down` leaves the server running
(`mise -C <primary> daemons stop db` stops it for everyone). The primary checkout must be on a
commit that has this setup.

`PGHOST`, `PGPORT`, `PGUSER`, `PGDATABASE` and `DATABASE_URL` are set in `[env]`, so they are
identical in every checkout without a daemon (a `db` daemon exports its own offset port and
database for a linked worktree, which `[env]` overrides). The database is `sphericon_<dir>`: the
checkout's directory name lowercased to `[a-z0-9_]`, cut to 31 characters plus a hash of the path
when it exceeds 40, so with the `_test`, `_e2e` and `_diff` twins (`TEST_DB_URL`, `E2E_DB_URL`,
`DIFF_DB_URL`; `DB_NAMES` lists all four) every name stays under Postgres's 63-byte identifier
limit and nothing is truncated or collides. Never set `DATABASE_URL` in `.env`: explicit values
override it. `SPHERICON_PG_HOST` / `SPHERICON_PG_PORT` point every checkout at another server (an
external Postgres): one that answers is used as is and never started. CI runs the same path: its
single checkout is its own primary.

Postgres refuses to run as root, so the `db` daemon does not start in a container that runs as root
and `db:up` fails with that error. Run mise as a regular user there, or start a Postgres 18 of your
own on `PGHOST:PGPORT` (as another user, or via `SPHERICON_PG_HOST` / `SPHERICON_PG_PORT`):
`db:up` uses a server that already answers.

Never hand-name a scratch database. `mise run worktrees:rm <path>` removes a worktree with its
databases (the four of `DB_NAMES`, the test workers' `_w<i>` clones and `_tpl_<hash>` templates) and
its daemons, from the primary checkout. `mise run worktrees:gc` (also run by `setup` and
`worktrees:prune`, and by `db:up` of the primary checkout in the background at most once a day, never on CI or with
`SPHERICON_PG_HOST` set; its log is `<git dir>/sphericon-gc.log`) is the net under it for a worktree
deleted any other way; `--apply` to act, a dry run otherwise. It drops the shared server's databases that
no live worktree's `DB_NAMES` covers and runs `mise daemons prune --yes` for dead state; it
refuses to drop anything when it cannot enumerate or read a live worktree, including one on a
commit from before `DB_NAMES` (rebase it onto main), and never touches the primary's daemon.
The `testdb_tpl_*` databases are pgtestdb's template and per-test databases (the
`testhelper.RealDB` family, below), owned by a test that may be running in any checkout: pgtestdb
drops each instance when its test ends, so gc drops only the leftovers of a killed run: an
instance after a day (a plain `DROP DATABASE`, refused while anything is connected) and a template
after a week (pgtestdb rebuilds it).
`mise run worktrees:prune -- --apply` first removes the worktrees whose branch is already in
`origin/main`. A worktree from before this setup still runs its own `db` daemon: it retires with
its next rebase onto main plus `mise daemons stop`. Several tasks in one command need `:::` between
them (`mise run check:fe ::: check:deps`); otherwise the extra names become arguments.

Test packages run in parallel, each process on a database of its own, through
`dbcleaner.WithTemplate` (wired in `internal/testhelper`): it leases `<test db>_w<i>` (an advisory
lock on the shared test database, released when the process ends), cloned from
`<test db>_tpl_<hash>`, a schema-only template built once per hash of the schema and the bus. The
fixtures are loaded into the worker at the start of every process, because they use relative
dates. `DATABASE_URL` is rewritten to that database for the process, so a test that loads the
config reaches it too. `worktrees:gc` keeps these with their checkout. The end-to-end suite keeps
one database and `-p 1`.

**Heavy tasks queue machine-wide.** The CPU-heavy commands (`go test`, `golangci-lint`, `tsc`,
`knip`, vitest, ent/ogen/goverter generation) run through [`heavy`](https://github.com/mokevnin/heavy)
(`"github:mokevnin/heavy"` in `.mise.toml`; also `brew install mokevnin/tap/heavy`), which holds one
of a few slots for the command's duration. A slot is an exclusive `flock` on a file in
`${TMPDIR}/heavy/default` (`HEAVY_DIR`, `HEAVY_POOL`), shared by every checkout and agent on the
machine, and the kernel frees it when the holder dies however it dies. A run that finds all slots
busy prints `heavy: all N slots of pool 'default' are busy, waiting for one` and waits, so ten agents
starting `mise run check` together take turns instead of starving each other. `HEAVY_SLOTS` sets how
many run at once (default: a quarter of the cores, at least one); `heavy --status` shows who holds
them. In CI a heavy command runs at once with no queue, and so does one started inside a held slot
(`HEAVY_HELD`). Wrap a new heavy command the same way: `heavy -- <command>`.

CI runs the Go tests with `-shuffle=on` (the seed is printed; reproduce an order with
`mise run test -- -shuffle=<seed>`), so a test must not depend on another's side effects. Locally
`mise run test` leaves shuffling off because any `-shuffle` flag disables Go's test result cache: an
unchanged package is `(cached)` on the next run (`-count=1` forces a run).

**Nightly, not per PR.** `.github/workflows/nightly.yml` runs `mise run test:cover` and
`mise run check:vulns` (govulncheck) once a night and on demand (`workflow_dispatch`). The coverage
floor in `.testcoverage.yml` is raised when coverage rises, never lowered. A new advisory can fail
`check:vulns` with no change on our side, so it is deliberately not part of `check`: put a check
there when its result does not depend on the diff under review.

**Tool versions.** `.mise.toml` declares most tools as `latest`; `mise.lock` pins them, and
Dependabot has no mise ecosystem, so nothing moves the lock but a commit. `.github/workflows/tools.yml`
runs weekly and keeps one `ready-for-agent` issue ("Update the mise tools") listing the tools behind
their newest release, closing it when the lock is current. Bump with `mise upgrade <tool>` (it
rewrites `mise.lock`), run `mise run check`, commit as `build(deps):`.

## Dev environment

`mise run dev` starts the mise daemons (`.mise.toml`) behind Caddy with HTTPS at
**https://sphericon.localhost** (run `caddy trust` once). The external API is also exposed at
**https://api.sphericon.localhost** — Caddy rewrites `/*` → `/api/*` to the same backend, so
the subdomain root mirrors the binary's `/api` path (RudderStack-style edge; the binary
stays path-based). In prod the ingress in front of the binary does the same rewrite for
`api.sphericon.app`. Daemons: `db`, `dns` (CoreDNS for Sending domain and Join domain verification: `dev/Corefile`, zones in `dev/dns/`, UDP and TCP on :15353, offset in a linked worktree; the backend reads it through `SPHERICON_DNS_ADDR`), `mailpit` (web UI at :8025, behind Caddy at **https://mailpit.localhost** (`MAIL_HOST`); SMTP :1025),
`backend` (real Go server under air on `:3300`, hot reload), `frontend` (Vite on `:5173`),
`demo` (Vite on `:5373`: the Brightpath demo storefront, `packages/storefront`, a customer site on its
own origin `brightpath.localhost` that sends events through `@sphericon/analytics` and the
`@sphericon/node` SDK; it is not part of the release image), `caddy` (:443). Inspect with `mise daemons ls|logs|status`. Migrations run via goose
(`mise run db:migrate`); the backend does not self-migrate. Dev defaults (JWT secret, dev
`SPHERICON_ENCRYPTION_KEY`, SMTP) live in the `[env]` table of `.mise.toml`; personal overrides go in
the gitignored `.env` (read by the app) or `.mise.local.toml`.

**Ports and origins per checkout.** Every daemon declares `port = { auto = true, base = <port> }`:
the primary checkout gets exactly the ports above (caddy 443, vite 5173, backend 3300, mailpit
8025/1025, metrics 9090); a linked worktree gets one slot offset (`base + N`, the same `N` for all
of its daemons), so many stacks run at once. The slot comes from a hash of the checkout's path,
not a registry: two worktrees can land on the same one (two of 14 here did), and then the second
stack fails to start with an error naming the daemon and the other project. Pin the later one
in its `.mise.local.toml` (`[daemons.backend]` ... `port = 3601`, the same for `frontend`,
`mailpit` and `caddy`; SMTP, metrics and Caddy's admin/HTTP ports follow from them). mise
resolves the ports at config load, so
`[env]` templates them (`PORT`, `SPHERICON_SMTP_PORT`, `SPHERICON_METRICS_ADDR`, `SPHERICON_APP_URL`, `APP_HOST`, `API_HOST`), and
the Caddyfile (`{$BACKEND_PORT}`, `{$APP_HOST}`, ...), `vite.config.ts` and the `ready_cmd`s read the
exported `<NAME>_PORT` (`ready_port` cannot be templated). Ports with no daemon of their own (SMTP,
metrics) are the base plus that same offset; Caddy's admin API (`CADDY_ADMIN`, 2019 + offset, what
`caddy trust --address "$CADDY_ADMIN"` reads) and HTTP redirect listener (`CADDY_HTTP_PORT`: 80 in
the primary checkout, 4000 + offset elsewhere) are per instance too. A linked worktree's origin is
`https://<dir-name>.sphericon.localhost:<caddy port>` (API: `https://api.<dir-name>.sphericon.localhost:<caddy port>`):
browser cookies are shared across the ports of one host, so a per-worktree host keeps the session
cookies apart; `*.localhost` resolves to loopback and Caddy's internal CA signs each name.
`mise env | grep SPHERICON_APP_URL` prints it. Caddy stays the edge instead of pitchfork's own proxy
(`proxy = "<label>"`, stable per-worktree hostnames) because that proxy needs per-machine setup
(`pitchfork proxy setup`: DNS, trust store, ports), would still leave the primary on Caddy's :443,
and Caddy already does the `/api` rewrite and the path routing in one file.
