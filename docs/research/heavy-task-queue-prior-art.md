# Prior art for `mise/scripts/heavy.sh`: a machine-wide counting semaphore

Researched 2026-10-24 against primary sources only (each tool's own repository: README, source,
releases, GitHub API metadata). Every factual claim carries its source URL. Anything not
confirmed on a primary page fetched during this research is marked **unverified**. Results of
commands run here are labelled **local run** (macOS 27 arm64, 10 cores, mise 2026.10.17, node
26.12.1, uv 0.13.4, bash 3.2, Homebrew `flock`); they are evidence, not a citation. Every
candidate was installed in a scratch `MISE_DATA_DIR` under `/private/tmp/claude-501/s`, never in
the repo. Repo facts come from `mise/scripts/heavy.sh` (49 lines), the 19 `heavy.sh` command
lines in `mise/tasks.toml`, `AGENTS.md` and `docs/agents/dev-environment.md` (lines 62-70).
GitHub star counts and dates are as of today's API read.

This note extends section 6 of `replacing-the-mise-shell-scripts.md`, which looked only at
GNU `sem`, `flock`, mise `jobs` and pitchfork.

**Short answer.** A search of GitHub finds no mature, widely used cross-process counting
semaphore that fits. What exists is (a) a cluster of brand-new (September-October 2026), one-
person tools written for exactly our situation ("several coding agents on one machine"), all with
0-1 stars, and (b) old generic tools that are mutexes, daemons or per-invocation limits. The best
ready-made fit is `deblasis/incoda`: it passed every local test, installs with one `github:` line,
but needs a one-time per-machine slot configuration, has no CI skip of its own and is six weeks
old. Every large project that solves the same problem in public (JAX, XLA) does it with the
same `flock`-per-slot loop as `heavy.sh`. Ranked: **1. rewrite in TypeScript with loopback-port
slots** (primitive verified here, no install), **2. adopt `incoda`**, **3. keep `heavy.sh`**.

---

## 1. Requirements (read from `heavy.sh`)

| #   | Requirement                                                                                                         |
| --- | ------------------------------------------------------------------------------------------------------------------- |
| R1  | N slots shared by every process and checkout on the machine; N from an env var, default a quarter of the cores      |
| R2  | A waiter prints one message (`heavy: all N slots are busy`) and waits                                               |
| R3  | A nested call (a heavy task started inside a held slot) runs at once; `SPHERICON_HEAVY_HELD` today                  |
| R4  | The slot is released when the holder dies, including `SIGKILL`; a daemon the command leaves behind must not keep it |
| R5  | macOS and Linux                                                                                                     |
| R6  | CI (`CI` set) and machines without the primitive skip the queue                                                     |
| R7  | No new runtime (node 26, go, uv/python already present); installable through mise; repo scripts are TypeScript      |

---

## 2. Candidates

Verdict columns: yes / no / partial, from the README plus the local run in section 3.

