# Server SDK generators for languages beyond TypeScript

Researched October 2026 for ticket #573 (spec #568). Primary sources only: each tool's own repository
(shallow clone of the default branch, read, never run from the clone), its own docs tables and
licence file, the package registry entry of the tool, and trial generations from this repo's
`openapi/external.openapi.json` at `da081ae0` (the `integration/568` tip, where the `openapi-ts` SDK
`@sphericon/node` of #570 lives). No SDK code is added to the repo; trial output stayed in a scratch
directory.

| Tool                                     | Branch | Head at research time                      | Latest release                     |
| ---------------------------------------- | ------ | ------------------------------------------ | ---------------------------------- |
| OpenAPITools/openapi-generator           | master | `a03c03d2ace1c04cd0a141d457ff4f5716743b26` | v7.26.0 (2026-10-06)               |
| microsoft/kiota                          | main   | `31c61e46aa65d9152e2f03426793d4705aceccfd` | v1.36.1 (2026-10-12)               |
| openapi-generators/openapi-python-client | main   | `0a283fe21eeaa23046faf622d4ff4b2c826d2edb` | v0.29.1 (2026-08-30)               |
| ogen-go/ogen                             | main   | `d84283b7ff30f4f596ef1c848b775f3c918bcfca` | v1.24.0 (2026-08-07)               |
| oapi-codegen/oapi-codegen                | main   | `4153af7f17c7cd1760be30390eb25d00cc8980b7` | v2.8.0 (2026-07-17)                |
| fern-api/fern                            | main   | `c3087793bc8df3298a23edb5e90303bca4741360` | 5.23.3 (2026-05-12)                |
| speakeasy-api/speakeasy                  | main   | `2da9f2bfbe391b5c38d587bce03e9fd8b995ef3b` | v1.803.1 (2026-10-21)              |
| swagger-api/swagger-codegen              | master | `e27994e655f7881b5466ed2c41b734293a5b9663` | v2.4.53 (2026-10-22)               |
| oxidecomputer/progenitor                 | main   | `af512986d0ca96984e550691ab9813c501e93f9c` | none published as a GitHub release |
| stainless-api/stainless-api-cli          | main   | `7840ca61ce56118a1e3ccf563a6403b701b16694` | v0.1.0-alpha.79                    |

Licence, head and release come from the GitHub repository metadata (`api.github.com/repos/...`) and
were cross-read against each repository's `LICENSE` file where noted below.

## Question

Which generator should produce server SDKs from `openapi/external` for languages other than
TypeScript, and what follow-up tickets does that imply? Facts first; the recommendation closes the note.

## The input contract

`openapi/external.openapi.json` is OpenAPI 3.0.0, 78 paths, 104 operations, 161 component schemas, two
servers (`http://localhost:3000/api`, `https://api.getsphericon.app`), bearer-token auth. It has three
`oneOf` and three `discriminator` occurrences (the Integration config and similar unions), and five
`GET /export/*` operations (audit, contacts, erasures, events, messages) whose 200 response is
`application/x-ndjson` with an `X-Next-Cursor` header. Two schemas collide with common enum or
identifier habits: `EntityId` is a string with `format: int64`, and `AutomationStep` / `RuleAction`
carry a boolean `hasSecret`. These are the places where generators differ below.

## Candidates: licence, languages, maturity

### OpenAPI Generator (OpenAPITools/openapi-generator)

- Licence: Apache-2.0 (`LICENSE`). Community fork of Swagger Codegen (README, "OpenAPI Generator is a
  fork of Swagger Codegen").
- Languages: 175 generator pages under `docs/generators/`, covering client, server, schema and doc
  generators. Each page carries a `generator stability` row. Read from the pages: `python`, `go`, `java`,
  `csharp`, `kotlin`, `swift6`, `ruby`, `php`, `dart`, `rust` and `typescript-fetch` are STABLE;
  `php-nextgen` and `ruby-nextgen` are BETA.
- Spec compatibility (README): 1.0, 1.1, 1.2, 2.0, 3.0, 3.1 (3.1 "beta support").
- Runs as a JVM jar (Maven Central `openapi-generator-cli`, Java 11 minimum per README).
- Style: one generic engine with Mustache templates per language; the client is a flat set of
  tag-grouped API classes, per-operation methods.

### Kiota (microsoft/kiota)

- Licence: MIT (`LICENSE`).
- Languages (README "Supported languages" table, which marks each generator and its runtime libraries):
  C#, Go, Java, PHP and Python are generated and have abstractions, serialization and HTTP libraries
  (checkmarks); Ruby, TypeScript/JavaScript and Dart are in preview (the tools emoji).
  Every language needs Kiota's own small runtime packages (`kiota-abstractions-*`, `kiota-http-*`,
  `kiota-serialization-*`) as dependencies, hosted in separate Microsoft repositories.
- Spec: built on Microsoft.OpenAPI.NET; ships as a .NET tool, a container
  (`mcr.microsoft.com/openapi/kiota`) and binaries.
- Style: a fluent request-builder tree that mirrors the URL path, one builder class per path segment.

### openapi-python-client (openapi-generators/openapi-python-client)

- Licence: MIT (README badge; repository metadata).
- Language: Python only. "Generate modern Python clients from OpenAPI 3.0 and 3.1 documents" and
  "still in development and does not support all OpenAPI features" (README).
- Needs `ruff` on the PATH to format its output (README installation note).

### ogen (ogen-go/ogen) and oapi-codegen (oapi-codegen/oapi-codegen)

- Licence: Apache-2.0 both. Go only.
- ogen is already this repo's server generator: `mise/tasks.toml` runs
  `go tool ogen --target gen/external --package externalapi --clean openapi/external.openapi.json`, and
  ogen emits the client with the server. `gen/external/oas_client_gen.go` (15,067 lines) already holds a
  typed `Client` with one method per operation (for example `ContactsCreate`, `ContactsBatchUpsert`),
  compiled and exercised by this repo's own tests.

### Hosted or commercial generators

- Fern (`fern-api/fern`, Apache-2.0): the repository README opens with "This open-source project is no
  longer maintained" and points to the hosted product. Its generator directories (`generators/go`,
  `python`, `java`, `csharp`, `php`, `ruby`, `rust`, `swift`, `typescript`) exist, but
  the README says `fern generate --local` runs only legacy generator groups and "SDK Config targets use
  remote generation".
