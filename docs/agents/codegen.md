# Codegen pipeline

The diagram, goverter and the generated-versus-hand-written rules. The list of files never to
edit by hand is in `AGENTS.md`.

```
typespec/{site,external,collect,staff,service,telemetry}   ──tsp compile──▶  openapi/*.openapi.json
  openapi/site      ──ogen──▶ gen/site       (Go server)   ──openapi-ts──▶ src/generated/site (TS client + react-query + zod)
  openapi/external  ──ogen──▶ gen/external    (Go server)  ──openapi-ts──▶ packages/node/src/generated/external (TS client + SDK, `@sphericon/node`, private)
  openapi/collect   ──ogen──▶ gen/collect     (Go server)  ──openapi-ts──▶ packages/analytics/src/generated/collect (types only)
  openapi/staff  ──ogen──▶ gen/staff    (Go server)  ──openapi-ts──▶ src/generated/staff (TS client + react-query + zod)
  openapi/service   ──ogen──▶ gen/service     (Go server, internal listener only, with the `staff` license)
  openapi/telemetry ──ogen──▶ gen/telemetry   (Go client only, `ogen.telemetry.yml`; embedded in `spec.go`, checked by `internal/telemetryschema`)
src/routes/**       ──tsr generate──▶ src/routeTree.gen.ts   (TanStack Router file-based route tree)
ent/schema/*.go     ──entc──▶ ent/*           (Go ORM)
ent/schema/*.go + ent/template/scoped*.tmpl  ──entc──▶ ent/scoped.go, ent/scoped_registry.go  (scoped client: `client.Scoped(ws)`)
ent + gen/{site,external,staff,service}  ──goverter──▶ internal/api/{site,external}/resources/converter_gen.go, ee/staff/resources/{staff,service}_converter_gen.go
packages/emaildoc (pinned @react-email/editor) ──esbuild──▶ internal/emaildoc/assets/{emaildoc.js,schema.json}  (`mise run build:emaildoc`; not part of `generate`)
openapi/service (`x-required-scopes` per operation, declared in TypeSpec) ──cmd/scopegen──▶ ee/staff/service_scopes_gen.go (`ServiceOperationScopes`)
openapi/site (`x-required-permissions`, `x-allowed-while-withheld` per operation) ──cmd/scopegen──▶ internal/api/site/access_gen.go (`operationAccess`); the tables are `scopegen.Tables`
fixtures/*.yml (`# fixture: Name` rows)  ──cmd/fixturegen──▶ internal/fixtures/catalog_gen.go (named test constants)
Go interfaces (`mise run generate:mocks` task list)  ──moq──▶ <pkg>/mocks_gen_test.go (test mocks; add an interface to the task list)
```

- **goverter** maps ent entities → ogen resource DTOs. The `Converter` interface and its
  `// goverter:extend` helpers (ids, timestamps, optionals) are hand-written in
  each package's `resources.go`; the impl (`ConverterImpl`) is generated. Adding a DTO
  field that has no source mapping fails generation — that completeness check is the point.

- `mise run generate` — full cycle: typespec → openapi → backend (ent + ogen) → fixture catalog → frontend → i18n types → format.
- `mise run generate:typespec` / `mise run generate:backend` / `mise run generate:openapi` — partial regens.
- The README claims echo + oapi-codegen; that is **outdated**. The HTTP stack is **ogen**
  (`gen/*` are ogen servers, wired in `internal/server/server.go`).

### What is generated vs hand-written

Generated files carry a header (`Code generated … DO NOT EDIT`, or `// @ts-nocheck` on the
frontend) and are flagged `linguist-generated` in `.gitattributes` (GitHub collapses them in
diffs). Generated code lives in dedicated dirs — `internal/` is otherwise hand-written, its
only generated files being the two `converter_gen.go`, `ee/staff/resources/{staff,service}_converter_gen.go`, `ee/staff/service_scopes_gen.go`, `internal/api/site/access_gen.go`, `internal/fixtures/catalog_gen.go`, the moq `*_gen_test.go` mocks and `internal/emaildoc/assets/`.