| Candidate                     | Source                                                            | Licence                             | Stars | Latest release / push                    | mise line (verified)                                                     | R1 N shared                                                                 | R2 message           | R3 nested                 | R4 SIGKILL                                         | R5 mac+linux           | R6 CI skip                    | R7 no new runtime                    |
| ----------------------------- | ----------------------------------------------------------------- | ----------------------------------- | ----- | ---------------------------------------- | ------------------------------------------------------------------------ | --------------------------------------------------------------------------- | -------------------- | ------------------------- | -------------------------------------------------- | ---------------------- | ----------------------------- | ------------------------------------ |
| **incoda**                    | https://github.com/deblasis/incoda                                | MIT                                 | 1     | v0.7.3, 2026-10-08 (created 2026-09-01)  | `"github:deblasis/incoda" = "0.7.3"` installs                            | partial: slots live in a state-dir config, not an env var                   | yes                  | yes (`INCODA_HELD`)       | yes (kernel `flock`)                               | yes (release has both) | no, wrapper line needed       | yes (Go binary)                      |
| **loadmaster**                | https://github.com/ktanaka101/loadmaster                          | MIT or Apache-2.0 (API: Apache-2.0) | 1     | v0.17.0, 2026-10-18 (created 2026-10-10) | `"github:ktanaka101/loadmaster" = "0.17.0"` installs                     | partial: capacity in machine config, default cores-2, adapts to outside CPU | yes                  | yes (`LOADMASTER_HELD`)   | partial: slot kept while the task lives, by design | yes                    | no                            | yes (Rust binary)                    |
| **agent-task-queue** (`tq`)   | https://github.com/block/agent-task-queue                         | Apache-2.0                          | 73    | v0.4.1, 2026-04-22; pushed 2026-10-22    | `"pipx:agent-task-queue" = "0.4.1"` installs (`tq`)                      | partial: `--queue-capacity` flag on every call                              | yes                  | **no, deadlocks**         | yes (zombie lock cleared)                          | yes                    | no                            | partial: Python via uv               |
| **heavy** (mhdsilva)          | https://github.com/mhdsilva/heavy                                 | MIT                                 | 0     | no release; pushed 2026-10-03            | none: no release assets (git clone only)                                 | yes (`HEAVY_SLOTS`)                                                         | yes                  | yes (`HEAVY_HELD`)        | yes                                                | yes                    | partial: "no flock, no queue" | no: another shell script             |
| **lock** (DylanVann)          | https://github.com/DylanVann/lock                                 | MIT                                 | 1     | no release; pushed 2026-10-05            | none (Rust source build)                                                 | yes (`lock slots N`, state file)                                            | **unverified**       | yes (`LOCK_TASK`)         | **unverified**                                     | macOS app, Rust CLI    | no                            | no: needs cargo                      |
| **nlock**                     | https://github.com/erietz/nlock                                   | GPL-3.0                             | 0     | v0.1.5, 2026-10-23                       | **unverified** (release has per-OS tarballs)                             | yes (`--slots`)                                                             | yes                  | **unverified**            | yes                                                | yes                    | no                            | no: needs a daemon (`brew services`) |
| **pueue**                     | https://github.com/Nukesor/pueue                                  | Apache-2.0                          | 6388  | v4.0.4, 2026-03-02                       | `"github:Nukesor/pueue" = "4.0.4"` installs only one of `pueue`/`pueued` | groups with `parallel`                                                      | queue, not a message | no                        | daemon owns jobs                                   | yes                    | no                            | no: daemon                           |
| GNU `sem` (GNU parallel)      | https://www.gnu.org/software/parallel/sem.html                    | GPL-3.0+                            | n/a   | n/a                                      | not in `mise registry`                                                   | yes (`--id`)                                                                | no                   | no                        | **unverified**                                     | needs Perl + install   | no                            | no                                   |
| task-spooler `ts`             | https://github.com/justanhduc/task-spooler                        | GPL-2.0                             | 444   | pushed 2024-03-06                        | not in `mise registry`                                                   | yes (`ts -S N`)                                                             | queue                | no                        | **unverified**                                     | brew/apt               | no                            | no: per-socket server                |
| GNU make jobserver            | https://github.com/mirror/make (doc/make.texi, "POSIX Jobserver") | GPL-3.0+                            | n/a   | n/a                                      | n/a                                                                      | token pipe/FIFO                                                             | no                   | via inherited `MAKEFLAGS` | **no, token lost**                                 | yes                    | n/a                           | only tools that speak it             |
| `lockrun`, `lockfile-progs`   | https://github.com/pushcx/lockrun                                 | none stated                         | 64    | pushed 2016-01-04                        | n/a                                                                      | no: one instance only                                                       | no                   | no                        | file lock                                          | yes                    | no                            | n/a                                  |
| `semaphoric` (shell, `flock`) | https://github.com/timbunce/semaphoric                            | Apache-2.0                          | 7     | pushed 2015-03-18                        | n/a                                                                      | yes                                                                         | **unverified**       | no                        | `flock`                                            | yes                    | no                            | no: shell                            |

---

## 3. Findings per candidate

### 3.1 incoda (best ready-made fit)

What it is: "A keyed, machine-local job lane for builds, tests and AI-agent fleets", one static Go
binary, no dependencies. Source: https://github.com/deblasis/incoda (README).

