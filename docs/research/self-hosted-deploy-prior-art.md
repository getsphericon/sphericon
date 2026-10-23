# Self-hosted deployment prior art: modern Go open-core and single-binary projects

Researched 2026-10-18 from primary sources only: shallow clones of each project's default branch (commit SHAs
below; permalinks use the first 8 characters), each project's own docs as they live in its repository (Listmonk,
Hatchet, Zitadel, Teleport, VictoriaMetrics, Coder) or in its docs repository (Temporal's `temporalio/documentation`),
and sphericon's own files at its working tree. No blog posts, no third-party write-ups. This note **builds on**
[self-hoster-upgrade-prior-art.md](./self-hoster-upgrade-prior-art.md) (migration timing, guards, `upgrade`
command, breaking-change notes); it does not repeat that note's Gitea, GitLab, Grafana, Metabase, Authentik, Vault,
Sentry or PostHog findings. New here: distribution formats, config and secrets, first-run, the **edition and license
mechanics of one open-core binary**, health, backup, and HA across eight Go projects.

Limits, stated once. "Not found" means a targeted search of the named files found nothing, not that it cannot exist
elsewhere. Helm charts for Hatchet, Temporal, Zitadel and Teleport live in separate repositories that were **not**
cloned, so chart behaviour is taken from the docs pages that describe it and is marked as such. SigNoz's
installer, Foundry, is a separate project and was not read. Release signing of the other projects was not
examined. Docs-site URLs were not fetched: every doc citation below is the file in the cloned repository.

| Repo                            | Branch | Commit                                     | Last commit |
| ------------------------------- | ------ | ------------------------------------------ | ----------- |
| knadh/listmonk                  | master | `33896f18bcc9136327cee26e38ebbb23e9f39717` | 2026-10-19  |
| SigNoz/signoz                   | main   | `6c6632f940858ef63e8dca1c6fd5c1c19469bffb` | 2026-10-16  |
| temporalio/temporal             | main   | `abc10e2c0e05ab8dd554859c7ec3a6c60257b857` | 2026-10-18  |
| temporalio/documentation        | main   | `15a0f373952e1542d044fe094c9686a2abea182d` | 2026-10-16  |
| hatchet-dev/hatchet             | main   | `4cc02dd69f31d0cfb83fb3ecd32e88f058678e88` | 2026-10-17  |
| gravitational/teleport          | master | `1283425b60ec5f60d509ba4c791183d452923ff7` | 2026-08-25  |
| zitadel/zitadel                 | main   | `5cd91e00d62fa5fbbe100eb9fd21c01d4426da84` | 2026-10-16  |
| VictoriaMetrics/VictoriaMetrics | master | `90b89d5806d54a4636e3a8ed15b6770d2b5bdd36` | 2026-10-16  |
| coder/coder                     | main   | `121393842cc1ee2ca8cb769e400c4b3d17a49a82` | 2026-10-18  |
| mattermost/mattermost (note)    | master | `f38678d08d67a4d21ea3821f78b530ea7f8f8ec2` | 2026-10-18  |

Permalink form: `https://github.com/<org>/<repo>/blob/<8-char sha>/<path>#L<n>`. All eight main projects were
committed to within the last two months, so none is dropped as unmaintained. Teleport's last commit is the oldest
(2026-08-25) and is still recent.

## 1. Summary of findings

1. **Only some of these ship one binary unlocked by a runtime key, as sphericon does.** Same binary plus key: Coder
   (signed JWT validated locally, added through the UI or `coder licenses add`) and, in the Mattermost note below, RSA
   verification with a primary and an alternate public key. **Separate enterprise artifacts**: VictoriaMetrics
   (`-enterprise` binaries and images, with the license code absent from the public repo), Teleport
   (`IsEnterpriseBuild`, enterprise binaries downloaded from an account dashboard), SigNoz (`cmd/community` and
   `cmd/enterprise` built and shipped separately, license checked **online** against SigNoz's servers).
   **No license key at all**: Listmonk (AGPL), Temporal (MIT), Hatchet (MIT), Zitadel (AGPL). sphericon's
   Ed25519-JWT-in-one-binary design is closest to Coder and is the cleanest of the group for a self-hoster, because
   verification is offline and there is no second artifact to track.
2. **Every project that migrates at boot paid for it in HA and probes, and the ones that separate it now do so by
   default.** Coder disabled Helm liveness probes by default in v2.30 because migrations ran under them, and tells HA
   operators to scale to zero or one before `helm upgrade` because old replicas hold locks
   ([best-practices.md:60](https://github.com/coder/coder/blob/12139384/docs/install/operate/upgrade/best-practices.md#L60)).
   Zitadel splits `init`, `setup` (every release) and `start`, and its chart runs the first two as Jobs
   ([updating_scaling.mdx](https://github.com/zitadel/zitadel/blob/5cd91e00/apps/docs/content/self-hosting/manage/updating_scaling.mdx)).
   Hatchet ships a dedicated `hatchet-migrate` binary and image and its compose file runs it as a service. This is
   direct support for ADR 0033's "migrations are explicit by default", and it is evidence **against** the earlier
   note's recommendation (a) to default `AUTO_MIGRATE=true` (section 6).
3. **Listmonk is no longer the "explicit step" example the earlier note filed it under.** Its shipped compose file
   chains `--install --idempotent --yes && --upgrade --yes && run` on every container start
   ([docker-compose.yml:22](https://github.com/knadh/listmonk/blob/33896f18/docker-compose.yml#L22)), and its upgrade doc
   calls `docker compose pull && up` the whole Docker upgrade. The explicit step is hidden inside the compose command.
4. **First-run is a designed step in four of the eight, and open registration is sphericon's gap.** Listmonk
   (`LISTMONK_ADMIN_USER`/`_PASSWORD` at install, optionally an API user), Zitadel (`FIRSTINSTANCE_*` env), Hatchet
   (`hatchet-admin quickstart`, `ADMIN_` seed variables) and Coder (first user through the login flow) all create the
   first administrator by a deliberate path. sphericon's first run is "open the app, register, create a workspace"
   ([quickstart.md:30](../guide/quickstart.md)), `SiteAuthRegister` is open to anyone
   ([internal/api/site/auth.go:21](../../internal/api/site/auth.go)), and no switch to close sign-up was found in
   the env table or in `internal/` (section 7, gap 1).
5. **sphericon is ahead of every project here on upgrade mechanics** (a floor recorded in the database, a Major walk,
   checksum-verified `upgrade`, upgrade notes embedded in the binary, a first-party OCI Helm chart with a pre-upgrade
   hook, a documented restore drill). The deltas are small: no shipped compose file, a stale `go run ./cmd/genkey` in the
   env table, an accepted ADR (0035) that the code does not implement yet, and the first-run gap above.
6. **Version-skipping policy splits the field**: Temporal and Teleport forbid it (one minor, or one major, at a time),
   VictoriaMetrics and Hatchet allow it unless release notes say otherwise, Coder warns that big jumps run more migrations.
   sphericon's "any release inside a Major, the last release of each Major across" is a defensible middle and the only
   one enforced by code.

## 2. Per-project findings

### 2.1 Listmonk (Go, AGPL, one binary plus Postgres; maintained)

- **Distribution.** A binary with the SPA embedded (`pack-bin` in
  [.goreleaser.yml](https://github.com/knadh/listmonk/blob/33896f18/.goreleaser.yml): linux, darwin, windows and BSDs,
  `CGO_ENABLED=0`), a Docker image on Docker Hub and GHCR built from `alpine:latest`
  ([Dockerfile:1](https://github.com/knadh/listmonk/blob/33896f18/Dockerfile#L1)), and a compose file. The Helm chart is **community
  maintained** by a third party (HelmForge), not first party
  ([installation.md](https://github.com/knadh/listmonk/blob/33896f18/docs/docs/content/installation.md), "Helm chart" section).
- **Schema.** Explicit commands: `--install` (tables), `--upgrade` (idempotent migrations, prompts to back up unless
  `--yes`), then the server. At boot a pending upgrade is **fatal**: "there are %d pending database upgrade(s) ...
  Backup the database and run listmonk --upgrade"
  ([cmd/upgrade.go:120](https://github.com/knadh/listmonk/blob/33896f18/cmd/upgrade.go#L120)). The Docker path automates
  both through the compose `command` (finding 3). No lock was looked for in this pass (the earlier note found none).
- **Rollback.** "DBs that have been upgraded with a particular version shouldn't be used with older versions"; the
  documented path is to restore the pre-upgrade database and pin the older image tag
  ([upgrade.md](https://github.com/knadh/listmonk/blob/33896f18/docs/docs/content/upgrade.md), "Downgrade").
- **Config.** TOML file (`--new-config` generates one), or purely env with `LISTMONK_<section>__<key>` and
  `--config=""`; most settings live in the admin UI, only low-level and DB settings are file/env
  ([configuration.md](https://github.com/knadh/listmonk/blob/33896f18/docs/docs/content/configuration.md)). The image
  entrypoint turns any `LISTMONK_*_FILE` variable into the plain variable from the file's content, for Docker secrets
  ([docker-entrypoint.sh:42-52](https://github.com/knadh/listmonk/blob/33896f18/docker-entrypoint.sh#L42)).
- **License.** None; AGPL only ([LICENSE](https://github.com/knadh/listmonk/blob/33896f18/LICENSE)). No enterprise tier.
- **First run.** Visiting the app creates the Super Admin, or `LISTMONK_ADMIN_USER`/`_PASSWORD` (min 3 and 8 chars)
  during `--install`, plus optional `LISTMONK_ADMIN_API_USER` whose token is printed to stderr
  ([cmd/install.go:82-104](https://github.com/knadh/listmonk/blob/33896f18/cmd/install.go#L82)).
- **Health, backup, HA.** `GET /health` ([cmd/handlers.go:338](https://github.com/knadh/listmonk/blob/33896f18/cmd/handlers.go#L338)).
  Backup guidance is the one-line "always take a backup of the Postgres database before upgrading". No HA guidance found.
- **Notes.** An in-app update checker (`checkUpdates`, [cmd/updates.go:39](https://github.com/knadh/listmonk/blob/33896f18/cmd/updates.go#L39));
  a "Nightly" channel is documented with an untested warning; v4's breaking change (multi-user) is written up as an
  "Upgrading to v4.x.x" section inside the same upgrade page.

### 2.2 SigNoz (Go backend plus React; two binaries; not a single-binary self-host)

Relevant mostly for its edition split; its own deployment needs ClickHouse and ZooKeeper (the migration guide
describes "ClickHouse + ZooKeeper + SQLite", [deploy/MIGRATION.md](https://github.com/SigNoz/signoz/blob/6c6632f9/deploy/MIGRATION.md)),
so it is not comparable on "one binary plus Postgres". One line of scope: kept because its edition mechanics are the
opposite of sphericon's.

- **Distribution.** Compose and `install.sh` are **deprecated** in favour of Foundry, a separate installer
  (`foundryctl forge` generates manifests, `cast` applies them)
  ([deploy/README.md:8](https://github.com/SigNoz/signoz/blob/6c6632f9/deploy/README.md#L8)); Foundry itself was **not read
  (unverified)**. Two separate builds, `cmd/community` and `cmd/enterprise`, each with its own Dockerfile
  ([cmd/community/Dockerfile](https://github.com/SigNoz/signoz/blob/6c6632f9/cmd/community/Dockerfile): `alpine:3.20.3`,
  `ENTRYPOINT ["./signoz", "server"]`) and its own release workflow
  ([gor-signoz-community.yaml](https://github.com/SigNoz/signoz/blob/6c6632f9/.github/workflows/gor-signoz-community.yaml)).
- **Schema.** The app's own SQL store migrates when the server is constructed
  ([pkg/signoz/signoz.go:373](https://github.com/SigNoz/signoz/blob/6c6632f9/pkg/signoz/signoz.go#L373)) under a
  `migration_lock` table ([pkg/sqlmigrator/migrator.go:18,51](https://github.com/SigNoz/signoz/blob/6c6632f9/pkg/sqlmigrator/migrator.go#L18)).
  Auto-migrate at boot, locked. Rollback story: not found.
- **Config.** `SIGNOZ_*` env variables with deprecation warnings when a name changes, naming the replacement
  ([pkg/signoz/config.go:224-240](https://github.com/SigNoz/signoz/blob/6c6632f9/pkg/signoz/config.go#L224)). A pattern worth
  noting: a renamed variable keeps working one cycle and logs the new name.
- **License.** Everything under `ee/` and `cmd/enterprise/` is under an Enterprise license that requires an agreement
  and "a valid SigNoz Enterprise license for the correct number of user seats"
  ([ee/LICENSE](https://github.com/SigNoz/signoz/blob/6c6632f9/ee/LICENSE)). The key is **validated online**: the
  provider polls the upstream (`zeus.GetLicense`) every `PollInterval` and tolerates `FailureThreshold` missed polls
  ([ee/licensing/httplicensing/provider.go:55-65,96,180-182](https://github.com/SigNoz/signoz/blob/6c6632f9/ee/licensing/httplicensing/provider.go#L180)).
  A self-hoster therefore needs outbound access to SigNoz.
- First run, health, backup, HA, upgrade notes: not read.

### 2.3 Temporal (Go, MIT; a service plus a database, not a single binary)

- **Distribution.** `temporalio/server` image from `alpine`, non-root user, `exec temporal-server start`
  ([docker/targets/server.Dockerfile](https://github.com/temporalio/temporal/blob/abc10e2c/docker/targets/server.Dockerfile),
  [entrypoint.sh](https://github.com/temporalio/temporal/blob/abc10e2c/docker/scripts/sh/entrypoint.sh)); a separate
  `admin-tools` image; compose in the `temporalio/samples-server` repo and Helm charts in `temporalio/helm-charts`, both
  referenced by the docs ([deployment.mdx:46,134](https://github.com/temporalio/documentation/blob/15a0f373/docs/production-deployment/self-hosted-guide/deployment.mdx#L46)).
  A server config template needs `sprig` rendering from 1.30 (same file, "Helm chart configuration").
- **Schema.** Explicit, separate tools (`temporal-sql-tool update-schema`, Cassandra and Elasticsearch tools) run
  **before** the server upgrade; the earlier note covers the boot-time check that refuses an older DB but tolerates a
  newer one ([version.go:9-28](https://github.com/temporalio/temporal/blob/abc10e2c/common/persistence/schema/version.go#L9)).
- **Ordering.** "Sequential Upgrades ... one minor version at a time", patch-latest first; "no guarantee that there is
  compatibility between _any_ two non-consecutive versions"; allow about ten minutes per version for shard metadata
  ([upgrade-server.mdx](https://github.com/temporalio/documentation/blob/15a0f373/docs/production-deployment/self-hosted-guide/upgrade-server.mdx)).
  Schema changes "will be called out directly in the release notes".
- **Config.** A YAML template; env substitution at start (the docs link `docker/config_template.yaml`). Not read in depth.
- **License.** MIT ([LICENSE](https://github.com/temporalio/temporal/blob/abc10e2c/LICENSE)); the paid product is Temporal
  Cloud, so there is no in-binary key. Not relevant to the license split.
- Backup, first-run, health: the self-hosted guide has `checklist.mdx` and `monitoring.mdx` that were not read in
  depth. **Unverified.**

### 2.4 Hatchet (Go, MIT; control plane of several binaries plus Postgres)

- **Distribution.** Separate binaries `hatchet-api`, `hatchet-engine`, `hatchet-admin`, `hatchet-migrate`,
  `hatchet-lite` (one image bundling engine and API), a CLI (`hatchet server start`), a compose file with a
  `hatchet-migrate` service, and two Helm charts, `hatchet-stack` and `hatchet-ha`
  ([self-hosting/index.mdx:31-40](https://github.com/hatchet-dev/hatchet/blob/4cc02dd6/frontend/docs/content/docs/self-hosting/index.mdx#L31),
  [docker-compose.mdx:78](https://github.com/hatchet-dev/hatchet/blob/4cc02dd6/frontend/docs/content/docs/self-hosting/docker-compose.mdx#L78)).
  The charts live in a separate repository **(not read)**.
- **Schema.** goose, with `lock.NewPostgresSessionLocker` and a `SessionLock` before migrating, and a baseline when
  `goose_db_version` is missing
  ([cmd/hatchet-migrate/migrate/run.go:160-185](https://github.com/hatchet-dev/hatchet/blob/4cc02dd6/cmd/hatchet-migrate/migrate/run.go#L160)).
  This is the same library and locker sphericon uses.
  **A contradiction in the sources.** The upgrade doc says "Hatchet runs database migrations automatically on engine
  startup. No separate migration step is required"
  ([upgrading-downgrading.mdx:36](https://github.com/hatchet-dev/hatchet/blob/4cc02dd6/frontend/docs/content/docs/self-hosting/upgrading-downgrading.mdx#L36)),
  yet the only non-test caller of `RunMigrations` outside `hatchet-migrate` is the test harness
  ([pkg/testing/harness/engine.go:181-185](https://github.com/hatchet-dev/hatchet/blob/4cc02dd6/pkg/testing/harness/engine.go#L181)),
  and both the Lite image start script and the compose file run `hatchet-migrate` first
  ([hack/lite/start.sh:4](https://github.com/hatchet-dev/hatchet/blob/4cc02dd6/hack/lite/start.sh#L4)). So "automatic"
  means the packaging runs the migrate binary before the engine, not that the engine migrates itself. Whether the
  Helm chart does so by a hook Job was **not verified**.
- **Rollback.** Snapshot first and restore it (recommended), or `hatchet-migrate --down <timestamp>` found from the last
  migration file of the target tag; "Both ... will result in data loss and some downtime"
  ([upgrading-downgrading.mdx](https://github.com/hatchet-dev/hatchet/blob/4cc02dd6/frontend/docs/content/docs/self-hosting/upgrading-downgrading.mdx),
  [downgrading-db-schema-manually.mdx](https://github.com/hatchet-dev/hatchet/blob/4cc02dd6/frontend/docs/content/docs/self-hosting/downgrading-db-schema-manually.mdx)).
  A release can add a Postgres extension requirement (`btree_gist` from v0.87.6, allow-listing needed on some managed
  providers, same file line 41): an upgrade prerequisite sphericon would also have to call out.
- **Config.** Env only, grouped by prefix (`SERVER_`, `DATABASE_`, `ADMIN_` ...), every secret also as `_FILE`; the
  required encryption keysets are base64 Tink keysets, or files, or Cloud KMS
  ([configuration-options.mdx:19-52](https://github.com/hatchet-dev/hatchet/blob/4cc02dd6/frontend/docs/content/docs/self-hosting/configuration-options.mdx#L19)).
  `hatchet-admin quickstart` generates keysets and config
  ([cmd/hatchet-admin/cli/quickstart.go:34](https://github.com/hatchet-dev/hatchet/blob/4cc02dd6/cmd/hatchet-admin/cli/quickstart.go#L34)).
- **License.** MIT ([LICENSE](https://github.com/hatchet-dev/hatchet/blob/4cc02dd6/LICENSE)); no key.
- **HA.** Documented as splitting the services into separate Deployments with their own replica counts (`grpc`,
  `controllers`, `scheduler`) in the HA chart, plus an HA database and a 3-replica RabbitMQ
  ([high-availability.mdx](https://github.com/hatchet-dev/hatchet/blob/4cc02dd6/frontend/docs/content/docs/self-hosting/high-availability.mdx)).

### 2.5 Teleport (Go, AGPL core; separate Enterprise build)

- **Distribution.** A single `teleport` binary installed by a one-line script that takes an **edition**
  (`oss`, `enterprise`, `cloud`) and a version and uses the OS package manager; Docker; Helm (`teleport-cluster`
  chart) ([upgrading-manual.mdx](https://github.com/gravitational/teleport/blob/1283425b/docs/pages/upgrading/upgrading-manual.mdx)).
- **Edition mechanics.** The build contains an `IsEnterpriseBuild` / `IsOSSBuild` distinction in its `modules`
  interface ([lib/modules/modules.go:283-287](https://github.com/gravitational/teleport/blob/1283425b/lib/modules/modules.go#L283)):
  enterprise code is compiled in by a different build, and the enterprise **binary is downloaded from the customer's
  account dashboard** together with a license file. The license is an **X.509 certificate plus private key in PEM**,
  read by the Auth Service from `auth_service.license_file` or `license.pem` in `data_dir`, and "the same license on all
  clusters" so seats are not double-counted
  ([installation/self-hosted/license.mdx](https://github.com/gravitational/teleport/blob/1283425b/docs/pages/installation/self-hosted/license.mdx),
  [lib/config/fileconf.go:909](https://github.com/gravitational/teleport/blob/1283425b/lib/config/fileconf.go#L909)).
  On Kubernetes it is a Secret named `license`. How the build tag is applied is in a private repository **(unverified)**.
- **Ordering and migrations.** Auth Service first, then Proxy (never newer than Auth), then agents (never newer than
  Proxy); **one major at a time** ("You must not upgrade directly from v15 to v18"); back up first because the Auth
  Service "may perform data migrations on its backend"
  ([upgrading/overview.mdx:51-54](https://github.com/gravitational/teleport/blob/1283425b/docs/pages/upgrading/overview.mdx#L51)).
  Agents also have managed updates (a separate updater). Rollback: not read.
- Config (a `teleport.yaml` file), first-run and health: not read in this pass.

### 2.6 Zitadel (Go, AGPL; one binary, Postgres)

- **Distribution.** A Linux/macOS binary, a compose stack (`deploy/compose`, Traefik in front), a Helm chart (own
  chart with an optional Postgres subchart), and an image whose final stage is **`FROM scratch`** with
  `HEALTHCHECK NONE` and a non-root user
  ([apps/api/Dockerfile:27-37](https://github.com/zitadel/zitadel/blob/5cd91e00/apps/api/Dockerfile#L27),
  [self-hosting/deploy](https://github.com/zitadel/zitadel/tree/5cd91e00/apps/docs/content/self-hosting/deploy)).
  Since the image has no shell, health probing is a subcommand: `zitadel ready`
  ([cmd/ready](https://github.com/zitadel/zitadel/tree/5cd91e00/cmd/ready)).
- **Schema.** Three phases: `init` (once ever: database, user and schemas; the provisioning half needs a superuser and
  can be skipped with `zitadel init schema`), `setup` (**every release**: migrations, idempotent), `start`.
  `start-from-init` combines them for getting started only. Setup is a numbered list of Go steps
  ([cmd/setup/](https://github.com/zitadel/zitadel/tree/5cd91e00/cmd/setup)). "The init phase has to run once over the
  full ZITADEL lifecycle ... The setup phase has to run each time a new ZITADEL version is deployed"; the Helm chart
  runs both in dedicated Jobs
  ([updating_scaling.mdx:7-28,38](https://github.com/zitadel/zitadel/blob/5cd91e00/apps/docs/content/self-hosting/manage/updating_scaling.mdx#L7)).
- **Upgrade notes.** A per-transition page (v3 to v4: back up, go to the latest v3.4.x, "soak until legacy-signed
  JWTs/sessions have expired", then v4, then `setup` and `start`) and "technical advisories" referenced by number
  ([upgrade-v3-to-v4.mdx](https://github.com/zitadel/zitadel/blob/5cd91e00/apps/docs/content/self-hosting/manage/upgrade-v3-to-v4.mdx)).
- **Config and secrets.** `ZITADEL_*` env mirroring a defaults YAML (`cmd/defaults.yaml`), plus a **masterkey of
  exactly 32 characters** passed as `--masterkey`/`ZITADEL_MASTERKEY`, "a single ZITADEL instance always needs the same"
  one ([_compose.mdx:48-51](https://github.com/zitadel/zitadel/blob/5cd91e00/apps/docs/content/self-hosting/manage/configure/_compose.mdx#L48)).
  This is the same role as `SPHERICON_ENCRYPTION_KEY`.
- **First run.** `ZITADEL_FIRSTINSTANCE_*` env creates the first organisation, admin and a machine user with a PAT
  written to a path ([deploy/compose/docker-compose.yml](https://github.com/zitadel/zitadel/blob/5cd91e00/deploy/compose/docker-compose.yml)).
- **Backup.** Distinctive: the binary is stateless and the state is an event log, "no need for a special backup job";
  use Postgres's own tooling
  ([production.mdx:154-163](https://github.com/zitadel/zitadel/blob/5cd91e00/apps/docs/content/self-hosting/manage/production.mdx#L154)).
- **License and telemetry.** AGPL ([LICENSE](https://github.com/zitadel/zitadel/blob/5cd91e00/LICENSE)); no license-key flag
  was found in the self-hosting docs read. A **Service Ping** sends anonymized usage by default, off with
  `ServicePing.Enabled: false` ([service_ping.mdx](https://github.com/zitadel/zitadel/blob/5cd91e00/apps/docs/content/self-hosting/manage/service_ping.mdx)):
  the shape ADR 0032 plans for sphericon.

### 2.7 VictoriaMetrics (Go, Apache-2.0 core plus Enterprise; single binaries and a cluster)

- **Distribution.** Binaries per component (`victoria-metrics-prod`, `vmagent`, `vmalert`, `vmbackup` ...) as tarballs,
  Docker images on Docker Hub and Quay (alpine root image, scratch as an option:
  [deployment/docker/Makefile:6-7](https://github.com/VictoriaMetrics/VictoriaMetrics/blob/90b89d58/deployment/docker/Makefile#L6)),
  compose files for single and cluster, Helm charts and a Kubernetes operator
  ([enterprise.md:110-122](https://github.com/VictoriaMetrics/VictoriaMetrics/blob/90b89d58/docs/victoriametrics/enterprise.md#L110)).
  It is the project closest to "one static binary configured by flags".
- **Config.** Command-line flags; `-envflag.enable` (with `-envflag.prefix`) reads them from the environment
  ([victoria_metrics_common_flags.md:53-56](https://github.com/VictoriaMetrics/VictoriaMetrics/blob/90b89d58/docs/victoriametrics/victoria_metrics_common_flags.md#L53)).
  No database schema: it owns its storage, so "migrations" are data-format compatibility.
- **Upgrade and downgrade rules.** "It is safe upgrading ... safe skipping multiple versions ... also safe downgrading to
  older versions unless release notes say otherwise"; stop with SIGINT, wait, start the new binary; single-node
  "cannot be restarted / upgraded or downgraded without downtime", the cluster can be restarted node by node in the
  order `vmstorage`, `vminsert`, `vmselect` ("no downtime strategy") or stopped for a "minimum downtime strategy" when
  versions are incompatible
  ([README.md:234-250](https://github.com/VictoriaMetrics/VictoriaMetrics/blob/90b89d58/docs/victoriametrics/README.md#L234),
  [FAQ.md:510-512](https://github.com/VictoriaMetrics/VictoriaMetrics/blob/90b89d58/docs/victoriametrics/FAQ.md#L510),
  [Cluster-VictoriaMetrics.md:701-760](https://github.com/VictoriaMetrics/VictoriaMetrics/blob/90b89d58/docs/victoriametrics/Cluster-VictoriaMetrics.md#L701)).
  The compatibility promise is carried by one convention: notes in a `CHANGELOG.md` whose `tip` section accumulates
  `FEATURE`/`BUGFIX` lines
  ([changelog/CHANGELOG.md](https://github.com/VictoriaMetrics/VictoriaMetrics/blob/90b89d58/docs/victoriametrics/changelog/CHANGELOG.md)).
- **License key in the "same binary"? Not quite.** Enterprise components are **separate artifacts with an `enterprise`
  suffix** (`victoria-metrics-linux-amd64-v1.154.0-enterprise.tar.gz`, `victoriametrics/victoria-metrics:v1.154.0-enterprise`)
  and "all the Enterprise components require" `-license` (the key) or `-licenseFile` (a path, re-read every
  `-licenseFile.reloadInterval`, default one hour), and `-license.forceOffline` turns on offline verification for keys
  issued for it, otherwise the key is verified against VictoriaMetrics' license server
  ([enterprise.md:127-170](https://github.com/VictoriaMetrics/VictoriaMetrics/blob/90b89d58/docs/victoriametrics/enterprise.md#L127)).
  The license-checking Go code is **not in the public repository** (a search of the Go sources for `licenseFile` found
  no hit), which is how the closed part stays closed. The docs' own compose example mounts the license file read-only.
- **Backup and HA.** `vmbackup` (incremental, snapshot based) and `vmrestore` are shipped tools, documented in
  [README.md:2512](https://github.com/VictoriaMetrics/VictoriaMetrics/blob/90b89d58/docs/victoriametrics/README.md#L2512)
  and pointed to from the cluster page; the cluster is the HA/scaling story
  ([Cluster-VictoriaMetrics.md:325-372](https://github.com/VictoriaMetrics/VictoriaMetrics/blob/90b89d58/docs/victoriametrics/Cluster-VictoriaMetrics.md#L325)).
  Not comparable to Postgres-backed products, but the **only project here that ships its own backup tool**.
- **Release cadence.** Enterprise-only **LTS lines** supported for 12 months, a new one every 6 months, two at a time
  ([LTS-releases.md:18](https://github.com/VictoriaMetrics/VictoriaMetrics/blob/90b89d58/docs/victoriametrics/LTS-releases.md#L18)).
  A paid-tier benefit that is a release policy, not code.

### 2.8 Coder (Go, AGPL core, Enterprise/Premium in the same repository; added)

Added because it is the closest to sphericon on both axes the research asked about: one Go binary, Postgres, boot-time
migrations (a cautionary tale), and an offline signed-JWT license unlocking enterprise features in the same
deployment. Chosen over Flipt and OpenMeter, which were not examined (one added project had to carry both points).

- **Distribution.** A binary via an install script, RPM, Docker, docker-compose, Helm (`helm/coder`), AMI, Windows
  installer ([install/operate/upgrade.md](https://github.com/coder/coder/blob/12139384/docs/install/operate/upgrade.md)).
  The upgrade for each is "reinstall by the original method".
- **Schema.** golang-migrate, run at server start through `ConnectToPostgres(..., migrations.Up, ...)`
  ([cli/server.go:3246](https://github.com/coder/coder/blob/12139384/cli/server.go#L3246)), with
  `pg_advisory_xact_lock` and a 2-minute lock timeout
  ([coderd/database/migrations/migrate.go:107-112](https://github.com/coder/coder/blob/12139384/coderd/database/migrations/migrate.go#L107),
  [txnmigrator.go:46](https://github.com/coder/coder/blob/12139384/coderd/database/migrations/txnmigrator.go#L46)).
- **Rollback and its costs.** "Coder does not support rollbacks" ([upgrade.md:8](https://github.com/coder/coder/blob/12139384/docs/install/operate/upgrade.md#L8)).
  Migrations take exclusive locks, so on Kubernetes old replicas' connections can block new ones: scale to zero (or
  one) before `helm upgrade`; large upgrades "typically 4-7 minutes"; and liveness probes were **disabled by default in
  the chart from v2.30.0** because they killed pods during long migrations
  ([best-practices.md:12-60](https://github.com/coder/coder/blob/12139384/docs/install/operate/upgrade/best-practices.md#L12)).
  This is exactly the failure ADR 0033 names.
- **License.** "Signed JWTs that are validated locally ... No outbound connection ... air-gapped" work
  ([install/prepare/licensing.md:13](https://github.com/coder/coder/blob/12139384/docs/install/prepare/licensing.md#L13)).
  The key is **stored in the database** (added by an Owner in the UI or `coder licenses add -f/-l`), not in env, and only
  unexpired licenses are loaded (`GetUnexpiredLicenses`,
  [enterprise/coderd/license/license.go:49](https://github.com/coder/coder/blob/12139384/enterprise/coderd/license/license.go#L49)).
  After expiry there is an `EntitlementGracePeriod` before features drop
  ([license.go:440-444](https://github.com/coder/coder/blob/12139384/enterprise/coderd/license/license.go#L440)); seats
  count users active in the last 90 days (licensing.md, FAQ). Enterprise code sits under `enterprise/` with
  `LICENSE.enterprise` at the root; whether it is the same compiled binary was **not verified**, but the licensing page
  describes unlocking features by adding a key to a running deployment.
- **Config and health.** `CODER_*` env and flags, e.g. `CODER_PG_CONNECTION_URL`
  ([admin/setup/index.md:108](https://github.com/coder/coder/blob/12139384/docs/admin/setup/index.md#L108)); `GET /healthz`
  ([admin/monitoring/health-check.md:17](https://github.com/coder/coder/blob/12139384/docs/admin/monitoring/health-check.md#L17)).
  First-run creates the first user through `coder login` or the web UI (details not read; **unverified**).

### 2.9 Mattermost (short note, license split only)

Dropped as a deployment comparison (legacy-style packaging, and the earlier note already covers its migrations). One
fact is instructive for `ee/`: the license is an RSA-signed blob checked in the binary against a **primary and an
alternate public key**, so the vendor can rotate the signing key without breaking issued licenses
([server/channels/utils/license.go:95-108](https://github.com/mattermost/mattermost/blob/f38678d0/server/channels/utils/license.go#L95)),
and enterprise code compiles into the same server from a directory with its own license
([server/enterprise/LICENSE](https://github.com/mattermost/mattermost/blob/f38678d0/server/enterprise/LICENSE)).
sphericon's `licensekey.ProductionKey` is one public key (internal/app/app.go:764).

## 3. sphericon today (primary files)

- **Formats.** Release tarballs for linux and darwin on amd64 and arm64 with `checksums.txt`
  ([.goreleaser.yaml](../../.goreleaser.yaml)); a multi-arch image `ghcr.io/getsphericon/sphericon` on `scratch`,
  non-root, `HEALTHCHECK` through `sphericon healthcheck` on `/healthz`, `SPHERICON_ENV=production` by default
  ([Dockerfile.release](../../Dockerfile.release)); a first-party OCI Helm chart with a pre-upgrade migrate Job, a
  `/readyz` probe, two replicas and a PDB
  ([charts/sphericon/templates/migrate-job.yaml](../../charts/sphericon/templates/migrate-job.yaml),
  [self-hosting.md:255-274](../self-hosting.md)); a systemd unit in the docs ([self-hosting.md:155-199](../self-hosting.md)).
  **No compose file**: none is tracked (`git ls-files` finds none) and the self-hosting page has no compose section,
  although [ROADMAP.md:224](../ROADMAP.md) says "compose ships `postgres:16`".
- **Schema.** Embedded goose migrations, explicit by default: `sphericon migrate`, `migrate --status [--json]`; opt-in
  `SPHERICON_DB_AUTO_MIGRATE` under a session advisory lock with `SPHERICON_DB_MIGRATE_TIMEOUT`
  ([self-hosting.md:95-122](../self-hosting.md), [ADR 0033](../adr/0033-upgrades-add-within-a-major-and-never-stop-a-running-instance.md)).
  A `schema_state` row holds the Major and a floor; guard refusals name the version and a docs link
  ([upgrading.md:"When it refuses"](../operations/upgrading.md)).
- **Upgrade.** `sphericon upgrade [--restart|--from|--version|--yes|--check]`, a Major walk, sha256-verified
  (releases not signed yet), embedded changelog with `Upgrade:` notes, `upgrade-notes --since`, Helm and Docker refusals
  naming the right command ([ADR 0034](../adr/0034-sphericon-upgrade-and-a-parseable-version.md),
  [upgrading.md](../operations/upgrading.md)). Rollback is a restore, because migrations are forward-only.
- **Config.** Env (and `.env`), one table of variables ([self-hosting.md:20-81](../self-hosting.md)); `sphericon genkey`
  for the Tink keyset ([cmd/server/main.go:44-49](../../cmd/server/main.go)); no `_FILE` variants.
- **License.** `SPHERICON_LICENSE_KEY`, an Ed25519 JWT with a `features` claim, verified offline against a compiled-in
  public key; empty is the core, a bad key fails boot
  ([ee/licensekey/licensekey.go:57-78](../../ee/licensekey/licensekey.go),
  [internal/app/app.go:764](../../internal/app/app.go)). ADR 0035 (accepted) says an expired key must load as no
  features instead of failing boot; **the code still returns `ErrInvalid` for an expired key** (the doc comment on
  `ErrInvalid` at licensekey.go:40 lists "expired", and `Parse` has no special case), so the ADR is not implemented.
- **First run.** Register in the UI, create a workspace; `SPHERICON_REQUIRE_EMAIL_VERIFICATION` defaults on
  ([quickstart.md:30](../guide/quickstart.md), [self-hosting.md:57](../self-hosting.md)). `workspace`, `user`, `staff`,
  `service-credential` commands exist for recovery and the platform surface ([cmd/server/cli.go:62-72](../../cmd/server/cli.go)).
- **Health, backup, HA.** `/healthz` (liveness), `/readyz` (DB ping); opt-in metrics listener (ADR 0018); backup is
  `pg_dump` plus the encryption key, PITR guidance and a restore drill
  ([backup.md](../operations/backup.md), [self-hosting.md:240-253](../self-hosting.md)); scaling notes on connection
  budgets and PgBouncer ([scaling.md](../operations/scaling.md)).

## 4. Comparison table

"n/a" means the project has no such thing. "not read" means this pass did not read a source for it.

| Concern                     | sphericon                                                  | Listmonk                                                | SigNoz                                     | Temporal                              | Hatchet                                            | Teleport                                  | Zitadel                                   | VictoriaMetrics                                                                | Coder                                      |
| --------------------------- | ---------------------------------------------------------- | ------------------------------------------------------- | ------------------------------------------ | ------------------------------------- | -------------------------------------------------- | ----------------------------------------- | ----------------------------------------- | ------------------------------------------------------------------------------ | ------------------------------------------ |
| Binary and image            | one binary (linux, darwin), scratch image, non-root        | one binary, alpine image                                | two builds (community, enterprise), alpine | server and admin-tools images, alpine | several binaries, Lite bundles two                 | one binary, edition-specific              | one binary, `scratch` image               | per-component binaries, alpine or scratch                                      | one binary, many installers                |
| Compose                     | none shipped (ROADMAP says one)                            | yes, chains install, upgrade, run                       | deprecated for Foundry                     | in `samples-server` repo              | yes, with a `hatchet-migrate` service              | not read                                  | yes (Traefik)                             | yes (single and cluster)                                                       | documented                                 |
| Helm                        | first-party OCI chart, pre-upgrade hook                    | community only                                          | via Foundry (not read)                     | `temporalio/helm-charts`              | `hatchet-stack`, `hatchet-ha` (separate repo)      | `teleport-cluster`                        | own chart, init and setup Jobs            | charts and operator                                                            | `helm/coder`                               |
| Schema creation and upgrade | explicit `migrate`; opt-in auto under advisory lock        | explicit `--install`/`--upgrade`; boot fatal if pending | auto at construction, lock table           | separate SQL tools before server      | separate `hatchet-migrate`, goose and session lock | Auth Service migrates backend on upgrade  | `init`, `setup` each release, `start`     | n/a (own storage)                                                              | auto at boot, advisory xact lock           |
| Newer-schema guard          | floor in DB; refuses too-old and skipped Major             | none found                                              | not found                                  | allows newer by design                | not found                                          | not read                                  | not found                                 | n/a                                                                            | not found                                  |
| Rollback                    | restore backup (forward-only)                              | restore backup                                          | not found                                  | not stated                            | restore snapshot or `--down`                       | not read                                  | not stated                                | "safe" unless notes say                                                        | "does not support rollbacks"               |
| Skip versions               | any release in a Major; walks Majors                       | not stated                                              | not read                                   | no: one minor at a time               | not stated                                         | no: one major at a time                   | via advisories; soak v3.4.x               | yes unless notes say                                                           | warns: more migrations                     |
| Config                      | env and `.env`, one table, no `_FILE`                      | TOML or `LISTMONK_*`, `_FILE` in image                  | `SIGNOZ_*` with deprecation logs           | YAML template                         | env by prefix, `_FILE`, Tink keysets               | `teleport.yaml`                           | env plus defaults YAML, 32-char masterkey | flags, `-envflag.enable`                                                       | `CODER_*` env and flags                    |
| Secret generation           | `sphericon genkey`, `openssl`                              | none needed                                             | not read                                   | not read                              | `hatchet-admin quickstart`                         | not read                                  | masterkey by `tr`/`urandom` snippet       | n/a                                                                            | not read                                   |
| License key                 | `SPHERICON_LICENSE_KEY`, Ed25519 JWT, offline, same binary | n/a (AGPL)                                              | online check vs vendor, separate build     | n/a (MIT)                             | n/a (MIT)                                          | X.509 PEM file, separate enterprise build | n/a found (AGPL)                          | `-license`/`-licenseFile`, vendor server unless offline key, separate artifact | JWT validated locally, in DB, grace period |
| Expiry behaviour            | code: boot fails; ADR 0035: fall back to core              | n/a                                                     | tolerates N missed polls                   | n/a                                   | n/a                                                | not read                                  | n/a                                       | not read                                                                       | grace period, then features drop           |
| First run                   | UI register (open), then workspace                         | UI or `LISTMONK_ADMIN_*` at install                     | not read                                   | n/a                                   | `hatchet-admin quickstart`, `ADMIN_` env           | not read                                  | `FIRSTINSTANCE_*` env                     | n/a                                                                            | first user via login (not verified)        |
| Health                      | `/healthz`, `/readyz`                                      | `/health`                                               | not read                                   | not read                              | not read                                           | not read                                  | `zitadel ready`                           | endpoint docs not read                                                         | `/healthz`                                 |
| Backup                      | `pg_dump`, PITR, restore drill                             | one-line warning                                        | not read                                   | not stated                            | snapshot, per-provider links                       | back up before upgrade                    | event-sourced, use Postgres tools         | `vmbackup`, `vmrestore` shipped                                                | snapshot before upgrade                    |
| HA and scale                | 2 replicas, PDB, pooled conns                              | none found                                              | not read                                   | not read                              | split services, HA chart                           | multi-Auth (not read)                     | stateless binary, N replicas              | cluster, rolling order                                                         | scale down before helm upgrade             |
| Release notes               | release-please, `Upgrade:` lines, embedded                 | release notes, upgrade page sections                    | not read                                   | schema changes called out in notes    | docs page, extension caveats                       | upgrade overview                          | per-transition page, advisories           | CHANGELOG `tip`, notes                                                         | best-practices page                        |
| Telemetry                   | planned (ADR 0032), not built                              | update checker                                          | not read                                   | UI notice                             | not read                                           | not read                                  | Service Ping, default on, opt out         | not read                                                                       | not read                                   |

## 5. Where this note disagrees with, or corrects, the earlier note

1. **Recommendation (a) "default `AUTO_MIGRATE=true`" and (d) "Compose: `AUTO_MIGRATE=true`, a `migrate` one-shot is
   unnecessary".** ADR 0033 already overrode both, and the new evidence agrees with the ADR: Coder's liveness-probe and
   scale-down guidance, Zitadel's separate Jobs, Hatchet's migrate image. Plausible and Directus (the earlier note's
   precedents) are not Go; the Go projects read here either separate migration or hide it in a chained command.
   For a future sphericon compose file the shape these projects point to is a one-shot `migrate` service that the app
   service waits on (`depends_on` with `service_completed_successfully`), not auto-migrate; Hatchet's compose does this
   explicitly.
2. **Listmonk row ("Explicit: `--upgrade`; non-final builds auto-run", "no explicit step in compose")** is incomplete:
   the shipped compose command runs `--install --idempotent` and `--upgrade --yes` on every start
   ([docker-compose.yml:22](https://github.com/knadh/listmonk/blob/33896f18/docker-compose.yml#L22)).
3. **Temporal "tool run once"** is right for the schema tool, but the docs add a rule the earlier note did not carry:
   upgrade one minor at a time and wait about ten minutes per version for shard rebalancing.
4. **Hatchet** was in the earlier note as "startup catalog, unverified"; it is a separate goose-based binary, and its docs
   overstate "automatic on engine startup" (section 2.4).
5. Not re-examined, so not contradicted: the earlier note's Mattermost, Gitea, GitLab, Grafana, Metabase, Authentik rows.

## 6. What the projects suggest about sphericon's choices

- **Explicit migration plus a floor guard is the right default** and is stronger than anything here. The one cheap
  thing nobody else has and sphericon does is naming the next command in the refusal text; Coder's docs and Zitadel's
  phases show the cost of not separating.
- **One binary, runtime key, offline verify** (sphericon, Coder) is a better self-hoster experience than a second
  artifact (VictoriaMetrics, Teleport, SigNoz). The cost sphericon accepts is that EE code ships to everyone; the
  runtime gate is the only barrier. VictoriaMetrics keeps the license code out of the public repo; SigNoz and Teleport
  keep the enterprise tree under a separate license. ADR 0014 already chose in-repo `ee/` with its own `LICENSE`.
- **Online license checks cost self-hosters an outbound dependency** (SigNoz, VictoriaMetrics by default). sphericon's
  design avoids it; keep it.
- **Expiry policy** (ADR 0035 versus Coder's grace period) is a product choice, not a mechanics gap.

## 7. Gaps and ideas for sphericon

Each is marked **worth an issue** or **not**, with the reason. All sphericon-side claims were checked in the working
tree on 2026-10-18.

1. **Closed first-run / open registration. Worth an issue.** A fresh public Instance has no deliberate first-admin step:
   `SiteAuthRegister` creates a user and workspace for any caller
   ([internal/api/site/auth.go:21,64](../../internal/api/site/auth.go)), and no env switch or CLI creates the first user
   or closes sign-up (searched `internal/` and the env table; none found). Listmonk, Zitadel, Hatchet and Coder all
   offer a deliberate first-run. Mitigations that exist: email verification defaults on (so the registrant needs a
   mailbox), rate limits on sign-up, and `SPHERICON_BOOTSTRAP_TOKEN` (which this pass saw only as an external-API token,
   not a first-user mechanism). What to scope: either `sphericon user create`/first-owner provisioning plus a
   `SPHERICON_REGISTRATION=closed|first-only|open` switch, or documentation of the risk. Needs a product decision (ADR 0004
   makes workspaces multi-user by Membership; a self-host with one company may want "invite only after the first").
2. **A shipped docker compose file. Worth an issue.** Six of the eight projects ship or document one; sphericon's
   self-hosting page has none and ROADMAP.md:224 claims one exists (stale or missing). Shape per section 5: Postgres
   with a healthcheck, a one-shot `migrate` service, the app on `depends_on: service_completed_successfully`, pinned
   image tags, secrets via an env file. Include it in the release workflow's artifacts or in the docs, and add a row to
   the docs that `self-hosting.md` links.
3. **ADR 0035 is accepted but not implemented. Worth an issue if none is open** (tracker not queried; not verified).
   `licensekey.Parse` returns `ErrInvalid` for an expired key
   ([ee/licensekey/licensekey.go:40,63-72](../../ee/licensekey/licensekey.go)), so an expired subscription still stops
   the Instance at the next restart, the exact outcome the ADR exists to prevent. Coder shows a variant (a grace period
   before features drop); decide whether the ADR's immediate fallback is right, then implement it with the warning,
   banner and `license_expired` metric the ADR lists. This is also the decision where VictoriaMetrics'
   `-licenseFile.reloadInterval` would matter: ADR 0035 judges expiry at boot only, so renewing needs a restart.
4. **Docs drift. Not an issue; fix directly.** `self-hosting.md:25` says "`go run ./cmd/genkey`", a directory that does
   not exist (the command is `sphericon genkey`, as lines 89 and `cmd/server/main.go:44` say); `ROADMAP.md:224` (item 2).
5. **`_FILE` variants for secrets. Not worth an issue yet.** Listmonk (entrypoint) and Hatchet support them for Docker
   and Swarm secrets; sphericon's Helm chart consumes a Secret as env, and `EnvironmentFile` covers systemd. Revisit
   when someone asks for Docker secrets; a generic loader for `SPHERICON_*_FILE` is small if wanted.
6. **A `ready` subcommand so a `scratch` or distroless image can self-probe. Done.** Zitadel uses a `scratch` image
   and `HEALTHCHECK NONE` plus `zitadel ready`. sphericon's image is now `scratch` too, and its `HEALTHCHECK` runs
   `sphericon healthcheck`, which probes the server's own `/healthz`; Listmonk, Temporal, SigNoz and VictoriaMetrics'
   default stay on alpine.
7. **Release signing and SBOM. Not a new issue.** sphericon has `checksums.txt` and states signing is a separate task
   (ADR 0034, no `signs:` in `.goreleaser.yaml`); this pass did not examine how the other projects sign, so there is
   no comparison to add.
8. **License-key rotation (alternate public key, Mattermost). Not worth an issue now.** Relevant only when the
   production signing key must change; adding a second public key to `licensekey` is trivial at that time, and keys
   issued so far are few.
9. **A built-in backup tool (`vmbackup`). Not worth an issue.** VictoriaMetrics owns its storage; sphericon's state is
   Postgres, `pg_dump`/PITR are the standard tools and the restore drill is already documented.
10. **Per-transition upgrade page and numbered advisories (Zitadel, Temporal).** Not worth an issue: ADR 0034 chose
    embedded `Upgrade:` notes with no per-version table, and the Major rule gives the same information.
11. **Enterprise-only LTS lines (VictoriaMetrics). Not worth an issue.** A paid-tier release policy; sphericon is
    pre-1.0 and ADR 0014 forbids gating product value.
12. **Extension prerequisites called out in upgrade notes (Hatchet `btree_gist`).** Fold into the existing `Upgrade:`
    notes convention if a migration ever requires a Postgres extension; no work now.

## Sources

Repositories at the commits in the table above, files cited inline. Additional files read without an inline cite:
`docs/docs/content/installation.md`, `upgrade.md`, `configuration.md` (Listmonk); `cmd/community/.goreleaser.yaml` (SigNoz);
`docs/production-deployment/self-hosted-guide/{deployment,upgrade-server}.mdx` (Temporal documentation repo);
`frontend/docs/content/docs/self-hosting/{index,docker-compose,hatchet-lite,high-availability}.mdx` (Hatchet);
`docs/pages/upgrading/overview.mdx`, `docs/pages/installation/self-hosted/license.mdx` (Teleport);
`apps/docs/content/self-hosting/**` (Zitadel); `docs/victoriametrics/{enterprise,README,FAQ,LTS-releases}.md`
(VictoriaMetrics); `docs/install/**` (Coder). sphericon: `docs/self-hosting.md`, `docs/operations/*.md`,
`docs/adr/{0014,0018,0033,0034,0035}-*.md`, `Dockerfile`, `Dockerfile.release`, `.goreleaser.yaml`,
`.github/workflows/release-please.yml`, `charts/sphericon/`, `cmd/server/*.go`, `ee/licensekey/licensekey.go`,
`internal/app/app.go`.

Could not verify: SigNoz Foundry and its Helm story; Helm charts of Hatchet, Temporal, Zitadel and Teleport (separate
repositories); Teleport's config, first-run, health and rollback; Temporal backup and health pages; whether Coder's
enterprise code is the same compiled binary; the other projects' release signing; whether an issue for ADR 0035
already exists in sphericon's tracker.
