---
status: accepted
---

# Every entry point is a driver that calls one operation of the core

> Refines [[0102-the-core-is-layered-and-an-operation-across-modules-is-a-use-case]] and
> [[0104-a-use-case-has-one-call-contract]]: they said what a driver may call; this says what a
> driver is, how much it may call, and moves the CLI's operations out of the wiring.
>
> **Amended by [[0112-an-opt-out-is-an-event-on-every-channel-and-the-broadcast-counts-it-from-the-event]]:** the Broadcast counts
> an opt-out only from its event, on every Channel, and `unsubscriberecording` no longer counts it.
> The decision that a Telegram opt-out is recorded through `unsubscriberecording` with the same
> effects in the same transaction is reversed.

## Context

Review rule 5 says a driver "only receives input, calls a domain module or a use case and returns the
result". Nothing checks how much a driver calls, and some code that receives input is not under the
driver rules at all:

- **Handlers that compose an operation.** About 30 handler functions of `/site` and `/api` call two
  or more modules of the core. Some of them hold a rule a surface should not hold:
  - User deletion: `site/user_deletion.go` calls `users`, the Login throttle, the deletion link and
    the Second factor.
  - The sign-in callback of a Sign-in provider calls six packages and branches on the Second factor.
  - The Audit log page is written three times (`/site`, `/api`, the Enterprise Staff console). Each
    copy checks the License, builds a 402 `problem.Error` in the handler and builds the same filter.
  - A Test send of a Broadcast: whether the System sender may send it, and the wait its cap
    imposes, are decided by which argument the driver passes (`/site` an `Asker`, `/api` `nil`).
  - A Workspace's suspension checks for a reason twice, in the CLI and in the Workspace oversight.
  - A token's bootstrap calls `CheckBootstrap`, `workspaces.BootstrapScope` and `Mint` in the
    handler.
- **Drivers outside the driver rules.**
  - `internal/server` sits in `wiring`, which holds the raw client. Its tracking handler queries
    ent, builds scopes without an actor, opens `WithinScopedTx`, stamps the Outbound message and
    builds and publishes the event itself. Its hooks handler looks up an Integration by its ingest
    key and builds the scope the use case runs in.
  - `cmd/` is `unowned`, so nothing checks the CLI. Its operations live in `internal/app`
    (ADR 0104), with raw lookups of their own (`workspaces.IDByPublicID`, a User by email).
  - The Enterprise Edition's workers, consumers and schedules are in `ee/jobs.go` (`ee-edition`),
    and the send-outcome subscriber is in the wiring.
- **Rules in the runner.** The Broadcast worker decides when to give up and what a stale transition
  means. A Telegram opt-out is counted on the Broadcast by a consumer after the commit, while every
  other Unsubscribe goes through `unsubscriberecording` in one transaction.
- **Writers in the domain tier.** ADR 0104 removed `io.Writer` from use cases only. The CLI's
  reports are still functions of `users`, `staffmembers`, `suspension` and `servicecredentials`, and
  `upgrade`, `preset` and `dataexport` write to a writer they are given.
- **One resource built twice.** `/site` and `/api` share 87 calls into the core and differ in how they
  build the same resource (an Integration, a Workspace, a Segment preview). 145 of the 175 commits
  that changed `/api` since September also changed `/site`.

The obvious uniform rule is "a driver calls only use cases". Its count speaks against it: about 310
handler functions of `/site` and `/api` call one module once. A use case for each would only forward
the call, and `releasecheck.LatestRelease` already shows that shape.

## Decisions

- **A driver is any code that receives input from outside the program:** an HTTP handler wherever
  it is mounted, an MCP tool, a CLI command, a river worker and an event consumer. The wiring builds
  the program and handles no input.
  - The tracking, hooks and TLS-ask handlers leave `internal/server` for driver packages under
    `internal/api`. `internal/server` keeps the router and the middleware.
  - The CLI's operations leave `internal/app` for a driver package of the CLI, and `cmd/` is a
    component with the driver rules.
  - The Enterprise Edition's workers, consumers and schedules and the send-outcome subscriber move to
    driver components with the runner's rules. `ee/ee.go` keeps building the Edition.
  - The use cases a driver needs come from the container, not from the router.