- **Slots and sharing (R1).** A queue has a slot count (default 1); `incoda config builds --slots 2`
  sets it; every caller of a key contends for the same lane from any folder or worktree
  ("nothing is ever resolved from the working directory"); state is per user under
  `~/Library/Application Support/incoda` or `$XDG_STATE_HOME/incoda`, overridable by `INCODA_DIR`.
  Source: README sections "Slots" and "Machine-local and per-user, never per-directory".
- **Release is a lock held by the kernel (R4).** "Every participant holds an OS-level exclusive lock
  on its ticket file for its whole lifetime: `flock` on Unix... The kernel releases it when the
  process ends for any reason, including `SIGKILL`." Same README. The README's "known limits" say
  a hard kill of `incoda` on Unix can orphan the child.
- **Nested (R3).** "`run` exports `INCODA_HELD` to its child... A nested `incoda run` on one of those
  keys runs at once inside the parent's lane", after checking the ticket is alive and held by an
  ancestor. Same README.
- **mise.** `mise ls-remote github:deblasis/incoda` lists 0.7.1 to 0.7.3; the release has one asset
  per OS/arch (`incoda_darwin_arm64`, ...) plus `SHA256SUMS`
  (https://github.com/deblasis/incoda/releases/tag/v0.7.3); mise installed it as `incoda`
  (5.7 MB). **Local run.**

**Local run** (2-slot pool `builds`, `INCODA_DIR` in scratch):

- A and B started, C printed `queue "builds" busy (pool; 2 slot(s), 2 ahead of you), waited 0s,
  limit 30m0s` and the two holders' pids and commands, then waited. R2 met.
- `kill -9` of A's `incoda` wrapper: C acquired the slot about 2 s later and ran (A's `sleep` child
  stayed alive as an orphan, as the README warns). R4 met.
- Nested with 1 slot: `incoda run --queue builds -- sh -c 'incoda run --queue builds -- echo inner'`
  printed `already held by a parent incoda; running inside its lane` and `inner`. R3 met.
  The child's env had `INCODA_HELD=builds=<ticket>`.
- Startup cost: 0.09 s wall for `incoda run ... -- true`.
- **Gaps found.** (1) A project queue made with `incoda config heavy --slots 2` is **refused**
  (exit 120, "not linked to any pool") until linked to one of the four machine-wide pools
  (`builds`, `computer-use`, `tests`, `vm`); the pool `builds` itself works as a queue after
  `incoda config builds --slots 2`. The slot count is state in the user's state directory shared
  by every project using the pool, not an env var, so "a quarter of the cores" needs a setup step
  (`incoda config builds --slots $((cores/4))`). (2) Every run prints `incoda: acquired queue ...`
  on stderr. (3) The default wait limit is 30 minutes (exit 121); `--wait` changes it. (4) No CI
  switch of its own.
- **CI skip, local run.** A mise `[env]` template makes the prefix empty under `CI`:
  `HEAVY = "{% if get_env(name='CI', default='') != '' %}{% else %}incoda run --queue builds --{% endif %}"`
  and a task line `$HEAVY sh -c "echo ran"` printed `HEAVY=[incoda run --queue builds --]` locally
  and `HEAVY=[]` with `CI=true`.
- **Risk.** Created 2026-09-01, 1 star, one maintainer, v0.x: "maintained" is not yet shown by
  history. Also `incoda` pools are machine-wide and per user, so another tool using `builds` shares
  our slots.

### 3.2 loadmaster

"Weighted job admission for parallel AI agents": one machine-wide queue with a per-task weight and a
machine capacity, coordinated through a SQLite file, no daemon.
Source: https://github.com/ktanaka101/loadmaster (README "At a glance").

- Tasks are named in a committed `.loadmaster.toml` (`cmd` array, arguments after `--` appended);
  capacity is in `~/.config/loadmaster/config.toml` (`LOADMASTER_CONFIG` moves it), default "number
  of CPU cores minus 2"; tasks run in the directory of `.loadmaster.toml`. A worktree whose branch
  lacks the file fails. README "Configuration".
- A generic task gives a plain semaphore: `[tasks.heavy] weight = 1 cmd = ["sh","-c","exec \"$@\"","sh"]`,
  called as `loadmaster run heavy -- go test ./...` (**local run**, works).
