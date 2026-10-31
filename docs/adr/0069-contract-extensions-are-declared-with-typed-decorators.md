---
status: accepted
---

# Contract extensions are declared with typed decorators

The contract carries what an operation needs beyond its HTTP shape as OpenAPI extensions that Go
reads: the scopes and Permissions of [[0061-access-to-an-operation-is-declared-in-the-contract]],
the marks of [[0020-session-expiry-epoch-and-two-step-login]] and
[[0062-sign-up-items-the-operator-sets-and-every-decision-is-recorded]],
the MCP projection and the untrusted fields of [[0016-mcp-surface-projection-of-external-api]].
Each was written as a raw `@extension("x-…", value)`, which takes any key and any value: a
misspelled key or a wrong value compiled, and was caught, if at all, by `cmd/scopegen` or a Go test
after the whole generation, with no line of the `.tsp` to point at. `#{ hide: true }` on an
operation would have exposed it as an MCP tool.

## Decisions

- **Every extension a Go reader depends on is written by a decorator** (`@requiresPermissions`,
  `@requiresScopes`, `@requiresServiceScopes`, `@grantable` and `@grantedWith`,
  `@allowedWhileWithheld`, `@allowedWhileAcceptancePending`, `@untrusted`, `@mcp`), typed by the
  contract's own enums where it names their members. The compiler refuses an unknown key, a value
  of the wrong type and a member of another enum; the decorator refuses what a type cannot say,
  such as an empty `@mcp` or a second declaration on the same target.
- **The OpenAPI documents do not change.** A decorator writes the extension a raw `@extension`
  wrote, so ogen, openapi-ts, `cmd/scopegen` and `internal/mcpserver` read what they read before,
  and stay the one path from the contract to Go (no emitter writes Go from TypeSpec).
- **What each surface must declare is checked at `tsp compile`.** A surface names its rule with
  `@everyOperationDeclares`: every /api and /service operation declares its scopes; on /site an
  operation with `workspaceId` in its path declares its Permissions, and no other operation
  declares Permissions or `@allowedWhileWithheld`. `cmd/scopegen` keeps its checks as the last
  guard on the OpenAPI document.
- **A raw `@extension` of one of these keys fails compilation**, so the old form cannot come back
  past its decorator.
- **The decorators live in the repository** (`typespec/lib`, imported by path), not in a package:
  they serve one contract. Their tests compile fixtures with the TypeSpec compiler under node's
  test runner (`mise run test:typespec`), as Vitest runs in the browser.

## Considered options

- **A TypeSpec emitter that writes the Go tables.** Rejected: it would be a second path from the
  contract to Go beside the OpenAPI document that every other reader uses, and would move
  `cmd/scopegen`'s tested Go into JavaScript for the same output.
- **A text check that bans `"x-…"` strings in `.tsp` files.** Rejected: it catches the old form
  but types nothing, and points at no misuse of a well-named key.
- **TypeSpec linter rules** (`$linter`) for the surface checks. Rejected: a linter rule runs only
  where a `tspconfig.yaml` enables it, and a library imported by path is no package to name there;
  `$onValidate` runs in every compilation that imports the decorators.
