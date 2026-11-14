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
ent/schema/*.go + ent/template/*.tmpl  ──entc + internal/entgen──▶ ent/scoped*.go (scoped client: `client.Scoped(ws)`), ent/kinds.go (the manifest of kinds), ent/roles.go (the manifest of roles), ent/stateform.go, ent/revision.go
ent + gen/{site,external,staff,staffee,service}  ──goverter (cmd/goverter)──▶ internal/api/{site,external,staff}/resources/converter_gen.go, ee/api/staff/resources/{staff,service}_converter_gen.go
ent + liveuse + gen/* (what a refusal names, the site's problem to every surface's)  ──goverter──▶ internal/api/problem/mapping/converter_gen.go  (rendered by internal/api/problem)
gen/{site,external,staff} request bodies  ──goverter──▶ internal/api/{site,external,staff}/resources/inputs_gen.go  (`Inputs`: bodies to module inputs, updates as internal/kernel/patch fields)
gen/service request bodies  ──goverter──▶ ee/api/staff/resources/service_inputs_gen.go  (`ServiceInputs`; no gen/staffee body is a module input)
packages/emaildoc (pinned @react-email/editor) ──esbuild──▶ internal/kernel/emaildoc/assets/{emaildoc.js,schema.json}  (`mise run build:emaildoc`; not part of `generate`)
openapi/service (`x-required-scopes` per operation, from `@requiresServiceScopes`) ──cmd/scopegen──▶ ee/api/staff/service_scopes_gen.go (`ServiceOperationScopes`)
openapi/external (`x-required-scopes` per operation, from `@requiresScopes`) ──cmd/scopegen──▶ internal/api/auth/external_scopes_gen.go (`ExternalOperationScopes`)
openapi/external (`x-required-permissions` from `@requiresPermissions`, optional per operation: what a Connection's User must hold) ──cmd/scopegen──▶ internal/api/auth/external_permissions_gen.go (`ExternalOperationPermissions`)
openapi/site (`x-required-permissions`, `x-allowed-while-withheld`, `x-allowed-while-acceptance-pending` per operation, from `@requiresPermissions`, `@allowedWhileWithheld`, `@allowedWhileAcceptancePending`) ──cmd/scopegen──▶ internal/api/site/access_gen.go (`operationAccess`)
openapi/site (`x-grant-permissions` on each grantable scope enum, from `@grantable` and `@grantedWith`) ──cmd/scopegen──▶ internal/domain/identity/memberships/grant_permissions_gen.go (`GrantPermissions`)
openapi/site (`Permission` enum) ──cmd/scopegen──▶ internal/domain/identity/memberships/permissions_gen.go (`Permission` and its `Perm…` constants)
openapi/external (`ApiTokenScope` enum) ──cmd/scopegen──▶ internal/domain/identity/apitokens/scopes_gen.go (`Scope`, its constants and the `Scopes` vocabulary)
openapi/service (`ServiceScope` enum) ──cmd/scopegen──▶ ee/domain/identity/servicecredentials/scopes_gen.go (`Scope`, its constants and the `Scopes` vocabulary, so the Service credentials module never imports gen/service)
openapi/site (`GrantableScope`, `SendScope`, `SetupScope`, `PersonalScope` enums) ──cmd/scopegen──▶ internal/kernel/tokenscope/user_scopes_gen.go (each type, its constants and its list, so a core package never imports gen/site for them)
openapi/* (the request fields: every request body's properties and every operation's parameters) ──cmd/scopegen──▶ internal/kernel/fieldname/fieldnames_gen.go (a constant per json name, which a refusal of a field is reported under; ast-grep `refused-field-is-a-constant`)
openapi/{site,staff} (`x-rate-limit` per operation, from `@rateLimited`) ──cmd/scopegen──▶ internal/infra/ratelimit/{site,staff}_limits_gen.go (the ServeMux pattern of each limited operation and its `RouteLimit`)
openapi/site (`SystemEventAction` enum) ──cmd/scopegen──▶ internal/kernel/systemevent/actions_gen.go (an untyped constant per System event action and the `Actions` list, which the bus registry and the ent schema's retention predicate name); the files are `scopegen.Tables`
openapi/{site,external,service,collect} (the value bounds `boundgen.Bounds` lists: lengths, ranges, item counts, patterns, defaults) ──cmd/boundgen──▶ internal/kernel/bounds/bounds_gen.go (a constant per bound and the owner that checks it, ADR 0107) and packages/analytics/src/bounds.gen.ts (the bounds the tracker owns, which cannot import zod)
fixtures/*.yml (`# fixture: Name` rows)  ──cmd/fixturegen──▶ internal/fixtures/catalog_gen.go (named test constants)
internal/kernel/i18n/locales/en.json  ──cmd/i18ngen──▶ internal/kernel/i18n/ids_gen.go (one i18n.ID per message: a call can only name a message the catalog has)
config (the `environment` struct: `env`, `envDefault` and `validate` tags, profile defaults, field doc comments)  ──cmd/configdoc──▶ docs/configuration.md (the configuration reference the docs site copies)
Go interfaces (`mise run generate:mocks` task list)  ──moq──▶ <pkg>/mocks_gen_test.go (test mocks; add an interface to the task list)
```

- **`internal/entgen` holds the rules of ent generation.** `ent/entc.go` is a thin main that runs
  `entgen.Hook` on the graph it generates: the hook classifies every entity once, by the marker each
  mixin declares in its `Annotations()` (`ent/schema/kinds.go`: Workspace-owned, archivable,
  discardable, Versioned, with a revision) and by its structure (a singleton is a whole unique index
  on `workspace_id` alone), decodes each annotation into its `ent/schema` type once, and checks the
  graph against every rule, failing the generation with all the problems at once; then it hands the
  templates of `ent/template` the classification's funcs, which have no side effects. A kind is never
  found by a column name. It does not import the generated `ent` package, so generation never depends
  on its previous output compiling. A new rule gets a schema under `internal/entgen/testdata/` that
  breaks it and a row in `TestCheckFindsEveryProblem`; `TestTheSchemaBreaksNoRule` holds the real
  schema to all of them.
- **The scoped client keeps a write as a recipe** (`ent/template/scoped.tmpl`, ADR 0017). A wrapper
  (`<N>ScopedCreate`, `<N>ScopedUpdateOne`, `<N>ScopedDelete`, …) holds its predicates and what its
  setters set, and builds the ent builder only when the write runs, on the client of the scope it runs
  on, so that client's hooks apply from the start. Every write of an entity goes through its one
  `write<N>`: for an audited entity (ADR 0022) under an audited scope it joins or opens the
  transaction that carries the entries, reads the rows before, runs the write on that transaction's
  client, reads them after and records one entry per row whose audited fields changed (every row, for
  a Versioned definition's move: an edit of its draft changes only the version rows). A template never
  touches ent's unexported fields: an ent upgrade can neither break the build there nor move a write
  out of its transaction (`TestAnAuditedWriteRunsInTheTransactionItsOpenerOpens` commits on a real
  database to catch it, for the create, updates and deletes of the registry). The verbs of an entity's writes (create, update, delete, then discard, or
  archive and unarchive, by its kind) are `entgen.Verbs`: the wrappers and the Versioned definitions'
  pointers name a verb through the `verb` func, which fails the generation on one the entity has not,
  and the registry's `AuditActions` lists them, which the contract's `AuditAction` enum must match.
- **The manifest of kinds** (`ent/kinds.go`, `ent.Kinds`): per entity, its table and whether it is
  Workspace-owned, archivable, discardable, Versioned, with a revision and a singleton. The
  migration generator reads the Workspace-owned tables from it (`internal/infra/devdb/tenantkeys.go`),
  the Revision hook the types with a revision (`internal/app/entclient`), and the scoped client's
  registry (`ent.ScopedEntities`) carries each entity's kind and wrapper type, which the tests of
  `internal/scopedclient` choose their entities by.
- **The manifest of roles** (constants of `ent`, read by the analyzers of `tools/analyzers` through
  `go/types`): `TypeOf_<Type>` and `EntityOf_<Type>` for every exported type (what it is and the
  entity it belongs to), `RoleOf_<Type>_<Method>` (`RoleRead`, `RoleStart`, `RoleStep` or `RoleNone`)
  for every exported method a template emits and for the reads and writes ent's own methods start, and
  `SetsState_`/`ClearsState_<Type>_<Method>` for the update and upsert methods that write a machine or
  latch field. `ent/template/roles.tmpl` declares ent's own types (`ent/roles.go`) and `stateform.tmpl`
  their state writes; every other template declares its constants beside the type or method it emits.
  The roles of ent's own methods are the list `roles.tmpl` keeps of the reads and writes ent generates
  (the client's `Create`…`DeleteOneID`, `Query`, `Get`, an edge's `Query<Edge>`, an entity's `Update`, a
  create's `OnConflict`): after an ent upgrade that adds such a method, add it there, since the test
  checks only that each listed one exists. A template that adds an exported method adds its role there: `ent/schema/roles_test.go` fails on a
  method without one, on an entry that names nothing, and on the analyzers' stand-ins of `ent` that break
  the same rules.
- **goverter** also maps request bodies → module inputs (`Inputs` in each surface's
  `resources/inputs.go`): an update becomes the module's patch (`internal/kernel/patch`: an absent key
  keeps, null clears, a value sets). Its extends only bind this surface's Opt types to the shared
  helpers of `internal/kernel/convert`.
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
  handler. The extends' bodies are the generic helpers of `internal/kernel/convert` (`Opt`, `Nil`, `OptVia`, …),
  which hold the absent-or-null rule once for every surface.
- **Enums go member by member.** `mise run generate:backend` runs our build of goverter
  (`cmd/goverter`), which adds the `value` transformer (`internal/goverterenum`): members match by
  value, as ent and the contracts name them differently. Each pair of enums is a method in the
  converter's Enums block, `// goverter:enum:transform value`; goverter then refuses a member with
  no counterpart until the method maps it (`goverter:enum:map DecisionShown @panic`). A nested enum
  field never takes a method's own transform, and goverter casts an enum whose package the
  converter's file does not import (it sees the package through ent's export data, without its
  constants), so the method is what makes the check happen. `TestAPIConvertsNoEnumByCast`
  (`internal/goverterenum`) fails on any enum-to-enum cast left in a converter or handler package or
  in `internal/api/problem` (the refusals), generated or hand-written; an optional enum goes through `convert.OptVia` (response) or
  `convert.PtrVia` (request) and the enum method.

- **An error is the `default` response, never a status of its own.** Every operation of a
  surface returns its success type `| Problem` (`common/errors.tsp`): one `@error` model with no
  status code, which the OpenAPI emitter writes as the operation's `default` response, its body the
  one `ProblemDetails` (with `usedBy` for a refused removal and `workspaces` for a refused User
  deletion). ogen runs with `convenient_errors: on` (`ogen.yml`), so it reduces that response to one
  error type per surface (`*ProblemDetailsStatusCode`): a handler returns `(*T, error)` (or `error`
  when it answers no body), the Handler's `NewError` turns the error into the problem through
  `internal/api/problem` (`problem.Site`, `problem.External`, …), and a client returns the success type
  or that error (`testhelper.SiteRefusal` and its kin read it in a test). An operation without the
  `default` response fails the generation (and `check:openapi`, rule `operation-declares-problem`);
  a status code listed beside it would bring a per-operation sum type back.

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
  | `@rateLimited("login" \| "forgot_password" \| "human")` on an operation    | `x-rate-limit`           | `lib/main.tsp`                   |
  | `@untrusted`                                                               | `x-untrusted`            | `lib/main.tsp`                   |
  | `@mcp(#{ hidden?, name?, send? })`                                         | `x-mcp`                  | `lib/main.tsp`                   |
  | `@everyOperationDeclares("scopes" \| "workspacePermissions")` on a surface | (a check)                | `lib/main.tsp`                   |

  A decorator typed by a contract enum is declared beside it; `lib/decorators.ts` implements them
  all and checks, in `$onValidate`, what each surface's operations must declare (the checks
  `cmd/scopegen` repeats on the OpenAPI document). The OpenAPI output is what a raw `@extension`
  wrote. A new extension a Go reader depends on gets a decorator here, a fixture under
  `typespec/lib/testdata/` (`// expect-next-line: <code>` marks a statement that must fail) and a
  case in `decorators.test.ts`; `mise run test:typespec` runs them.

- **Descriptions say what happens, not which code does it.** `$onValidate` also refuses a
  description (a doc comment, `@doc`, `@summary`, `@returns`/`@returnsDoc`, `@errors`/`@errorsDoc`)
  that names the code behind the contract: the words module, package or use case, a Go identifier
  such as `emailcase.Of`, a Go path or file (`internal/…`, `lookup.go`). Such a
  description goes stale when the code moves; it says what happens to the value instead, in the
  words of `GLOSSARY.md`. Its fixture is `typespec/lib/testdata/descriptions.tsp`.

- `mise run generate` — full cycle: typespec → openapi → backend (ent + ogen) → fixture catalog → configuration reference → frontend → i18n types → format.
- `mise run generate:typespec` / `mise run generate:backend` / `mise run generate:openapi` — partial regens.

### What is generated vs hand-written

The generated files are the ones [`.gitattributes`](../../.gitattributes) marks
`linguist-generated` (GitHub collapses them in diffs and leaves them out of the language stats).
All but the OpenAPI documents and `internal/kernel/emaildoc/assets/` carry a header (`// Code generated …
DO NOT EDIT.`, or `// @ts-nocheck` on the frontend).
`check:generated` (`mise/scripts/check-generated.ts`, part of `mise run check`) fails on a file with
the header that no pattern marks and on a pattern that matches no file. Generated code lives in
dedicated dirs (`gen/`, `ent/`, `openapi/`, the `generated/` dirs); `internal/` and `ee/` are
hand-written but for the `*_gen.go` and `*_gen_test.go` files, `internal/kernel/emaildoc/assets/`,
and the tracker bundle `internal/server/assets/t.js` (`mise run build:tracker`).