- Nested: "The inner call sees `LOADMASTER_HELD` and runs directly in that job's slot" (README "Nested
  runs"). **Local run**, capacity 1: `outer` then `inner`, no deadlock.
- **Local run**, capacity 2, three jobs: C printed `is waiting (weight 1, 1 job ahead) for 1 core (1 of
  1 in use; 9 of 10 cores are busy beyond what running jobs got)`: the capacity shrank to 1 because
  the machine was busy outside the queue (`adapt_to_outside_cpu`, README "Machine"). So "N slots"
  is not exact unless `adapt_to_outside_cpu = false`.
- `kill -9` of the wrapper: C did **not** start (the task child was still running); C started 5.2 s
  later when the child ended. This is the README's design ("A task keeps its slot while it runs,
  even if `loadmaster` itself is killed"), not a leak, but R4 as written (release on holder death)
  is only met once the command itself dies.
- mise installs it (4.8 MB). Startup 0.07 s. **Local run.**
- Fit: a weighted-admission design is more than we need and it moves the slot count to a per-
  machine config file; `cd ent && ...` and `cd tools/analyzers && ...` task lines would run in the
  wrong directory unless wrapped in `sh -c`.

### 3.3 agent-task-queue (`tq`, Block)

"Local task queuing for AI agents", Python, SQLite `queue.db`, FIFO, capacity 1 by default, an MCP
server plus a `tq` CLI. Source: https://github.com/block/agent-task-queue (README "How It Works").

- Capacities come from `--queue-capacity=SCOPE=N` flags that "are process-local and are not
  persisted"; every caller must pass the same flags (README). Data directory defaults to
  `/tmp/agent-task-queue` (`TASK_QUEUE_DATA_DIR`).
- mise: `pipx:agent-task-queue` installs in 1.3 s and exposes `tq` and `agent-task-queue`
  (**local run**); uv supplies the Python, which the repo already has.
- **Local run**, `--queue-capacity=heavy=2`, three jobs: C printed `Position #1 in queue. Waiting...`
  and, after `kill -9` of A's `tq`, `WARNING: Cleared zombie lock (ID: 1)` and ran. R2, R4 met (by a
  liveness check of the recorded pid, not the kernel; exact mechanism **unverified**).
- **Local run**, nested with the default capacity: the inner `tq run` waited and ended with
  `TIMEOUT after 5s`: a deadlock. A wrapper would need an env test in the task line.
- `tq run` has a default timeout of 1200 s applied to the command (`-t`); a `go test -race ./...`
  could hit it. Startup 0.29 s (Python).
- Fit: wrong shape (agent MCP server first, exclusive queue by default), no nested rule, 20-minute
  default kill.

### 3.4 heavy (mhdsilva), lock (DylanVann), nlock (erietz)

- **heavy** is a 341-line bash script that is `heavy.sh` almost exactly: `HEAVY_SLOTS` default CPUs/4
  (min 1), `HEAVY_HELD` for nesting, a "queued; slots held by" message, `nice`/`ionice`, an optional
  systemd cgroup cap, and, relevant to us, a **`perl` fallback for `flock`** ("perl's flock (ships
  with macOS)"), so macOS needs no `brew install flock`. Source:
  https://github.com/mhdsilva/heavy (README "Platforms", script lines 90-95 and 164-170). No release,
  so mise cannot install it, and it is bash, which our rule excludes. **Local run** (git clone in
  scratch, `HEAVY_SLOTS=2`): C queued with the holders listed, took the slot 2 s after `kill -9` of A,
  nested call with 1 slot ran at once.
- **lock** (Rust): shared/exclusive/light tasks, FIFO, a state file, a macOS app, `LOCK_TASK` for
  nested calls, and a machine-wide GNU make jobserver for builds. Source:
  https://github.com/DylanVann/lock. Default slots `max(2, cpus/3)`, no release, needs `cargo`.
  Not run here (**unverified**).
- **nlock** (Zig): named locks with `--slots n`, FIFO, but "a small daemon that tracks every lock"
  started with `brew services start nlock`. Source: https://github.com/erietz/nlock. A daemon is a
  new moving part and the licence is GPL-3.0. Not run here (**unverified**).

### 3.5 The GNU make jobserver protocol and its implementations

