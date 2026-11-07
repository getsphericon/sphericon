---
status: accepted
---

# A use case has one call contract

> Refines [[0102-the-core-is-layered-and-an-operation-across-modules-is-a-use-case]]: it said what a
> use case is; this says how one is called.

## Context

The use cases (`internal/usecase`, `ee/usecase`) grew one at a time, and each chose its own call
shape:

- **Inputs:** positional arguments next to `XInput` structs. A Workspace's deletion took the actor
  and `now` as arguments, while the Release check and the Needs attention report read a clock
  given to them.
- **Outputs:** tuples (`(*ent.User, []string, error)`) and named results next to single values. A
  batch answered `[]BatchOutcome` in one place and `[]error` in another.
- **Shape:** some operations were package functions (`stepexecution.OnEvent`,
  `auditlogretention.Prune`), and the Abuse detector was a `Detector`, not a `UseCase`.
- **Driver code:** some packages wrote a command's report to an `io.Writer`.

A caller had to learn each package's shape anew, and a reviewer could check none of it.

## Decisions

- **A use case package exports `New` and the methods of its `UseCase`.**
  - `New(deps...) *UseCase` is the only exported function. Each dependency is a parameter: there
    are no functional options and no `With…` builders.
  - `New` returns the use case alone. A dependency that can fail to build is built and checked by
    the wiring, which hands it in.
  - No other exported type has methods that take a `context.Context`: an operation is a method of
    `UseCase`. A port the use case declares is an interface the wiring fills.
  - Value types are allowed: an input, a result, a port's argument, a job's args.
- **A method takes `ctx`, then the scope, then at most two required arguments.**
  - The scope is the `*ent.Scoped` of the Workspace the operation acts in.
  - An operation that belongs to no Workspace when it starts takes no scope argument. Its use case
    holds the raw client from `New` (the closed list of `docs/architecture.md`, Workspace scoping),
    because a driver holds no raw client to pass. A method that joined its caller's transaction
    would take that transaction's client as its scope. None does: the deletion of a User spends a
    Second factor code through the function `secondfactorcheck.UseCase.DeletionCode` hands it.
  - A third argument, or any optional one, goes into a typed `XInput` struct named after the method.
- **The actor is part of the input. The clock is a dependency.**
  - Who acts is one of the required arguments or a field of the `XInput`.
  - The time comes from the Instance clock (`app.Clock`), which the wiring gives to `New`. No
    method takes `now`, and no use case reads `time.Now`; a test gives `New` the clock it needs.
- **A method returns one value, or a named struct.**
  - One value is returned as it is (`*ent.Workspace`, `[]Item`), with an `error` beside it.
  - Two or more values go into a named result struct (`XResult`, or a name of the glossary).
  - No exported method returns a tuple or has named results.
- **Errors come from the shared taxonomy.** A refusal is a `validation` error that `api/problem`
  maps (ADR 0103). A protocol's driver that answers in its own words maps the same refusals: the
  OAuth token endpoint answers `connectionissuance.ErrInvalidGrant` (a
  `validation.UnknownReference`) as `invalid_grant` and `ErrScopeExceedsGrant` (a refused rule) as
  `invalid_scope`. Any other error is a failure the surface answers as 500.
- **A batch returns `batch.Result`** (`internal/kernel/batch`), one `batch.Item` per item in the order
  given: the item's value or its error. A batch never returns `[]error` or a type of its own.
- **A use case holds no driver code.** It mounts no HTTP route and writes no report to an
  `io.Writer`; the API, the CLI or the job runner does that with the value the use case returns.
  Its job's args may stay with it, since it queues the job, but the worker and the schedule belong
  to the runner (#1153).
- **The `usecaseshape` analyzer checks the form** (`tools/analyzers/usecaseshape`, review rule 5)
  in every package under `internal/usecase` and `ee/usecase`. It refuses:
  - an exported function other than `New`, and a `New` that returns anything but `*UseCase`;
  - an exported type other than `UseCase` with a method that takes a `context.Context`, and an
    exported method that returns `*UseCase` (a builder);
  - an exported method with more than two results, a second result that is not `error`, or named
    results;
  - more than two arguments after `ctx` and the scope;
  - a `now` argument, a `func() time.Time` argument of a method, and any use of `time.Now`;
  - an `io.Writer` argument, and a `[]error` result.

## Considered options

- **Functional options for optional dependencies.** An option hides a dependency the wiring must
  remember to give, and the use cases that had them always got them in production. Each one is a
  parameter of `New` instead.
- **`now` as an argument.** It lets a test pick the time, but so does a clock given to `New`. With
  an argument, each driver reads the time on its own, and two operations of the same request can see
  two different moments.
- **The raw client as an argument of a cross-Workspace operation.** A driver holds no raw client
  (`docs/architecture.md`), so it could not pass one. The use cases that need one hold it.
- **An ast-grep rule.** The checks need types (`*ent.Scoped`, `error`, `io.Writer`), so the rule
  is an analyzer of our own.

## Consequences

- Every use case changes to this shape, with its callers in the API surfaces, MCP, the jobs and
  the CLI. The generated contracts do not change.
- What was not an operation leaves the use cases:
  - The CLI's reports and keyset changes move to `internal/app`, beside the operations the CLI
    calls there.
  - The send-outcome counter, an event subscriber, moves to the wiring (`app.RecordSendOutcome`).
  - What the Telemetry report says of the process (its host and deploy shape) is read by the wiring,
    and the Instance's report offset belongs to `instancestates`.
  - The Erasure's identifier is an input value that the use case checks. Both surfaces now refuse an
    invalid address the same way, where only the site checked it before.
- The domain modules are not covered: the call contract binds the use case tiers only. The
  Workspace oversight module's tuples go too (#1160), but no check holds it to the rule.
- A new use case is checked by the analyzer from its first commit.
