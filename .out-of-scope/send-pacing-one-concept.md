# Consolidating send pacing into one concept

This project does not fold the pieces of Broadcast send pacing into a single module. The bucket
core stays in `internal/providerlimit`, and the planner and the progress estimate in
`internal/broadcasts` keep calling it.

## Why this is out of scope

The candidate came from an architecture review and was rated speculative: "how fast does a Broadcast
go" is read in several places, so a single pacing concept looked tidier. Reading the code found no
disagreement to fix:

- `Pace` (the planner) and `progressOf` (the ETA) take their step from the same `interval()` in
  `internal/broadcasts/plan.go`, which is `providerlimit.EffectiveOf(integ).Limits().Interval()`.
- The ETA is the last scheduled recipient job while that is still ahead of now. Only once it has
  passed does it re-derive `pending × step`. ADR 0023 names the cases where an estimate drifts (a
  limit lowered mid-send, Transactional traffic spending capacity, provider throttling), and a new
  module would not remove them.
- The limits the Integration pages show and the limits the limiter applies both come from
  `EffectiveOf`, the lowest of the manual and the provider value.

What would move is about fifteen lines of wiring: the thin adapter in Outbound send and the
`interval()` helper. The shared core is already deep, with a model test and a property test.

Revisit if a concrete disagreement turns up, such as an ETA that drifts from the pacing actually
applied, or a limit shown that is not the limit enforced.

## Prior requests

- #488: "Decide whether send pacing deserves one concept"
