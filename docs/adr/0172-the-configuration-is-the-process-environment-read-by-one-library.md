---
status: accepted
---

# The configuration is the process environment, read by one library

> Amends [[0060-an-instance-runs-in-one-mode]] (an empty `SPHERICON_ENV` no longer counts as
> development, and `SPHERICON_MODE=saas` no longer refuses a start for want of the settings only a
> deployment gives).

## Context

The `config` package read its variables with caarlos0/env, but most of the package was code around
it: a second struct the variables were parsed into and then copied field by field into `Config`, a
stack of `.env` files layered under the process environment, a `_FILE` sibling for each secret,
defaults that depended on `SPHERICON_ENV` (no rate limits in `e2e`, text logs and private SMTP hosts
outside production), validator rules with messages of its own, cross-field refusals (the `saas`
Mode's required settings, the JWT secret's strength outside development, listener ports, Sign-in
pairs, a bare Reply-To) and a generator that walked the package's AST to write the configuration
reference. Each piece was small; together they were most of the package, and every new variable
touched several of them.

The project is greenfield: no Instance depends on the variables' names or formats.

## Decisions

- **The process environment is the only source.** The binary reads no `.env` file and no `_FILE`
  sibling. A developer's checkout gets its variables from the `[env]` table of `.mise.toml` (personal
  overrides in `.mise.local.toml`), a deployment from its orchestrator: Kubernetes `env`/`envFrom`
  with a Secret, compose `environment`/`env_file`, systemd `Environment`/`EnvironmentFile`. The Helm
  chart takes its secrets through `existingSecret` alone.
- **No default depends on `SPHERICON_ENV`.** Each default is the production one, in an `envDefault`
  tag. The dev stack sets `SPHERICON_LOG_FORMAT=text` and `SPHERICON_SMTP_ALLOW_PRIVATE_HOSTS=true` in
  `.mise.toml`; the end-to-end harness sets its zero rate limits, its private SMTP host and its empty
  metrics address itself. `SPHERICON_ENV` only sets `IsDev` and `IsE2E`, and unset is `production`.
- **One struct, parsed by caarlos0/env.** `config.Environment` holds every variable with its `env`
  and `envDefault` tags and is embedded in `Config`, which adds what Load derives (the Mode's
  switches, the dev flags) and what a test sets. Typed fields replace the copying: durations,
  `[]netip.Prefix`, comma-separated lists, `webhook.Allowlist` and `Mode` through `UnmarshalText`,
  Sign-in credentials as nested structs with `envPrefix`, the idle-connection default through
  `,expand`.
- **The start refuses only what cannot work.** A missing or empty `SPHERICON_DATABASE_URL`,
  `SPHERICON_JWT_SECRET` or `SPHERICON_ENCRYPTION_KEY` (`required,notEmpty`), a value that does not
  parse as its type, and an unknown `SPHERICON_MODE`. Ranges, cross-field rules and the JWT secret's
  strength are not checked at the start; `sphericon doctor` keeps its own check of the secret.
- **envdoc writes the reference** (`docs/configuration.md`) from the same tags and the fields' doc
  comments, between markers in a hand-written page (`mise run generate:configdoc`).
- **The variables' contract changes with it:** the retention and live-window variables are Go
  durations (`SPHERICON_EVENTS_RETENTION=9600h` for 400 days, `SPHERICON_MESSAGES_RETENTION`,
  `SPHERICON_UNVERIFIED_USER_RETENTION`, `SPHERICON_EVENT_LIVE_WINDOW`,
  `SPHERICON_TEST_INBOX_RETENTION`); lists are comma-separated; the System sender's display name
  is `SPHERICON_SYSTEM_EMAIL_FROM_NAME` beside a bare `SPHERICON_SYSTEM_EMAIL_FROM`; `DO_NOT_TRACK`
  is a boolean; a free-form value is taken as written (no trimming, lowercasing or trailing-slash
  stripping);
  a Sign-in provider is enabled by its client id.

## Considered Options

- **Keep the validation with go-playground/validator tags**: it would bring back the `saas`
  requirements, the pairs and the ranges as tags, at the price of a second library and of messages
  that name Go fields; rejected, since a wrong value of these shows at once where it is used.
- **A loader that layers files (koanf, viper, aconfig)**: it keeps the `.env` stack and the
  profiles, which mise and the harnesses already provide, and none of them writes the reference;
  rejected.
- **A configuration file with a schema (JSON Schema, CUE)**: the most declarative validation, but
  the mapping from the environment, the defaults and the decoding stay code; rejected.

## Consequences

- The package is the struct, `Load` and a few derived getters; a new variable is one tagged field
  with its doc comment.
- A typo in a value of a range or a pair (a rate limit, a half-set Sign-in provider, a listener on
  the public port) is no longer refused at the start: it shows where the value is used.
- A secret mounted as a file must be exported as a variable by whatever starts the process.
