# Codegen pipeline

The diagram, goverter and the generated-versus-hand-written rules. The list of files never to
edit by hand is in `AGENTS.md`.

```
typespec/{site,external,collect,staff,staffee,service,telemetry}   ──tsp compile──▶  openapi/*.openapi.json
  openapi/site      ──ogen──▶ gen/site       (Go server)   ──openapi-ts──▶ src/generated/site (TS client + react-query + zod)
  openapi/external  ──ogen──▶ gen/external    (Go server)  ──openapi-ts──▶ packages/node/src/generated/external (TS client + SDK, `@sphericon/node`, private)
  openapi/collect   ──ogen──▶ gen/collect     (Go server)  ──openapi-ts──▶ packages/analytics/src/generated/collect (types only)
  openapi/staff  ──ogen──▶ gen/staff    (Go server, core)  ──openapi-ts──▶ src/generated/staff (TS client + react-query + zod)
  openapi/staffee ──ogen──▶ gen/staffee (Go server, the console's Enterprise sections)  ──openapi-ts──▶ src/generated/staffee (TS client + react-query + zod)
  openapi/service   ──ogen──▶ gen/service     (Go server, internal listener only, with the `staff` license)
  openapi/telemetry ──ogen──▶ gen/telemetry   (Go client only, `ogen.telemetry.yml`; embedded in `spec.go`, checked by `internal/telemetryschema`)
src/routes/**       ──tsr generate──▶ src/routeTree.gen.ts   (TanStack Router file-based route tree)
ent/schema/*.go     ──entc──▶ ent/*           (Go ORM)
ent/schema/*.go + ent/template/scoped*.tmpl  ──entc──▶ ent/scoped.go, ent/scoped_registry.go  (scoped client: `client.Scoped(ws)`)
ent + gen/{site,external,staff,staffee,service}  ──goverter──▶ internal/api/{site,external,staff}/resources/converter_gen.go, ee/staff/resources/{staff,service}_converter_gen.go
gen/{site,external} request bodies  ──goverter──▶ internal/api/{site,external}/resources/inputs_gen.go  (`Inputs`: bodies to module inputs, updates as internal/patch fields)
packages/emaildoc (pinned @react-email/editor) ──esbuild──▶ internal/emaildoc/assets/{emaildoc.js,schema.json}  (`mise run build:emaildoc`; not part of `generate`)
openapi/service (`x-required-scopes` per operation, from `@requiresServiceScopes`) ──cmd/scopegen──▶ ee/staff/service_scopes_gen.go (`ServiceOperationScopes`)
openapi/external (`x-required-scopes` per operation, from `@requiresScopes`) ──cmd/scopegen──▶ internal/api/auth/external_scopes_gen.go (`ExternalOperationScopes`)
openapi/external (`x-required-permissions` from `@requiresPermissions`, optional per operation: what a Connection's User must hold) ──cmd/scopegen──▶ internal/api/auth/external_permissions_gen.go (`ExternalOperationPermissions`)
openapi/site (`x-required-permissions`, `x-allowed-while-withheld`, `x-allowed-while-acceptance-pending` per operation, from `@requiresPermissions`, `@allowedWhileWithheld`, `@allowedWhileAcceptancePending`) ──cmd/scopegen──▶ internal/api/site/access_gen.go (`operationAccess`)
openapi/site (`x-grant-permissions` on each grantable scope enum, from `@grantable` and `@grantedWith`) ──cmd/scopegen──▶ internal/accounts/grant_permissions_gen.go (`GrantPermissions`)
openapi/site (`Permission` enum) ──cmd/scopegen──▶ internal/accounts/permissions_gen.go (`Permission` and its `Perm…` constants)
openapi/external (`ApiTokenScope` enum) ──cmd/scopegen──▶ internal/apitokens/scopes_gen.go (`Scope`, its constants and the `Scopes` vocabulary); the files are `scopegen.Tables`
fixtures/*.yml (`# fixture: Name` rows)  ──cmd/fixturegen──▶ internal/fixtures/catalog_gen.go (named test constants)
Go interfaces (`mise run generate:mocks` task list)  ──moq──▶ <pkg>/mocks_gen_test.go (test mocks; add an interface to the task list)
```

- **goverter** also maps request bodies → module inputs (`Inputs` in each surface's
  `resources/inputs.go`): an update becomes the module's patch (`internal/patch`: an absent key
  keeps, null clears, a value sets). Its extends only bind this surface's Opt types to the shared
  helpers of `internal/convert`.
- **goverter** maps ent entities → ogen resource DTOs. The `Converter` interface and its
  `// goverter:extend` helpers (ids, timestamps, optionals) are hand-written in
  each package's `resources.go`; the impl (`ConverterImpl`) is generated. Adding a DTO
  field that has no source mapping fails generation — that completeness check is the point.

