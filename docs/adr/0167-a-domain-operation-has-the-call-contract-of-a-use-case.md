---
status: accepted
---

# A domain operation has the call contract of a use case

> Extends [[0104-a-use-case-has-one-call-contract]] to the domain modules.

## Context

ADR 0104 gives every use case one call shape, checked by `usecaseshape`, which reads only the use case
tier. The domain modules, about four hundred operations, follow no rule: most are methods of a `Module`,
some of other types with a role of their own (`StepExecution`, `Detector`, `Views`), and about two
hundred are package functions. Some forty take `now time.Time`, some twenty return three values or more,
about a hundred take more than two arguments after the scope, and a few take their clients through
functional options. The ban on reading the wall clock already holds everywhere. As operations move from
the use case tier to the modules that own their subject, they leave the analyzer's reach.

## Decisions

- **A domain operation follows ADR 0104's call contract**: an exported operation that takes a context is
  a method of an exported type made by a constructor `New…` that receives its dependencies, the clock
  and clients included; after the context and the scope (`*ent.Scoped`, or `*ent.Client` where the
  operation is not scoped) it takes at most two arguments, else an input struct; it returns at most a
  value and an `error`, with no named results; it takes no `now` and no client as an argument, and no
  functional options.
- **A module may have more than one such type.** A role with dependencies of its own, such as executing
  a Step or detecting abuse, stays a type of its own rather than being folded into `Module`; what the
  contract fixes is the call, not the count of types. An exported package function that takes a context
  is not allowed; a pure helper without one is.
- **One analyzer checks both tiers.** `usecaseshape` becomes `callshape`: the shared rules apply to the
  use case and domain tiers, and the rules of the use case tier alone (`New` and `UseCase` only) stay
  there.
- **The rule ratchets in.** It applies to every domain package at once, with a list of packages exempt
  while they are moved; the list may only shrink, and CI fails when it grows. Each migration removes its
  packages from it, and the list goes when it is empty.

## Consequences

- `tools/analyzers/usecaseshape` is renamed and extended; the exemption list and its check are added.
- The domain operations move to the contract package by package.

## Rejected

- **A second analyzer for the domain tier**: two copies of the same rules that would drift.
- **One `Module` type per package**: it would bundle unrelated dependencies into every constructor to
  satisfy a count.
- **Turning the rule on only for new code**: the old shapes would stay the examples new code copies.