- Speakeasy (`speakeasy-api/speakeasy`): the CLI repository is under the Elastic License 2.0
  (`LICENSE`: no hosted or managed service, no circumventing licence-key functionality). The README
  lists ten languages and points to Speakeasy's own docs for usage; this repository holds the CLI, not
  the SDK templates, so output licence and generator source could not be checked from the repository.
- Stainless (`stainless-api/stainless-api-cli`, Apache-2.0): the repository is the CLI of the hosted
  Stainless REST API and says "The CLI is unstable and the API may change". It is not a generator.

These three are outside the open-core constraint (generation must be reproducible by a contributor
with `mise run generate`, per #568 user stories 22-23) and were not tried.

### Others seen, not tried

- Swagger Codegen (`swagger-api/swagger-codegen`, Apache-2.0, v2.4.53): the project OpenAPI Generator
  forked from; README of OpenAPI Generator names it as such. Not a better choice than its fork for new
  work; not trialled.
- progenitor (`oxidecomputer/progenitor`, MPL-2.0): Rust only, no GitHub release published; the Rust
  stable alternative inside OpenAPI Generator is `rust`. Not trialled (no Rust requirement yet).
- AutoRest (`Azure/autorest`, MIT): last GitHub release is 2.0.4222 dated 2017-12-12; the repository
  is still pushed to, but it is Azure-SDK oriented. Not trialled.

## Trial generations from `openapi/external`

All runs on macOS arm64 with Java 25 (OpenAPI Generator jar 7.26.0), Docker 29.9.2 (Kiota 1.36.1 image
`mcr.microsoft.com/openapi/kiota:latest`), Python 3.15.1 and Go 1.27.3. The input was a copy of
`openapi/external.openapi.json`.

### OpenAPI Generator 7.26.0

| Language         | Command                                                                           | Result                                                                                                                                                                                                                                       |
| ---------------- | --------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| python           | `generate -g python --additional-properties=packageName=sphericon_client`         | exit 0, 196 `.py` files (49.8 k lines without tests); `pip install .` and `import sphericon_client` succeed; one API class per tag (`ContactsApi` ...), per-operation methods with `_with_http_info` and `_without_preload_content` variants |
| go               | `generate -g go --additional-properties=packageName=sphericon,isGoSubmodule=true` | exit 0, 192 `.go` files (62.6 k lines); **does not compile**: enum constants of different types redeclare each other (`ACTIVE`, `FAILED`, `CONTACTS_READ`, `DRAFT`, ...)                                                                     |
| go (retry)       | same plus `enumClassPrefix=true`                                                  | exit 0; still **does not compile**: `AutomationStep` and `RuleAction` have a field and a method both named `HasSecret`                                                                                                                       |
| typescript-fetch | `generate -g typescript-fetch`                                                    | exit 0, 192 `.ts` files (23.7 k lines); not compiled, trialled only for size comparison with the `openapi-ts` SDK                                                                                                                            |

Warnings in the Python run: "Failed to get the schema name: null" and "From (reserved word) cannot be
used as model name. Renamed to ModelFrom". The Go run printed the first of those. The five NDJSON
export operations generated without error (their response is typed as a string/file by default; not
exercised at runtime).

### openapi-python-client 0.29.1

`openapi-python-client generate --path external.openapi.json --meta none`: 332 `.py` files (45.7 k
lines), 20 API modules, import works. It prints five warnings, one per `GET /export/*`: "Cannot parse
response for status code ... (Unsupported content_type application/x-ndjson), response will be omitted
from generated client". So the five Export stream operations have no typed response. Without `ruff` on
the PATH it prints "ruff failed" but still writes the client.

### Kiota 1.36.1

`kiota generate -d external.openapi.json -l <lang> -n sphericon.client -c SphericonClient`:

| Language | Result                                                                                                                 |
| -------- | ---------------------------------------------------------------------------------------------------------------------- |
| python   | exit 0, 274 files (20.2 k lines); with the six `microsoft-kiota-*` packages installed, `import` of the client succeeds |
| go       | exit 0, 292 files (41.4 k lines); with `go get` of the `kiota-*-go` modules, `go build ./...` **succeeds**             |
| csharp   | exit 0, 292 files (26.6 k lines); not compiled (no .NET SDK on the trial machine)                                      |

Kiota's warnings were three, identical for all languages: multiple `servers` entries (only the first,
`http://localhost:3000/api`, becomes the default base URL, which the caller must override), format
`email` unsupported (string used), format `int64` on a String type ignored. It had no trouble with the
`oneOf`/`discriminator` unions or the NDJSON responses (not exercised at runtime).