- The protocol: a pipe or FIFO preloaded with one character token per slot; "To obtain an extra slot
  you must read a single character from the jobserver; to release a slot you must write a single
  character back." The manual tells implementers to "write back the tokens it read, even under error
  conditions... (`SIGINT`)", i.e. it relies on the holder cleaning up. Source:
  https://github.com/mirror/make/blob/master/doc/make.texi (node "POSIX Jobserver").
- **Local run:** a FIFO with 2 tokens, two holders that each read one, `kill -9` of holder A: zero
  tokens were available afterwards, so the slot was lost. A token is data in a pipe, not a kernel
  lock; R4 fails. (My emulation used a Python FIFO keeper, not GNU make 4.4; macOS ships make 3.81,
  which has no FIFO jobserver.)
- The protocol limits parallel **jobs inside a build**, not whole commands, and only tools that speak
  it take part: Ninja 1.13+ ("GNU Jobserver support",
  https://github.com/ninja-build/ninja/blob/master/doc/manual.asciidoc), cargo, GNU make 4.4+.
  `lock`'s README lists `tsc`, `go` and `bun` among tools that ignore it. So `go test`, `tsc`, vitest,
  golangci-lint (our heavy commands) would not take tokens. Ninja `pool`s are per build file
  (same manual, "Pools"), not cross-process.
- Implementations: https://github.com/rust-lang/jobserver-rs (83 stars, Apache-2.0, pushed 2026-10-06,
  a Rust library, not a CLI), https://github.com/olsner/jobclient (C client/server for shell scripts,
  MIT, last push 2022-07-19), https://github.com/kesyog/fifo-jobserver (0 stars, no release, "mostly
  just used for testing"), https://github.com/milahu/gnumake-jobclient-js (moved). None is a drop-in
  counting wrapper. Verdict: **no fit**.

### 3.6 Daemons and queues: pueue, task-spooler

