---
status: accepted
---

# Every error is the default Problem response

Each operation of the contract listed its error answers one status at a time (`| BadRequestProblem
| NotFoundProblem | UnprocessableEntityProblem …`, about 1,100 of them), and ogen made each listed
status a member of a per-operation sum type (`SiteContactsGetRes`). The Go side used none of those
members: a handler returned the module's error and the server's error handler wrote the problem
(review rule 15), so every handler wrapped its success in the sum (`return &res, nil`) and every
test unwrapped it again (`res.(*siteapi.SiteContactResource)`, close to 900 times). The generated
packages carried a type, an encoder and a decoder per listed status. The lists were also not the
truth: any operation could answer a 401, a 429 or a 500 it did not list.

## Decisions

- **Every operation declares one error response, `Problem`, as its `default`** (`common/errors.tsp`:
  an `@error` model with no status code, its body the one `ProblemDetails`). A refused removal's
  `usedBy` and a refused User deletion's `workspaces` are optional members of that body, not
  models of their own. No operation lists an error status.
- **ogen reduces it to one error type per surface** (`ogen.yml`, `convenient_errors: on`, which
  fails the generation when an operation lacks the response). A handler returns its success type
  and an error, or only an error when it answers no body; a client returns the same.
- **The answer is decided once, in `internal/problem`**, for every surface: each surface's
  Handler `NewError` calls `problem.Site`, `problem.External` and their kin, which map the error to
  a status and the site's `ProblemDetails` (`problem.Of`) and convert it to the surface's own type
  (goverter, `internal/problem/mapping`). The server's error handler, which ogen still calls for a
  request it cannot decode, answers through the same `problem.Of`. An answer no refusal kind gives
  (the 402 of a missing Enterprise license) is a `problem.Error` the handler returns.
- **The rate limit headers are not in the contract.** A 429 still carries `Retry-After` and the
  `X-RateLimit-*` headers, and its body `retryAfter`; the `default` response cannot declare headers
  every other answer lacks.

## Consequences

- The OpenAPI documents no longer say which statuses an operation answers; the operations'
  descriptions say it where a client must branch (a 409 of a refused removal), and the problem's
  `status` and `code` say it on the wire. The TypeScript clients type every error as
  `ProblemDetails`, as `src/utils/apiErrors.ts` already read them.
- `vacuum` checks that every operation declares the `default` response (`operation-declares-problem`)
  instead of its 400.
- A test reads a refusal with `testhelper.SiteRefusal` (and its kin), which checks the status and
  returns the problem.

## Considered options

- **A helper that unwraps the sum in tests, contract unchanged.** It removed the test lines and
  left the handlers' wrapping, the generated types and the lists that were not the truth.
- **Listing the statuses beside `default`.** ogen keeps a sum member for each listed status, so the
  sum types came back.
