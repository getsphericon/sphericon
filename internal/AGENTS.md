# Go test harness

`internal/testhelper.Setup(t)` is the standard fixture. Each test process gets a database of
its own, cloned from a template migrated + loaded with the committed YAML fixtures from
`fixtures/` (built once per change of either), then each test runs inside
a transaction that rolls back on cleanup (`pgcleaner`'s `Transaction` strategy, go-txdb underneath:
full isolation, DB stays at fixture state). Tests drive the real server in-memory through **actors** (typed ogen clients, see
below) — no sockets. See `internal/api/site/contacts_test.go`.

A test that needs a **real commit** (code that opens its own connection, the outbox router, concurrent
workers, a whole `app.App`) cannot ride a transaction. It takes a database of its own instead:
`testhelper.RealDB(t)` (a `*sql.DB`), `RealDBURL(t)` (a URL, e.g. `t.Setenv("DATABASE_URL", ...)` for
an app) or `EmptyDBURL(t)` (no schema, to migrate a fresh instance). pgtestdb clones it from a
template built once per schema or fixture change and drops it when the test ends, so no test
cleans up by hand or resets state at its start. The test role is a superuser (the fixture
loader disables foreign-key triggers), as on the shared test database.

**Build test scenarios on the committed fixtures — don't fabricate the primary
entities inline.** Reference fixture rows through the catalog constants below
(`fixtures.BroadcastDraftID`, `fixtures.ContactAliceID`, `fixtures.SegmentActiveID`, …),
never bare id literals (a deliberately missing id such as `999999` is fine). Query counts
that vary with the dataset dynamically instead of hardcoding them. If a
scenario isn't covered, **add a fixture row** rather than a `client.X.Create()` in the
test. Anchor rows flagged "DO NOT change" in the YAML are referenced across the suite —
leave them. txdb rolls back each test, so mutating a fixture row inside a test is fine.
Exception: incidental one-off records (e.g. a suppression to trip a specific edge) may be
created inline when no fixture expresses them.

## Fixture catalog

Anchor fixture rows have **named constants** in `internal/fixtures/catalog_gen.go`
(generated; never hand-edit). Reference `fixtures.AcmeID`, `fixtures.ContactAliceEmail`,
`fixtures.AcmeCollectKey`, … instead of bare id/email/key literals, so renaming or
removing an annotated row is a compile error and renumbering is absorbed by regeneration.

To add a name, put a line comment on the row's first line in `fixtures/*.yml` and run
`mise run generate`:

```yaml
- id: 1 # fixture: Acme
```

The generator (`cmd/fixturegen`, package `internal/fixtures/fixturegen`) reads the raw
files, so every template expression in fixtures must stay quoted (`'{{daysAgo 1}}'`).
It emits `<Name>ID` plus one constant per catalogued column the row carries (the
list is `catalogColumns` in `internal/fixtures/fixturegen/fixturegen.go`); unannotated
rows get nothing. It fails on a duplicate name or an annotation that matches no row.

### Credentials and anchor identities

Anchor credentials state their plaintext in the fixture and are hashed at load time by
the `argonHash` (user passwords, PHC argon2id) and `bcryptHash` (API token secrets)
template funcs, both at minimum cost (production verify code reads the cost from the
stored hash). The generator lifts the quoted literal into `<Name>Password` /
`<Name>Secret`, so `fixtures.OwnerJohnPassword`, `fixtures.AnchorTokenSecret` etc. can
authenticate as that identity; the full token is
`credentials.TokenValue(fixtures.AnchorTokenPrefix, fixtures.AnchorTokenSecret)`. Use them
only on annotated anchor rows, not dev tokens. The test values are plain constants in the
generated file; gitleaks needs no allowlist for them.

Anchor tenancy rows: Acme (owner `OwnerJohn`, member `MemberMary`), a second tenant
`Globex` (owner `OwnerJane`, membership `GlobexOwnerMembership`), and `OutsiderOscar`
with no membership. A test must not assume Acme has a single member or that only one
Workspace exists.

## Actors

An **actor** is a ready typed client for one API surface acting as one identity. Ask the
`TestEnv` for it; JWT signing, token generation and hashing (through the production token
code), security sources and the in-memory transport stay inside
`internal/testhelper/actor.go`. Tests never touch JWTs, headers or transports.

| Surface  | Identity                         | Call                                                        |
| -------- | -------------------------------- | ----------------------------------------------------------- |
| site     | fixture user                     | `env.SiteActor(t, fixtures.OwnerJohnEmail)`                 |
| site     | anonymous                        | `env.SiteAnonymous(t)`                                      |
| external | anchor token                     | `env.ExternalAnchor(t)`                                     |
| external | Acme token with exactly `scopes` | `env.ExternalScoped(t, "contacts:read")`                    |
| external | anonymous / raw bad credential   | `env.ExternalAnonymous(t)`, `env.ExternalWithToken(t, raw)` |
| collect  | Acme collect key                 | `env.CollectAcme(t)`                                        |
| collect  | anonymous / raw bad key          | `env.CollectAnonymous(t)`, `env.CollectWithKey(t, raw)`     |

Clients are the generated ogen clients (`siteapi.Client`, `externalapi.Client`,
`collectapi.Client`). JWT expiry comes from the helper's single `now()`, so a clock seam
can be injected there later. Do not mint JWTs, hash tokens, or build security sources or
clients in a test: ask the env for an actor. For a raw bearer (raw HTTP or MCP) use
`env.ScopedBearer(t, scopes...)`, or `env.ScopedBearerFor(t, fixtures.GlobexID, scopes...)`
for another tenant. Such helpers may persist a token with explicit scopes, but only
through the production hash/generate functions.
`env.MCPClient(t, bearer)` connects an MCP session.

Tenant-isolation tests use the committed Globex rows by name (`fixtures.SegmentGlobexID`,
`fixtures.TemplateGlobexName`, …). The admin role has no fixture user (a committed admin
would change the exact Acme member list), so a role test that needs one creates it inline;
owner and member are `OwnerJohn` and `MemberMary`.
