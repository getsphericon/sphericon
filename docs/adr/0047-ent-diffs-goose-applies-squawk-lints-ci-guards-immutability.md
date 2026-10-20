---
status: accepted
---

# Ent diffs, goose applies, Squawk lints, CI guards immutability

Migrations had two runners: the Atlas CLI in development (`atlas_schema_revisions`) and goose in
the binary, the start guard and the tests (`goose_db_version`). The CLI also owned a checksum file
(`atlas.sum`) and a lint, and a database touched by one runner was unsafe for the other.

## Decisions

- **goose is the only runner.** Development, tests and production apply `migrations/` with the same
  function (`internal/db.Migrate`, `sphericon migrate`) and read the same `goose_db_version` table.
- **ent diffs, through the library.** `cmd/db generate` uses ent's versioned-migration engine (the
  Atlas Go library, kept in `go.mod`) against a scratch diff database and writes a goose file. It
  needs no CLI and no checksum file: the file is hidden from the library in memory.
- **Squawk lints.** `cmd/upgradecheck` runs Squawk over the migrations a PR adds (ADR 0033).
- **CI guards immutability.** `cmd/upgradecheck immutable` fails a PR that modifies or deletes a
  migration already on `main`, replacing the checksum's job; only a PR body line
  `History-Collapse: <reason>` may rewrite history.
- The Atlas CLI, `atlas.hcl` and `migrations/atlas.sum` are removed. A developer with an older
  dev database runs `mise run db:reset` once.

## Consequences

- One tool less in the toolchain and one migration table to reason about.
- Editing an applied migration is caught by a CI failure, not by a hash mismatch at `migrate` time.