- **Contract decorators** (ADR 0069): every `x-*` extension a Go reader depends on is written by
  a typed decorator, never by a raw `@extension`, which `tsp compile` refuses for these keys.

  | Decorator                                                                  | Extension                | Declared in                      |
  | -------------------------------------------------------------------------- | ------------------------ | -------------------------------- |
  | `@requiresPermissions(#[Permission.…])`                                    | `x-required-permissions` | `common/permissions.tsp`         |
  | `@requiresScopes(#[ApiTokenScope.…])`                                      | `x-required-scopes`      | `common/scopes.tsp`              |
  | `@requiresServiceScopes(#[ServiceScope.…])`                                | `x-required-scopes`      | `service/resources/identity.tsp` |
  | `@grantable` on an enum, `@grantedWith(Permission.…)` on a member          | `x-grant-permissions`    | `common/scopes.tsp`              |
  | `@allowedWhileWithheld`, `@allowedWhileAcceptancePending`                  | `x-allowed-while-…`      | `lib/main.tsp`                   |
  | `@untrusted`                                                               | `x-untrusted`            | `lib/main.tsp`                   |
  | `@mcp(#{ hidden?, name?, send? })`                                         | `x-mcp`                  | `lib/main.tsp`                   |
  | `@everyOperationDeclares("scopes" \| "workspacePermissions")` on a surface | (a check)                | `lib/main.tsp`                   |

  A decorator typed by a contract enum is declared beside it; `lib/decorators.js` implements them
  all and checks, in `$onValidate`, what each surface's operations must declare (the checks
  `cmd/scopegen` repeats on the OpenAPI document). The OpenAPI output is what a raw `@extension`
  wrote. A new extension a Go reader depends on gets a decorator here, a fixture under
  `typespec/lib/testdata/` (`// expect-next-line: <code>` marks a statement that must fail) and a
  case in `decorators.test.ts`; `mise run test:typespec` runs them.

- `mise run generate` — full cycle: typespec → openapi → backend (ent + ogen) → fixture catalog → frontend → i18n types → format.
- `mise run generate:typespec` / `mise run generate:backend` / `mise run generate:openapi` — partial regens.
- The README claims echo + oapi-codegen; that is **outdated**. The HTTP stack is **ogen**
  (`gen/*` are ogen servers, wired in `internal/server/server.go`).

### What is generated vs hand-written

Generated files carry a header (`Code generated … DO NOT EDIT`, or `// @ts-nocheck` on the
frontend) and are flagged `linguist-generated` in `.gitattributes` (GitHub collapses them in
diffs). Generated code lives in dedicated dirs — `internal/` is otherwise hand-written, its
only generated files being the three `converter_gen.go`, the two `inputs_gen.go`, `ee/staff/resources/{staff,service}_converter_gen.go`, `ee/staff/service_scopes_gen.go`, `internal/api/site/access_gen.go`, `internal/api/auth/external_{scopes,permissions}_gen.go`, `internal/accounts/{permissions,grant_permissions}_gen.go`, `internal/apitokens/scopes_gen.go`, `internal/fixtures/catalog_gen.go`, the moq `*_gen_test.go` mocks and `internal/emaildoc/assets/`.
