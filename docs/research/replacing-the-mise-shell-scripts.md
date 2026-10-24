# Replacing the six shell scripts in `mise/scripts/`

Researched 2026-10-24 against primary sources only (each tool's own docs, source code and
release assets). Every factual claim carries its source URL. Anything not confirmed on a primary
page fetched during this research is marked **unverified**. Three results were checked by running
the tool locally (mise 2026.10.17, macOS arm64, golangci-lint 2.15.0, node 26.12.1); they are
labelled **local run** and are evidence, not a citation. Repo facts come from reading
`mise/scripts/*.sh`, `mise/tasks.toml`, `.mise.toml`, `mise.lock`, `.custom-gcl.yml`,
`.github/workflows/{ci,nightly,tools}.yml` and `.github/dependabot.yml`.

The rule applied: scripts are TypeScript only (node 26 strips types), a ready utility beats our
own code, and a script is rewritten only when no ready utility fits.

**Short answer.**

| #   | Script                            | Verdict                                                             | Plan                                                                                                                                      |
| --- | --------------------------------- | ------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------- |
| 1   | `schemalens.sh`                   | WORKS WITH CAVEATS                                                  | Replace with mise's `github:` backend (`additional_asset_patterns`); delete the script                                                    |
| 2   | `contract.sh`                     | WORKS WITH CAVEATS                                                  | Replace with a mise daemon + `daemons`/`depends_post` on the task, then a plain `schemathesis` call; `start-server-and-test` DOES NOT FIT |
| 3   | `outdated-issue.sh` + `tools.yml` | WORKS WITH CAVEATS                                                  | Replace with Renovate's `mise` manager if the owner accepts a bot and PRs instead of one issue; otherwise rewrite in TS                   |
| 4   | `check-lint-pins.sh`              | WORKS WITH CAVEATS (half)                                           | Drop the `.custom-gcl.yml` half with `golangci-lint custom --version`; rewrite the go.mod half in TS                                      |
| 5   | `check-dev-ports.sh`              | DOES NOT FIT                                                        | Rewrite in TS (or delete, see 5.4)                                                                                                        |
| 6   | `heavy.sh`                        | DOES NOT FIT (GNU `sem` is the nearest, and fails two requirements) | Rewrite in TS with loopback-port slots; keep the CI and nested-call rules                                                                 |

---

## 0. The six scripts and who calls them

| Script               | Called by                                                                                                                                  | Blast radius                                                                                                          |
| -------------------- | ------------------------------------------------------------------------------------------------------------------------------------------ | --------------------------------------------------------------------------------------------------------------------- |
| `schemalens.sh`      | task `check:schema` (`mise/tasks.toml:502-506`, sets `SCHEMALENS_VERSION`)                                                                 | 1 task; run by the `test` job of `.github/workflows/ci.yml` (line 193)                                                |
| `contract.sh`        | task `check:contract` (`mise/tasks.toml:573-583`, after `db:up` and four `go run` steps)                                                   | 1 task; run by `.github/workflows/nightly.yml` (line 94)                                                              |
| `outdated-issue.sh`  | `.github/workflows/tools.yml` (weekly cron, `GH_TOKEN`)                                                                                    | 1 workflow, 1 script                                                                                                  |
| `check-lint-pins.sh` | task `check:lint-pins`, a `depends` of `check:be` (`mise/tasks.toml:478`)                                                                  | 1 task, but inside `mise run check`, so every PR                                                                      |
| `check-dev-ports.sh` | task `dev` (`mise/tasks.toml:25`)                                                                                                          | 1 task, Linux only                                                                                                    |
| `heavy.sh`           | 19 command lines in `mise/tasks.toml` (test, test:race, test:cover, test:e2e, vitest, generate:backend, tsc, knip, custom-gcl, build:lint) | The widest: most heavy tasks, and so every CI job. Also described in `AGENTS.md` and `docs/agents/dev-environment.md` |

---

## 1. `schemalens.sh`: install the adapter through mise

### 1.1 What the script does

It downloads `schemalens-adapter-ent_<ver>_<os>_<arch>.tar.gz` from the `mokevnin/schemalens`
release, checks it against `checksums.txt`, caches it in `~/.cache/sphericon/` and puts it on
`PATH` for `schemalens check --adapter ent`. Its stated reason is that "mise's github backend
installs one asset per repo".

### 1.2 What mise's `github:` backend says

- **One repo as two tools via aliases.** "If they are separate tools, give each one a tool alias
  that points at the same repository, and select its asset with `matching`" and "Each alias is a
  separate tool with its own version and install directory."
  Source: https://mise.jdx.dev/dev-tools/backends/github.html (section "Installing several tools
  from one release"; same text in
  https://raw.githubusercontent.com/jdx/mise/main/docs/dev-tools/backends/github.md).