- **A driver function calls one operation of the core.** An operation is a call into a domain
  module or a use case that takes a `context.Context`. Reading the actor, the scope middleware
  builds, and mapping to and from the contract are not operations.
  - A driver function that needs a second call holds an operation of the core. That operation moves
    into the module that owns its rule, or into a use case when it spans several modules.
  - The analyzer `driverops` (`tools/analyzers`, review rule 5) checks every driver package.
- **The use case tier stays open.** A driver calls a domain module's operation directly when one
  module holds it, and a use case only when the operation spans several (ADR 0102). A closed tier
  would add a forwarding method for each of about 310 handler functions, with nothing behind it.
- **The operations that drivers compose today move into the core:**
  - User deletion and the sign-in through a Sign-in provider are use cases. The sign-in returns
    what happened (signed in, a Second factor is due, signed up), and the driver turns it into a
    cookie or a redirect.
  - `auditlog` serves the Audit log page, refusal without a License included, as a refusal of the
    shared taxonomy that `problem` maps once (review rule 15).
  - `outboundmessages` reads from the scope's actor whether the System sender may send a Test
    send. The System sender's cap is a refusal with a wait, which `problem` maps.
  - `suspension` requires the reason, and `apitokens` performs a token's bootstrap.
  - `outboundmessages` records an open or a click: it resolves the token, and in its own
    transaction stamps the message and publishes the event.
  - `callbackinterpretation` takes the ingest key and the request body. It finds the Integration,
    confirms a subscription and builds the scope itself.
  - The Broadcast worker hands the attempt to `broadcasts`, which decides on a retry or giving up
    ([[0107-a-domain-module-checks-every-input-to-its-writes]]: the job runner holds no domain rule).
  - ~~A Telegram opt-out is recorded through `unsubscriberecording`, with the same effects in the
    same transaction as any other Unsubscribe.~~ Reversed by ADR 0112: an opt-out is an event on
    every Channel, and the Broadcast counts it from the event.
- **A resource is built once below the contracts.** The Integration view and the Segment preview
  are operations of `integrations` and `segments`. The Workspace's view, which reads identity and
  sending, is a read use case named after the report it builds. `/site` and `/api` keep their own
  contracts and map the same values.
- **The domain tier writes to no `io.Writer`.** A module returns a value or an iterator
  (`iter.Seq2`). The CLI prints its reports and an upgrade's steps, and the HTTP driver streams a
  Data export.
- **easement states the order once.** `layers` lists the drivers, the use cases, the contexts from
  instance down to identity, the infrastructure and the kernel, in place of a `may_depend_on` list
  in each component. The use case layer is open. `ee-api` may depend on the Enterprise use cases.

## Considered options

- **Drivers call only use cases** (a closed use case layer). It is one flag in `layers` and one
  sentence of a rule. It costs a shallow use case for every single-module call, and the vocabulary
  the drivers import (sentinels, scopes, views) would need a re-export. The rule a driver must keep,
  one operation per function, does not need it.
- **A use case per context, grouping each context's operations.** It would be a facade per context,
  which ADR 0102 rejected, and it breaks the one `UseCase` per package of ADR 0104.
- **`max_fan_out` in easement.** It counts components a component depends on, not calls a function
  makes, so it cannot see a handler that composes two modules of one context.
- **One contract for `/site` and `/api`.** Their callers differ (a session and a token), and so do
  their promises (a public, stable `/api` that MCP projects, ADR 0016). The duplicate work is in
  building the resource, which moves below both.

## Consequences

- Review rule 5 reads: a driver receives input, calls one operation of a domain module or a use case,
  and returns the result.
- ADR 0104's placement of the CLI's operations in `internal/app` is replaced by the CLI's driver
  package.
- `docs/architecture.md` and `easement.yml` change with the code, one move per change. `driverops`
  is switched on when the last driver it would fail is moved.
- MCP needs no change: it runs the `/api` handlers, and inherits whatever they call.