### What the trials do and do not show

Compiling or importing is a floor, not a test of behaviour: no request was sent to a running server in
any trial, and the C# and Java output was not compiled. The `hasSecret` and enum-constant collisions
are properties of this contract meeting OpenAPI Generator's Go templates; they would likely recur if
the contract grows more enums (a new enum with an existing value name breaks the Go output).

## Recommendation

1. **Go: use the ogen client that is already generated** (`gen/external`). It is Apache-2.0, built from
   the same contract by the same `mise run generate` step, and compiled in this repo today. OpenAPI
   Generator's Go output does not build on this contract, and Kiota's Go output builds but needs six
   extra runtime modules and a 41 k-line tree. The remaining work is packaging, not generation.
2. **Python: openapi-python-client.** MIT, Python-only, typed, produces a small dependency-light
   client (plain `httpx` and `attrs`), imports cleanly. Its one gap is the five NDJSON Export
   operations, which it omits; Export is an Enterprise stream that integrators more likely read
   from a loader than from an SDK. If that gap matters, Kiota Python is the fallback (it covers NDJSON
   without a warning but brings six runtime packages).
3. **C#, Java, PHP: Kiota.** The only candidate whose C#, Java, PHP, Python and Go generators are
   first-party, MIT, marked as supported in its own table, with one design across languages. OpenAPI
   Generator is the alternative with the widest list (175 generators, many STABLE) and the
   same Apache-2.0 licence, but the Go failure above argues for checking each language's output on
   the contract before committing to it.
4. **Ruby and Swift: not yet.** Kiota's Ruby is preview; OpenAPI Generator `ruby` and `swift6` are
   STABLE but untried here. Open a ticket only when a user needs them.
5. **Do not adopt Fern, Speakeasy or Stainless.** Fern's open-source repository is unmaintained;
   Speakeasy's CLI is Elastic-licensed and its SDK generator is not in the repository; Stainless is a
   hosted service. None gives a contributor a `mise run generate` they can reproduce offline.

Consistent with #568 (SDKs generated from `openapi/external`, never hand-written, not published):
each language SDK is a generated tree plus a `mise run generate` step, a generated-code header and a
`.gitattributes` flag, like `@sphericon/node`.

## Follow-up tickets, one per language

Each ticket: generate from `openapi/external` as a `mise run generate` step, checked in with the
generated-code markers, one smoke test that creates a Contact through the token-authenticated API,
no publishing to a registry.

1. **Go SDK: package `gen/external`'s ogen client for integrators.** Decide the import path and
   whether the client is exposed as its own Go module (the generated package also contains the
   server); add a short usage example. No new generator.
2. **Python SDK with openapi-python-client.** Pin the version, run `ruff` (the tool needs it), set the
   base URL to the external API's `servers` entry, and decide how to handle the five NDJSON Export
   operations.
3. **C# SDK with Kiota.** Compile the output (not done in this research), override the base URL
   (Kiota uses the first `servers` entry), pin the runtime packages.
4. **Java SDK with Kiota.** Same as C#; first confirm the generated tree compiles (not tried here).
5. **PHP SDK with Kiota.** Same; first confirm the output (not tried here).
6. **Ruby / Swift / Rust / Kotlin: no ticket until requested.** Revisit OpenAPI Generator
   (`ruby`, `swift6`, `rust`, `kotlin`: all STABLE per its pages) with a trial on the then-current
   contract.

## Open questions

- The licence of generated output (as opposed to the generator) was not checked for any tool.
- Runtime behaviour against a live `/api`: untested for every language; the follow-up tickets carry it.
- Speakeasy's and Fern's hosted terms were not read; they were excluded on the repository evidence alone.