- **Two plain entries do not work.** "Two entries for the same `github:owner/repo` with different
  `matching` values do not work: mise treats them as one tool, uses the first entry's options, and
  never installs the other binary." Same source.
- **A base archive plus an add-on is a first-class case.** `additional_asset_patterns`:
  "Downloads more archives from the same release and extracts them into the primary asset's
  install directory, in the order listed." and "`mise lock` records the URL and checksum of every
  extra archive, and `mise install --locked` fails when one is missing from the lockfile."
  Same source (section `additional_asset_patterns`). The extra assets "must be archives" and are
  extracted "without the primary asset's `strip_components`, `bin` and `rename_exe`", and "a file
  in a later archive replaces the same path from an earlier one".
- **Options.** `asset_pattern`, `matching`, `matching_regex`, `additional_asset_patterns`,
  `bin_path`, `bin`, `rename_exe`, `filter_bins`, `version_prefix`, `checksum`, `size` all exist,
  and "These options can be set per platform: `asset_pattern`, `additional_asset_patterns`, `url`,
  ... `rename_exe`, `filter_bins`, `checksum` and `size`." Same source.
- **Verification.** "When GitHub reports a digest for the asset, or the release publishes a
  checksum file that lists it, mise checks the download against it. When lockfiles are enabled,
  mise records the checksum in mise.lock and checks later installs against it." Same source
  (section Verification). This is the check `schemalens.sh` does by hand with `checksums.txt`.
- **One archive, several binaries** is also supported (`rename_exe` as a table, `filter_bins`),
  but is not needed here: the schemalens release ships the CLI and each adapter as separate
  archives.

### 1.3 The release layout today (checked with `gh release download`, v0.1.3)

- `schemalens_0.1.3_<os>_<arch>.tar.gz` holds `LICENSE`, `README.md`, `schemalens`.
- `schemalens-adapter-ent_0.1.3_<os>_<arch>.tar.gz` holds `LICENSE`, `schemalens-adapter-ent`.
- Asset names use `darwin`/`linux` and `amd64`/`arm64`; Windows ships `.zip`.
- The `.mise.toml` already installs the CLI as `"github:mokevnin/schemalens" = "0.1.3"`.

So no change to the release layout is needed: the CLI archive is the primary asset and the ent
adapter archive is an `additional_asset_patterns` entry. Both binaries land in one install
directory, so both are on `PATH` and `schemalens check --adapter ent` finds
`schemalens-adapter-ent` (the script relies on the same PATH lookup).

### 1.4 Local run

**Local run** (scratch config, isolated `MISE_DATA_DIR`, not the repo): with a per-platform table

```toml
[tools."github:mokevnin/schemalens"]
version = "0.1.3"
[tools."github:mokevnin/schemalens".platforms]
macos-arm64 = { asset_pattern = "schemalens_*_darwin_arm64.tar.gz", additional_asset_patterns = [
  "schemalens-adapter-ent_*_darwin_arm64.tar.gz",
] }
linux-x64 = { asset_pattern = "schemalens_*_linux_amd64.tar.gz", additional_asset_patterns = [
  "schemalens-adapter-ent_*_linux_amd64.tar.gz",
] }
linux-arm64 = { asset_pattern = "schemalens_*_linux_arm64.tar.gz", additional_asset_patterns = [
  "schemalens-adapter-ent_*_linux_arm64.tar.gz",
] }
macos-x64 = { asset_pattern = "schemalens_*_darwin_amd64.tar.gz", additional_asset_patterns = [
  "schemalens-adapter-ent_*_darwin_amd64.tar.gz",
] }
```

`mise install` put `schemalens` and `schemalens-adapter-ent` in one directory, and `mise lock`
wrote, for every platform, the CLI `url` and a `[[...additional_artifacts]]` block with its own
`url`, `url_api` and `checksum = "sha256:..."` for the adapter.

### 1.5 Caveats

1. **Use per-platform tables, not a templated pattern.** A first attempt with the Tera templates
   (`{{ os(macos="darwin") }}`, `{{ arch(x64="amd64") }}`) resolved against the host: `mise lock`
   wrote the macOS adapter URL under the `linux-*` entries (**local run**). The documented
   templates (`os()`, `arch()`) are real, but they are evaluated for the machine running the
   command, so they cannot produce a multi-platform lockfile. Four explicit platform entries
   (macos-arm64, macos-x64, linux-x64, linux-arm64) avoid it; `mise.lock` already carries musl
   and Windows platform entries, and the `-musl` entries inherit the glibc asset in the local run.
   Windows gets only the CLI (no table), which is fine for this repo.
2. **Version bumps stay manual.** The adapter's version is the CLI's version (one tag), so the
   `SCHEMALENS_VERSION` env in `check:schema` disappears and `.mise.toml` becomes the single pin.