- **pueue** (6388 stars, Apache-2.0, v4.0.4, 2026-03-02,
  https://github.com/Nukesor/pueue): "Manage your shell commands" through the `pueued` daemon, with
  groups that have a `parallel` limit. Needs a running daemon, runs commands as the daemon's
  children (not the caller's), no nested rule. `mise ls-remote github:Nukesor/pueue` works, but the
  release ships `pueue` and `pueued` as separate assets and the `github:` backend installed only
  `pueue-aarch64-apple-darwin` (**local run**). Not tested further (**unverified**); the shape
  (daemon, detached jobs) does not match "a command wrapper that dies with its holder".
- **task-spooler** (444 stars, GPL-2.0, last push 2024-03-06,
  https://github.com/justanhduc/task-spooler): a per-socket job server; not in `mise registry`; not
  tested (**unverified**).

### 3.7 `sem`, `lockrun`, `flock` and friends

- GNU `sem`: counting, `--id`, state under `~/.parallel/semaphores/`; see section 6.2 of the earlier
  note for the quote and for why it was rejected (Perl, no nested rule, kill behaviour undocumented).
  Still not installable through mise (`mise registry parallel` and `mise registry flock` both return
  "tool not found in registry", **local run**) and not tested here (**unverified**).
- `flock` is a mutex per file; `discoteq/flock` (https://github.com/discoteq/flock, 245 stars, ISC) is
  what Homebrew ships. Its release assets are source tarballs (`mise ls-remote github:discoteq/flock`
  lists 0.3.0 and 0.4.0 but the two assets of each release are not binaries, so the `github:` backend
  gives nothing useful; **unverified** beyond the asset count of 2).
- `lockrun` (https://github.com/pushcx/lockrun, 64 stars, 2016) "ensure[s] that only one instance
  runs at a time": a mutex. `semaphoric` (https://github.com/timbunce/semaphoric, 7 stars, 2015) is a
  counting `flock` shell utility, i.e. our loop again.

### 3.8 Build tools and OS controls

- Ninja `pool`s and mise `jobs` limit parallelism inside one invocation (the earlier note, section
  6.2, and the Ninja manual above). **turborepo, nx, bazel, systemd, launchd, `nice`:** not checked
  in a primary source here (**unverified**). `nice` changes CPU share, not admission, which is what
  `loadmaster`'s README argues too ("Everything still runs at once; tests get slower and time out
  instead of waiting their turn").

### 3.9 How other repositories solve it

All of these are `flock` over numbered slot files in a shell script, i.e. the design `heavy.sh`
already has:

- **jax-ml/jax** (36k stars), `build/parallel_accelerator_execute.sh`: `JAX_LOCK_DIR` under `/tmp`,
  `exec {lock_fd}>".../jax_accelerator_lock_${i}_${j}"` then `flock -n "$lock_fd"` in a loop, "flock
  locks are automatically released when the FD is closed". Source:
  https://github.com/jax-ml/jax/blob/main/build/parallel_accelerator_execute.sh (lines 63-87).
  `openxla/xla` has copies under `build_tools/`.
- **Skrates/morphe**, `Justfile` lines 135-138: heavy recipes "queue on the flock (up to 2 h)", run
  under `nice` or a scope, `MORPHE_HEAVY_GATE=off` bypasses for dedicated CI runners and
  `_MORPHE_GATE_HELD` "makes nested gated recipes reentrant". Source:
  https://github.com/Skrates/morphe. This is our env-var nesting rule and CI skip
  verbatim.
- **cromford-ai/cromford**, `bin/mc-keyed-pool.sh`: "take a free slot (locks/mc-keyed-slot-<i>.lock,
  flock -n, i < N)", written after a hub ran at load 171-238 with 26-112 jobs alive. Source:
  https://github.com/cromford-ai/cromford/blob/main/bin/mc-keyed-pool.sh.

No repository found uses a TypeScript or Node semaphore for this (search of `gh search repos`
and `gh search code` for the phrases in the task; the search API rate limit cut the code search
short, so **unverified** as a negative).

### 3.10 The TypeScript primitive, checked here

The earlier note proposed a loopback TCP port per slot and marked three claims **unverified**. Local
run (`/private/tmp/claude-501/s/w/slot.mjs`, node 26.12.1):

- A process that `listen(47123, '127.0.0.1')` holds the slot; a second one gets `EADDRINUSE`
  (exit 3). Confirmed.
- `kill -9` of the holder, **while its spawned child (`sleep 15`, `stdio: 'inherit'`) was still
  alive**: a new process bound the same port at once. So the kernel frees the slot on SIGKILL and
  the child does not inherit the listening socket (node/libuv sets close-on-exec). This also covers
  the "left-behind daemon must not keep the slot" case (the e2e Mailpit) for a node-spawned child.
  Both **confirmed**.
- Remaining risk: a port clash with another program; mitigated by an env override
  (`SPHERICON_LOCK_PORT`) and by `EADDRINUSE` being indistinguishable from "slot busy" (a foreign
  listener would permanently take a slot; a wait that never ends is the failure mode). A Unix-
  domain socket path avoids that at the price of stale-file handling.

---

## 4. Recommendation (ranked)

### 1. Rewrite in TypeScript with loopback-port slots (recommended)

Meets R1-R7 with no install and no new dependency: node 26 runs `.ts` directly; the slot primitive is
verified above (kernel release on SIGKILL, no fd leak into a left-behind child).

Design (about 60 lines, `mise/scripts/heavy.ts`):

- `if (process.env.SPHERICON_HEAVY_HELD || process.env.CI)` then spawn the command directly (R3, R6).
  The "no flock" branch disappears (R6 second half).
- `slots = SPHERICON_HEAVY_SLOTS ?? max(1, floor(os.availableParallelism()/4))` (R1);
  base port `SPHERICON_LOCK_PORT` (default in the dynamic range).
- Loop over `base..base+slots-1`: `net.createServer().listen(port, '127.0.0.1')`; on success keep the
  server open in the parent and `spawn(cmd, args, { stdio: 'inherit', env: {...env, SPHERICON_HEAVY_HELD: '1'} })`;
  on `EADDRINUSE` try the next; if all busy print `heavy: all N slots are busy, waiting for one` once
  and retry after 1-3 s jittered (R2).
- Forward `SIGINT`/`SIGTERM` to the child and exit with its code or re-raise its signal.

Task line (replaces `mise/scripts/heavy.sh`, all 19 lines):

```
node mise/scripts/heavy.ts go test ${CI:+-shuffle=on} "$@"
```

Blast radius: 19 command lines in `mise/tasks.toml` (go test x4, vitest x2, entc, 6 ogen runs, goverter,
tsc, knip, custom-gcl x2, golangci-lint custom; the two `cd ent && ../mise/scripts/...` and
`cd tools/analyzers && ../../mise/scripts/...` lines need their relative path changed); delete
`mise/scripts/heavy.sh`; update the one filename mention in `AGENTS.md` ("Heavy tasks queue
machine-wide") and `docs/agents/dev-environment.md` lines 62-70; `flock` leaves the "needs on the
PATH" prose (no `brew install flock`). Cost per heavy command: one node start (**unverified**,
expect tens of ms; incoda's Go start measured 0.09 s here).

### 2. Adopt incoda (best ready-made, if a 6-week-old project is acceptable)

`.mise.toml`:

```
[tools]
"github:deblasis/incoda" = "0.7.3"

[env]
HEAVY = "{% if get_env(name='CI', default='') != '' %}{% else %}incoda run --queue builds --{% endif %}"
```

Task line: `$HEAVY go test ${CI:+-shuffle=on} "$@"` (the template and a `$HEAVY` line were run locally).
A one-time `setup` step must run `incoda config builds --slots "$(( $(getconf _NPROCESSORS_ONLN) / 4 ))"`
(minimum 1), because the slot count is stored in the user's state directory and the env var of
`heavy.sh` is gone; `SPHERICON_HEAVY_SLOTS` would become that command's argument. Nested calls need
no change (`INCODA_HELD`). Needs `incoda` also in the CI image if CI ever stops skipping (it
does skip through the template, but `mise install` still installs the tool unless CI excludes it).
Blast radius: the same 19 lines (prefix `mise/scripts/heavy.sh` becomes `$HEAVY`), `.mise.toml`,
`mise.lock`, the `setup` task, delete `heavy.sh`, docs as above. Open questions for the owner: the
pool is shared machine-wide by other projects on the machine; a stray `incoda config builds`
elsewhere changes our N; stderr gets an `acquired` line on every run; 30-minute default wait cap
(`--wait`); pre-1.0 CLI.

### 3. Keep `heavy.sh` (as is, or with the perl fallback)

It already satisfies R1-R6 and mirrors what JAX, XLA, morphe and cromford do. Its one weak spot is the
`flock` dependency on macOS (`brew install flock`): mhdsilva/heavy shows the `perl -MFcntl=:flock`
fallback works on stock macOS (`/usr/bin/perl` here, `flock` import verified, **local run**). It
violates only the "scripts are TypeScript only" rule.

### Not recommended

- **loadmaster**: weights and a machine config add concepts we do not need; capacity adapts to outside
  CPU; the `cwd` rule changes `cd ent && ...` lines.
- **agent-task-queue**: deadlocks on nesting, 20-minute default command timeout, Python runtime.
- **nlock, pueue, ts**: need a daemon or server.
- **GNU make jobserver**: loses a slot when the holder is killed; `go`, `tsc`, vitest do not speak it.
- **GNU `sem`, `lockrun`, `flock` CLIs**: not installable through mise, or a mutex.

---

## 5. Not confirmed in a primary source

- Whether any maintained, widely used (>100 stars, 2026 activity) tool exists beyond the ones listed:
  the GitHub search API hit its rate limit (403) partway through the code searches, so some queries
  ("jobserver rust cli" variants, "limit concurrency of commands across processes", "process limiter
  cli slots") returned nothing or were not repeated.
- Behaviour on Linux of incoda, loadmaster and tq (only macOS arm64 was run; the READMEs claim Linux).
- `lock` (DylanVann) and `nlock`: message, nested and kill behaviour not run; no mise install path.
- pueue and task-spooler: only installed or read, not run; GNU `sem` not run at all.
- How `tq` detects a zombie lock (pid liveness is inferred from the message only).
- turborepo, nx, bazel, systemd cgroups, launchd, `nice` as machine-wide limits across invocations.
- Node start-up cost of `heavy.ts` per heavy command (not measured).
- incoda behaviour when two checkouts call `incoda config builds` concurrently with different
  `--slots` (the README says a `run` may not disagree with the configured width; the config command
  itself was not raced).
- Whether `github:discoteq/flock` can ever install a usable binary (asset count seen, contents not
  inspected).
