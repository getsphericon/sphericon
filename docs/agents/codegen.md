# Codegen pipeline

The diagram, goverter and the generated-versus-hand-written rules. The list of generated files,
never edited by hand, is [`.gitattributes`](../../.gitattributes) (`linguist-generated`).

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
ent + gen/{site,external,staff,staffee,service}  ──goverter (cmd/goverter)──▶ internal/api/{site,external,staff}/resources/converter_gen.go, ee/staff/resources/{staff,service}_converter_gen.go
gen/{site,external,staff} request bodies  ──goverter──▶ internal/api/{site,external,staff}/resources/inputs_gen.go  (`Inputs`: bodies to module inputs, updates as internal/patch fields)
gen/service request bodies  ──goverter──▶ ee/staff/resources/service_inputs_gen.go  (`ServiceInputs`; no gen/staffee body is a module input)
packages/emaildoc (pinned @react-email/editor) ──esbuild──▶ internal/emaildoc/assets/{emaildoc.js,schema.json}  (`mise run build:emaildoc`; not part of `generate`)
openapi/service (`x-required-scopes` per operation, from `@requiresServiceScopes`) ──cmd/scopegen──▶ ee/staff/service_scopes_gen.go (`ServiceOperationScopes`)
openapi/external (`x-required-scopes` per operation, from `@requiresScopes`) ──cmd/scopegen──▶ internal/api/auth/external_scopes_gen.go (`ExternalOperationScopes`)
openapi/external (`x-required-permissions` from `@requiresPermissions`, optional per operation: what a Connection's User must hold) ──cmd/scopegen──▶ internal/api/auth/external_permissions_gen.go (`ExternalOperationPermissions`)
openapi/site (`x-required-permissions`, `x-allowed-while-withheld`, `x-allowed-while-acceptance-pending` per operation, from `@requiresPermissions`, `@allowedWhileWithheld`, `@allowedWhileAcceptancePending`) ──cmd/scopegen──▶ internal/api/site/access_gen.go (`operationAccess`)
openapi/site (`x-grant-permissions` on each grantable scope enum, from `@grantable` and `@grantedWith`) ──cmd/scopegen──▶ internal/accounts/grant_permissions_gen.go (`GrantPermissions`)
openapi/site (`Permission` enum) ──cmd/scopegen──▶ internal/accounts/permissions_gen.go (`Permission` and its `Perm…` constants)
openapi/external (`ApiTokenScope` enum) ──cmd/scopegen──▶ internal/apitokens/scopes_gen.go (`Scope`, its constants and the `Scopes` vocabulary); the files are `scopegen.Tables`
fixtures/*.yml (`# fixture: Name` rows)  ──cmd/fixturegen──▶ internal/fixtures/catalog_gen.go (named test constants)
internal/i18n/locales/en.json  ──cmd/i18ngen──▶ internal/i18n/ids_gen.go (one i18n.ID per message: a call can only name a message the catalog has)
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
  A list of resources is a list method of the `Converter` (`ContactsToResources(source
  []*ent.Contact) []…`) marked `// goverter:useEmptyOnNil:slice`, so no rows answer `[]`, never
  null; a handler does not wrap the single-item method in `lo.Map` or a loop (ast-grep rule
  `no-hand-mapped-list`). A paged list response is a page method of the `Converter` from the
  module's page (`ContactsListPage(source pagination.Page[*ent.Contact]) *…ContactsListOK`, also
  `useEmptyOnNil:slice`), named after the response, so the handler returns it and writes no
  envelope (ast-grep rule `no-hand-paged-response`). A feed read after a cursor is the same: the
  module builds a `pagination.CursorPage` with `pagination.Cursor(rows, limit, cursor, cursorOf)`
  from the limit+1 rows it read, and the page method (`UserFeedPage(source
  pagination.CursorPage[…]) *…UserFeedPage`) maps its items, `NextCursor` and `HasMore`; the rule
  also refuses a literal with a `NextCursor:` or `HasMore:` key. Items the handler maps itself (an
  extra per item) go through `pagination.Map`/`MapErr`/`MapCursor` first and the method, taking a
  page of resources, is marked `goverter:skipCopySameType`; an unpaginated list is
  `pagination.Whole(items)`. `annotate:unmapped` writes a
  `// Field: ignore` line into the generated code for each field the converter leaves to the
  handler. The extends' bodies are the generic helpers of `internal/convert` (`Opt`, `Nil`, `OptVia`, …),
  which hold the absent-or-null rule once for every surface.
- **Enums go member by member.** `mise run generate:backend` runs our build of goverter
  (`cmd/goverter`), which adds the `value` transformer (`internal/goverterenum`): members match by
  value, as ent and the contracts name them differently. Each pair of enums is a method in the
  converter's Enums block, `// goverter:enum:transform value`; goverter then refuses a member with
  no counterpart until the method maps it (`goverter:enum:map DecisionShown @panic`). A nested enum
  field never takes a method's own transform, and goverter casts an enum whose package the
  converter's file does not import (it sees the package through ent's export data, without its
  constants), so the method is what makes the check happen. `TestAPIConvertsNoEnumByCast`
  (`internal/goverterenum`) fails on any enum-to-enum cast left in a converter or handler package,
  generated or hand-written; an optional enum goes through `convert.OptVia` (response) or
  `convert.PtrVia` (request) and the enum method.

- **An opaque JSON object is a shared model, never an inline `Record<unknown>`.** A field whose
  members the contract leaves open is `JsonObject` (custom fields, properties, params, a diff) or
  `EditorDocument` (the email editor's document), both in `common/scalars.tsp`. ogen then emits one
  Go type per surface (`JsonObject`, `OptNilJsonObject`, …), so one converter extend per type
  serves every field; an inline record would get a type, and an extend, of its own per field.

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

### What is generated vs hand-written

The generated files are the ones [`.gitattributes`](../../.gitattributes) marks
`linguist-generated` (GitHub collapses them in diffs and leaves them out of the language stats).
All but the OpenAPI documents and `internal/emaildoc/assets/` carry a header (`// Code generated …
DO NOT EDIT.`, or `// @ts-nocheck` on the frontend).
`check:generated` (`mise/scripts/check-generated.ts`, part of `mise run check`) fails on a file with
the header that no pattern marks and on a pattern that matches no file. Generated code lives in
dedicated dirs (`gen/`, `ent/`, `openapi/`, the `generated/` dirs); `internal/` and `ee/` are
hand-written but for the `*_gen.go` and `*_gen_test.go` files and `internal/emaildoc/assets/`.