3. **`LICENSE` is overwritten** by the later archive (same path); harmless.
4. **Releases must keep publishing both archives under these names.** `asset_pattern` is a glob
   that "must match the whole name", and "when it matches several assets, mise picks the shortest
   name", so `schemalens_*_darwin_arm64.tar.gz` cannot collide with `schemalens-adapter-*`.
   Source: https://mise.jdx.dev/dev-tools/backends/github.html.

**Verdict: WORKS WITH CAVEATS.** `check:schema` becomes `run = "schemalens check --dsn \"$DATABASE_URL\" --adapter ent"` with
`depends = ["db:migrate"]`; delete `schemalens.sh` and the `SCHEMALENS_VERSION` env. The
script's own comment ("mise's github backend installs one asset per repo") is out of date:
`additional_asset_patterns` is the documented answer.

The `ubi:` and `aqua:` backends were not needed. The `ubi:` backend is built around one
executable per repo (**unverified**: not read in this pass), and an `aqua:` package needs a
registry entry for the repo, which is a change to a repo we do not control.

---

## 2. `contract.sh`: boot the server, fuzz, stop

### 2.1 What the script does

Builds `./cmd/server`, starts it with rate limits off, polls `/healthz`, runs `uvx schemathesis run
openapi/external.openapi.json --url ... -H "Authorization: Bearer ..."`, kills the server, prints
the distinct `"msg":"request failed"` log lines, and returns Schemathesis's exit code.

### 2.2 `start-server-and-test`: DOES NOT FIT

Read its README and `src/index.js` (https://github.com/bahmutov/start-server-and-test):

- Usage is `start-server-and-test start http://127.0.0.1:8080 test`; readiness uses `wait-on`
  ("Under the hood this module uses wait-on to ping the server"), default expected status 200
  and a 5 minute timeout. Source:
  https://raw.githubusercontent.com/bahmutov/start-server-and-test/master/README.md
- **Log capture: none.** The server is spawned with `stdio: ['ignore', 'inherit', 'inherit']`
  (`src/index.js`, `execa(start, { shell: true, stdio: [...] })`), so its output goes straight to
  the terminal; there is no file to `grep` for `request failed` afterwards. Source:
  https://raw.githubusercontent.com/bahmutov/start-server-and-test/master/src/index.js
- **Stop:** `tree-kill(server.pid, 'SIGINT')` in a `finally`, so the process tree is killed
  (good), but with SIGINT, not SIGTERM. Same source.
- **Exit code:** any failure is `process.exit(1)` (`src/bin/start.js`), so Schemathesis's exit
  codes (`0` pass, `1` check failed, `2` could not run) collapse to 1. Source:
  https://raw.githubusercontent.com/bahmutov/start-server-and-test/master/src/bin/start.js and,
  for Schemathesis's codes,
  https://raw.githubusercontent.com/schemathesis/schemathesis/master/docs/reference/cli.md
- **Env:** goes through the shell and inherits `process.env` (code reading; the README does not
  describe it). It would also add an npm dependency (`wait-on`, `execa@5`, `tree-kill`, ...) to a
  repo that wants none.

The failure-log tail is a feature of the script, and this tool cannot give it.

### 2.3 Schemathesis itself: no server management

The CLI reference documents no option that starts or manages a server. The only readiness
feature is `--wait-for-schema SECONDS`: "Maximum time in seconds to wait for the API schema to
become available. Useful when testing services that take time to start up." Our schema is a file
(`openapi/external.openapi.json`) with `--url` as the API base ("Required for file-based
schemas"), so there is no schema URL to wait on. Source:
https://raw.githubusercontent.com/schemathesis/schemathesis/master/docs/reference/cli.md.
The in-process modes are Python-only: "Test a Python web application ... in-process ... Requests
go straight to the ASGI or WSGI callable, so no server has to be running."
(https://raw.githubusercontent.com/schemathesis/schemathesis/master/docs/guides/python-apps.md);
they do not apply to a Go binary.

### 2.4 GitHub Actions `services:`: DOES NOT FIT

Services are Docker containers: "Docker containers that provide a simple and portable way for you
to host services", Linux runners only, defined by an `image`. The page does not describe running
a binary built in the job. Source:
https://docs.github.com/en/actions/using-containerized-services/about-service-containers
It would also make the nightly job the only place the contract run works; the local
`mise run check:contract` would not.

### 2.5 mise daemons (pitchfork): fits, with caveats

- A task can require daemons: "`daemons` ... Project daemons that must be running and ready
  before the task runs. Requires pitchfork 2.25.0 or later." mise "starts missing daemons and
  waits until they are ready before the task runs. They keep running afterwards; stop them with
  `mise daemons stop`." Source: https://mise.jdx.dev/tasks/task-configuration.html (section
  `daemons`).
- Cleanup that survives failure: `depends_post` "tasks run after this task finishes ... Post-
  dependencies run whether this task succeeds or fails, as long as it started." Same source
  (section `depends_post`). This replaces the `trap ... kill`.
- Readiness: `ready_http` ("HTTP endpoint URL to poll for readiness. By default, any 2xx
  response is ready."), `ready_cmd`, `ready_port`. Stop: "pitchfork sends the configured signal
  to the entire process group", SIGTERM by default, then SIGKILL after a timeout. Source:
  https://pitchfork.jdx.dev/reference/configuration.html. This covers the "kill the tree" need.
- `mise daemons logs [NAME]` shows daemon output, so the `request failed` tail becomes
  `mise daemons logs contract | grep '"msg":"request failed"' | sort -u | head -20`. Source:
  https://mise.jdx.dev/daemons.html (Manage daemons table). It is a small post task, not a script.
- Run command guidance: "A `run` command runs in the project's tool environment ... start the
  command with `exec` so it receives stop signals directly." Same daemons page.
- Daemons are marked experimental in mise (the repo already enables them for `dev`).

Caveats:

1. **Environment.** A daemon runs "in the project's tool environment" with `[env]`. That a
   task-level `env = { SPHERICON_ENV = "e2e", DATABASE_URL = ... }` does _not_ reach the daemon is
   my reading of those docs (**unverified**; test it). The safe form is a daemon `run` that sets
   them itself: `exec env SPHERICON_ENV=e2e DATABASE_URL=$E2E_DB_URL PORT=... .bin/server`,
   `E2E_DB_URL` being already in `[env]`.
2. **Build step.** A daemon `run` starts a long-lived process, so build first (`go build -o
   .bin/contract-server ./cmd/server` as a task in `depends`); do not use `go run` (the script's
   own comment says it leaves the server behind; whether pitchfork's process-group kill makes
   `go run` safe is **unverified**).
3. **State.** `mise daemons` keeps per-project supervisor state; a stuck daemon from a crashed
   run needs `mise daemons stop contract` (the same lifecycle `dev` already has).
4. The `uvx schemathesis run ...` line stays a plain task command; its exit code is the task's.

**Verdict: WORKS WITH CAVEATS** (mise daemons). Net effect: `contract.sh` (about 45 lines, with a
trap, a poll loop and log handling) becomes one `[daemons.contract]` block, one task with
`daemons = "contract"` and `depends_post = ["check:contract:stop"]`, and one command.

---

## 3. `outdated-issue.sh` and `tools.yml`

### 3.1 What exists

Weekly, `mise outdated --local --json`, one open issue titled "Update the mise tools", closed when
nothing is outdated. Most tools are declared `latest` in `.mise.toml`, so `mise.lock` is the only
pin.

### 3.2 Dependabot: DOES NOT FIT

The supported-ecosystems table lists, among others, `devcontainers`, `docker`, `github-actions`,
`gomod`, `npm`, `pre-commit`, `rust-toolchain`, `uv`, `vcpkg`; no `mise`, `asdf` or tool-versions
row appeared in the rows read. Source:
https://docs.github.com/en/code-security/dependabot/ecosystems-supported-by-dependabot/supported-ecosystems-and-repositories
(the page is long and was read in two parts; a final scan for a late `mise` row is **unverified**,
but the script's own claim, "Dependabot has no mise ecosystem", matches).

### 3.3 Renovate's `mise` manager: fits, with caveats

- Files and scope: `mise.toml`, `.mise.toml`, `mise/config.toml`, `.config/mise*` and
  environment/local variants; "Renovate supports top level `tools` and `tasks.*.tools` keys."
  Sources: https://docs.renovatebot.com/modules/manager/mise/ and
  https://raw.githubusercontent.com/renovatebot/renovate/main/lib/modules/manager/mise/readme.md
- Lock file: "Renovate supports mise lock files (`mise.lock`)." "Dependencies will have their
  `lockedVersion` extracted from the lock file." "Renovate can update lock files when
  dependencies change."
- `latest`: "Fuzzy selectors such as `latest`, `lts`, `25`, `25.1` ... `latest` remains
  unbounded; partial selectors remain restricted to their major/minor range." Fuzzy selectors are
  "resolved against their locked version and updated in `mise.lock` only", which is exactly this
  repo's situation (`latest` in `.mise.toml`, the lock holds the version). Concrete versions such
  as `"github:mokevnin/schemalens" = "0.1.3"` and `pnpm = "12"` follow ordinary source-file
  updates.
- Lock refresh: "Lock file maintenance is supported via the `lockFileMaintenance` option.
  Maintenance runs `mise lock --bump`, which advances fuzzy selectors ... to the latest matching
  version." Same source.
- Visibility without a PR: Renovate can keep a "Dependency Dashboard" issue
  (`dependencyDashboard`, "will lead to the creation of a 'Dependency Dashboard' issue within the
  repository"). Sources:
  https://raw.githubusercontent.com/renovatebot/renovate/main/docs/usage/configuration-options.md
  and https://raw.githubusercontent.com/renovatebot/renovate/main/docs/usage/key-concepts/dashboard.md

Caveats:

1. **Renovate is a hosted app or a self-run workflow**, a new third-party dependency and a new
   config file. The repo pins every Action by SHA (`pinact`) and gives Dependabot a 7-day
   cooldown; Renovate would need the equivalent (`minimumReleaseAge`, **unverified** here).
2. **The output changes**: Renovate opens PRs that edit `mise.lock`, instead of one
   `ready-for-agent` issue that an agent works. Whether that suits the repo's agent flow is the
   owner's decision.
3. "Renovate treats mise lockfile refreshes as an unsafe execution" and needs an existing
   `mise.lock` and a mise version with safe mode; "If mise does not support safe mode or version
   detection fails, Renovate blocks all mise operations." Same Renovate pages. The repo's mise is
   current, so this is a setup check, not a blocker.
4. Whether Renovate resolves versions for `github:`/`aqua:` backed tools in `[tools]`
   (golangci-lint is `aqua:`, vacuum and osv-scanner are `github:`) is **unverified**: the docs
   say "Out of 1095 known tools: 1073 supported, 4 possibly supported, 18 unsupported" for short
   names (https://docs.renovatebot.com/modules/manager/mise/) and say nothing about full backend
   names.
5. Only the first version of each tool is updated ("Secondary or fallback versions require manual
   updates").

### 3.4 mise itself

`mise outdated --bump` compares "against the newest release overall", `mise lock --bump`
"re-resolve[s] version requests against the latest available versions" and "never modifies config
files", `mise upgrade --bump` rewrites `.mise.toml`. Sources:
https://raw.githubusercontent.com/jdx/mise/main/docs/cli/outdated.md,
https://raw.githubusercontent.com/jdx/mise/main/docs/cli/lock.md,
https://raw.githubusercontent.com/jdx/mise/main/docs/cli/upgrade.md. These are the commands the
script already uses; none files an issue or a PR. mise has no scheduler.

**Verdict: WORKS WITH CAVEATS (Renovate).** It can replace both `outdated-issue.sh` and
`tools.yml`. If the owner does not want a bot, the fallback is a TypeScript rewrite of the script
(about 40 lines: `child_process` for `mise outdated --local --json` and `gh`, `JSON.parse`, a
table template string); the workflow stays.

---

## 4. `check-lint-pins.sh`

### 4.1 What it asserts

(a) `.custom-gcl.yml` `version` equals the golangci-lint version in `mise.lock`; (b)
`tools/analyzers/go.mod` requires the same `golang.org/x/tools` and
`github.com/golangci/plugin-module-register` as that golangci-lint release's own `go.mod`.

### 4.2 Half (a): remove it with a CLI flag

`golangci-lint custom` takes `--version`: the flag set defines `--version`, `--name` and
`--destination`, and `preRunE` loads the file, then `if c.opts.version != "" { cfg.Version =
c.opts.version }` before `cfg.Validate()`. `LoadConfiguration` only decodes the YAML, and
`Validate` ("root field 'version' is required") runs after the override, so the file may omit
`version`. Sources:
https://raw.githubusercontent.com/golangci/golangci-lint/main/pkg/commands/custom.go and
https://raw.githubusercontent.com/golangci/golangci-lint/main/pkg/commands/internal/configuration.go
**Local run**: `golangci-lint custom --help` on 2.15.0 lists `--version string  The golangci-lint
version used to build the custom binary`, and `golangci-lint version --short` prints `2.15.0`.
The file reference marks `version` as "Required" in the config
(https://raw.githubusercontent.com/golangci/golangci-lint/main/.custom-gcl.reference.yml), so
removing it relies on the flag path in the source above, not on a documented contract; **an
unconfirmed point is that the flag-only path is stable across releases**.

So `build:lint` becomes `golangci-lint custom --version "v$(golangci-lint version --short)"`
(under `heavy`), `version:` leaves `.custom-gcl.yml`, and "the file names the pinned release"
cannot drift. The env-var route does not exist: the source reads no environment variable for the
version (only a keep-temp-files one, `envKeepTempFiles`).

### 4.3 Half (b): no ready utility moves two modules to another module's go.mod versions

- `go list -m`, `go mod edit`, `go get` can read and set versions, but nothing derives the
  versions from another module's `go.mod`; that comparison is the script's job (`go mod download
  -json github.com/golangci/golangci-lint/v2@v<ver>` gives the `GoMod` path).
- Dependabot cannot group them across the boundary: groups are per update entry by `patterns`
  (the repo's `golang-x` group), and the analyzers module is a separate `go.mod` that the repo's
  `.github/dependabot.yml` does not list (`directory: '/'` only; **repo fact**). Even listed,
  grouping moves `golang.org/x/*` together but does not pin them to golangci-lint's versions.
  Docs: https://docs.github.com/en/code-security/dependabot/ecosystems-supported-by-dependabot/supported-ecosystems-and-repositories
- Renovate has no rule that couples a module to a version in a third module's `go.mod`
  (**unverified**, not found in the manager docs read).

**Verdict: WORKS WITH CAVEATS for (a); (b) rewrite in TypeScript.** The TS script needs only
node APIs: `child_process.execFileSync` for `golangci-lint version --short` and `go mod download
-json`, `fs.readFileSync` of the printed `GoMod` path and `tools/analyzers/go.mod`, a regex per
`require` line (or `go list -m -f '{{.Version}}'`, as today), and `process.exitCode`. About 30
lines, no dependency.

---

## 5. `check-dev-ports.sh`

### 5.1 What it does

On Linux, when the lowest of `CADDY_PORT`/`CADDY_HTTP_PORT` is below
`net.ipv4.ip_unprivileged_port_start` and Caddy has no `cap_net_bind_service`, it stops
`mise run dev` and prints two fixes (sysctl, or an unprivileged port block for
`.mise.local.toml`).

### 5.2 Sources

- Kernel: "`ip_unprivileged_port_start` ... defines the first unprivileged port in the network
  namespace. Privileged ports require root or CAP_NET_BIND_SERVICE in order to bind to them."
  Default 1024. Source:
  https://raw.githubusercontent.com/torvalds/linux/master/Documentation/networking/ip-sysctl.rst
- `CAP_NET_BIND_SERVICE`: "Bind a socket to Internet domain privileged ports (port numbers less
  than 1024)." Source: https://man7.org/linux/man-pages/man7/capabilities.7.html
- Caddy's systemd unit grants `AmbientCapabilities=CAP_NET_ADMIN CAP_NET_BIND_SERVICE`. Source:
  https://raw.githubusercontent.com/caddyserver/dist/master/init/caddy.service. That is the
  packaged-service answer; a dev daemon started by pitchfork does not get it.
- Caddy's docs pages read (`/docs/running`, `/docs/caddyfile/options`,
  `src/docs/markdown/running.md`, `install.md`, `faq.md`) do not discuss binding privileged
  ports or `setcap` (searched for `setcap`, `privileged`, `ip_unprivileged`, `permission
  denied`, `CAP_NET`: no hits). `http_port` and `https_port` are "For internal use only; does not
  change the HTTP port for clients", default 80 and 443, so they cannot lower what a browser
  connects to. Sources: https://caddyserver.com/docs/caddyfile/options and
  https://raw.githubusercontent.com/caddyserver/website/master/src/docs/markdown/running.md

### 5.3 Candidates

- **Caddy's own error**: the failure is the Go `bind: permission denied` that the script says
  only the daemon log shows. Whether `mise daemons start` surfaces a failed readiness check with
  the log tail is **unverified**; it could not be tested here (macOS has no such limit).
- **mise task guards**: `depends` and `run` can order a check, but mise has no built-in
  precondition (a `skip_if` exists for skipping, not for failing with advice;
  https://mise.jdx.dev/tasks/task-configuration.html lists it under "Skipping work"). **Unverified**
  beyond that listing.
- **Daemon `init`**: "Setup commands that run before the process starts" run in a daemon's
  start (https://mise.jdx.dev/daemons.html). It would still be our script, only moved.
- **A ready preflight utility**: none found in primary sources for "can this process bind port
  N". Not exhaustive.
- **Non-privileged HTTPS port**: already supported by the repo's design (primary checkout 443,
  other checkouts offset ports); making 443 optional changes the primary URL and the Caddyfile,
  a product decision, not a research one.

**Verdict: DOES NOT FIT.** Rewrite in TypeScript, node APIs only: `process.platform`,
`fs.readFileSync('/proc/sys/net/ipv4/ip_unprivileged_port_start')`, `child_process` for
`getcap`, `fs.realpathSync`, and the message. The "print the caddy block of `.mise.toml` with the
port changed" part (an `awk | sed` pipeline today) is a `readFileSync` plus a split on the first
blank line (about 40 lines). Alternative worth the owner's call: delete the check and accept
Caddy's `permission denied` in `mise daemons logs caddy` (smaller, but the advice is lost).

---

## 6. `heavy.sh`: a machine-wide counting semaphore

### 6.1 Requirements read from the script

N slots (default cores/4, min 1), shared by every checkout on the machine (files in
`${TMPDIR}/sphericon-heavy`), nested calls run at once (`SPHERICON_HEAVY_HELD`), no queue in CI,
kernel-released slot when the holder dies ("the kernel drops it when the holder ends, however it
ends"), the command does not inherit the lock descriptor (a daemon left behind must not keep the
slot), works on macOS (needs `brew install flock`).

### 6.2 Candidates

| Candidate                | Counting                                          | Cross-checkout  | Nested calls          | Holder killed   | macOS                                    | Via mise                          |
| ------------------------ | ------------------------------------------------- | --------------- | --------------------- | --------------- | ---------------------------------------- | --------------------------------- |
| `flock` alone            | no (one lock per file; the script's loop makes N) | yes             | no (script's env var) | kernel releases | not shipped; Homebrew `flock` (discoteq) | not in `mise registry` 2026.10.17 |
| GNU `sem`                | yes                                               | yes, via `--id` | no                    | see below       | needs Perl and GNU parallel              | not in `mise registry`            |
| mise `jobs`              | yes, per run                                      | no              | n/a                   | n/a             | yes                                      | built in                          |
| mise `interactive`/`raw` | exclusive only (one)                              | no              | n/a                   | n/a             | yes                                      | built in                          |
| pitchfork                | no                                                | n/a             | n/a                   | n/a             | yes                                      | `pitchfork`                       |
| `lockrun`, others        | **unverified**                                    |                 |                       |                 |                                          |                                   |
| node CLI                 | **none found in primary sources**                 |                 |                       |                 |                                          |                                   |

Evidence:

- **GNU `sem`** "acts as a counting semaphore": "When GNU sem is called with command it starts
  the command in the background. When num number of commands are running in the background, GNU
  sem waits for one of these to complete before starting the command." `-j N` sets the count,
  `--fg` runs in the foreground, `--id name` names the semaphore, "The semaphore is stored in
  ~/.parallel/semaphores/", and the default id is the controlling tty's name, so a script must set
  `--id` (the page says so). `--semaphore-timeout` takes the semaphore anyway after a timeout.
  Source: https://www.gnu.org/software/parallel/sem.html. It is a close fit for counting and for
  cross-checkout sharing (same `--id`, same `~/.parallel`). It does **not** document nested-call
  behaviour: a heavy task calling `sem --fg` again inside a held slot would take another slot,
  and with N=1 would deadlock, so the env-var bypass would still have to be a wrapper. What
  happens to a slot when the holder is SIGKILLed is not stated on that page (**unverified**).
  It needs Perl and a manual install; a search of `mise registry` (2026.10.17) found no `parallel`
  or `flock` entry, and the aqua registry code search (a fuzzy search) returned only unrelated
  packages, so installing it through mise is **unverified**/not available.
- **`flock` alone** is a mutex; a counting semaphore is exactly the loop the script already has.
  Homebrew's `flock` is the discoteq port ("**flock(1)** locks files",
  https://raw.githubusercontent.com/discoteq/flock/master/README.md). Not on macOS by default.
- **mise `jobs`**: "How many tasks run in parallel", inside one `mise run`
  (https://mise.jdx.dev/tasks/task-configuration.html). Separate `mise run` invocations
  (separate checkouts, agents) do not share it: the docs describe the setting per run, nothing
  cross-process (**inference**). The `interactive` lock "holds an exclusive lock from the task's
  first script command to its last, so no other task's commands run in the meantime", which is a
  per-run lock too. The docs mention no semaphore feature
  (https://mise.jdx.dev/tasks/task-configuration.html).
- **pitchfork** supervises daemons (`ready_*`, stop, `oneshot`); it has no cross-process counting
  primitive (https://pitchfork.jdx.dev/reference/configuration.html).
- **Node** has no `flock`: no `lock`-named export in `require('fs')` on node 26.12.1 (**local
  run**), and no `flock` in
  https://raw.githubusercontent.com/nodejs/node/main/doc/api/fs.md. `proper-lockfile` is a
  library (not a CLI) using the `mkdir` strategy ("works atomically on any kind of file system"),
  with staleness by `mtime` (`stale` default 10000 ms) and the note that it "automatically
  removes locks if the process exits, except if the process is killed with SIGKILL". Source:
  https://raw.githubusercontent.com/moxystudio/node-proper-lockfile/master/README.md. It is a
  mutex, not a counting semaphore, and a killed holder blocks the slot for `stale` ms, which is
  weaker than the kernel guarantee the script has.

### 6.3 Verdict: DOES NOT FIT; rewrite in TypeScript

No ready utility meets all four of nested calls, kernel-released slots on kill, macOS without an
extra install, and mise-installability. GNU `sem` is the nearest and misses nested calls and a
documented kill behaviour, plus Perl.

What a TS version needs (node only, no dependency):

- **Slot primitive: a loopback TCP port.** `net.createServer().listen(base + i, '127.0.0.1')`
  either binds (slot taken) or fails with `EADDRINUSE` (slot busy). The kernel frees the port when
  the process ends, however it ends, which is the property `flock` gives, on macOS and Linux with
  no install, no files and no stale-lock logic. Node documents `EADDRINUSE` as the listen error
  (https://raw.githubusercontent.com/nodejs/node/main/doc/api/net.md). That the bound socket
  releases on SIGKILL is general OS behaviour (**unverified** in a primary source here; test it
  with `kill -9`). Choose the base port in the dynamic range and make it configurable
  (`SPHERICON_LOCK_PORT`); the per-user cost is a possible clash with another process on those
  ports, which a Unix-domain-socket path under `SPHERICON_LOCK_DIR` would avoid, at the price of
  stale-socket cleanup (that variant has an unlink race, so prefer the port).
- **The command must not hold the slot's socket**: the wrapper keeps the listening server in the
  parent and spawns the child with `child_process.spawn(cmd, args, { stdio: 'inherit' })`; a
  listening socket created by node is close-on-exec, so a left-behind daemon does not keep the
  slot (**unverified**: confirm with the e2e Mailpit case the script comments on).
- **Keep the three rules**: `SPHERICON_HEAVY_HELD` set in the child's env makes nested calls run
  at once; `CI` set skips the queue; slots default to a quarter of `os.availableParallelism()`,
  minimum 1, overridable by `SPHERICON_HEAVY_SLOTS`. The "no flock, no queue" branch disappears.
- Retry loop: `setTimeout` with the same 1-3 s jitter, one "all slots busy" message. Forward
  `SIGINT`/`SIGTERM` to the child and exit with its code (`child.on('exit', ...)`, re-raise a
  signal exit).
- Startup cost: node adds roughly tens of milliseconds per heavy command (measure; **unverified**).

---

## 7. Ranked plan

Ordered by value for effort; blast radius is the number of tasks or workflows that change.

1. **`schemalens.sh`: replace with the mise `github:` backend** (`additional_asset_patterns`,
   per-platform tables). Reason: documented feature, verified by a local run, removes the
   hand-rolled download and checksum code. Blast radius: 1 task (`check:schema`), 1 CI job
   (`test`); `mise.lock` gains the adapter artifacts after `mise lock`. Risk: low.
2. **`check-lint-pins.sh`: remove half (a) with `golangci-lint custom --version`; rewrite half (b)
   in TypeScript.** Reason: the flag exists in the pinned release (local run) so one of two checks
   disappears; the go.mod comparison has no ready tool. Blast radius: 1 task, but it sits inside
   `mise run check` (every PR), plus `build:lint` and `.custom-gcl.yml`. Risk: low-medium (flag-only
   path is not a documented contract).
3. **`contract.sh`: replace with a mise daemon plus `daemons`/`depends_post`.** Reason: mise
   covers readiness, process-group stop, cleanup on failure and logs. Not
   `start-server-and-test` (no log capture, collapses exit codes, adds npm dependencies) and not
   Schemathesis or Actions services (no server management / Docker only). Blast radius: 1 task
   (`check:contract`, nightly only). Risk: medium (daemon env handling and the build step need a
   test; experimental feature).
4. **`outdated-issue.sh` + `tools.yml`: replace with Renovate's mise manager, if the owner accepts
   a bot and PRs instead of one issue; otherwise rewrite the script in TypeScript.** Reason: only
   Renovate understands `mise.lock` and `latest`, and it replaces the workflow too; Dependabot has
   no mise ecosystem. Blast radius: 1 workflow plus a new config file; caveats in 3.3 (new
   third-party dependency, `github:`/`aqua:` support unverified). Risk: medium, mostly a process
   decision.
5. **`check-dev-ports.sh`: rewrite in TypeScript** (or delete and rely on Caddy's own error).
   Reason: no ready preflight utility; the sysctl and `getcap` logic is node-only code.
   Blast radius: 1 task (`dev`), Linux only. Risk: low; cannot be tested on macOS.
6. **`heavy.sh`: rewrite in TypeScript with loopback-port slots.** Reason: no ready utility meets
   nested calls, kernel-released slots, macOS and mise-installability together. Blast radius: the
   largest, 19 command lines in `mise/tasks.toml` (swap `mise/scripts/heavy.sh` for
   `node mise/scripts/heavy.ts`), `AGENTS.md`, `docs/agents/dev-environment.md`, and every CI job
   that runs a heavy task (they skip the queue, so behaviour there does not change). Risk: medium;
   do it last and test killed-holder and nested cases explicitly. Once items 1-6 land,
   `check:shell` (shellcheck over `mise/scripts/*.sh`) has nothing left to check and can go.

## 8. Not confirmed in a primary source

- Renovate support for `github:`/`aqua:` backed tools, and a Renovate `minimumReleaseAge`
  equivalent of the Dependabot cooldown.
- A late `mise` row in Dependabot's ecosystem table (the page was read in two parts).
- Whether a task `env` reaches a mise daemon; whether pitchfork's group kill makes `go run`
  safe; whether `mise daemons start` prints the log tail on a failed readiness check.
- Caddy's behaviour and message on a `permission denied` bind (no Linux host here).
- `ubi:` backend limits; GNU `sem` behaviour when its holder is SIGKILLed; `lockrun` and any node
  semaphore CLI; that a node-bound loopback socket is released on SIGKILL and is not inherited by
  a left-behind child.
- Whether the `golangci-lint custom --version`-only path (no `version` in the file) is stable in
  later golangci-lint releases.
