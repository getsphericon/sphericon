---
status: accepted
build: [1722]
---

# An operation declares its effect, and the MCP hints follow it

> **Revised before it was built:** five effects instead of four (`write` split into `create` and
> `change`, as a replace and a state change are destructive where an addition is not), idempotency
> from the method, the compiler's cross-check against the send-class scopes, and `/api` alone.

> Amends [[0016-mcp-surface-projection-of-external-api]] (where a tool's behaviour hints come from, and
> where send-class is declared).

## Context

The MCP projection gives every tool the hints of the MCP specification (`readOnlyHint`,
`destructiveHint`, `idempotentHint`, `openWorldHint`) from the HTTP method alone: GET reads, PUT, PATCH
and DELETE are destructive, POST is neither. So the tools that send (Broadcast Schedule, Automation and
Rule Publish, Email send) are announced as harmless additions, and the reads that take a body (a Segment
preview, the email lint) are not announced as reads. A host that asks before a call by these hints asks
nothing before a send. Send-class is declared apart, only for MCP (`@@mcp(…, #{ send: true })`), so the
contract says twice, in two places, what an operation does.

Agents' hosts gate confirmation on the hints: a send is a "sensitive write" that a person confirms each
time. Email providers' own MCP servers differ: some mark their sends destructive and open-world, one
marks them not destructive, and those with reads sent as POST mark them read-only.

## Decision

- **Every operation of `/api` declares one effect**, the surface MCP projects:
  - `read`: it changes nothing (a GET, a Segment preview, the email lint, a dry run, an export);
  - `create`: it only adds (a new object, an Event, a Tag applied, a Suppression recorded);
  - `change`: it alters or replaces what exists, or moves it to another state (an update, Archive,
    Revert, Deactivate, Cancel, a domain's verification);
  - `remove`: it deletes;
  - `send`: it arms, starts or withdraws mail to people on its own authority (Schedule and
    Unschedule, Publish, an Email send).
- **The compiler holds the declaration true where it can**: a GET is `read` and needs no declaration,
  any other method must declare one, and a GET that declares another is an error; an operation is
  `send` exactly when it requires a send-class scope (`SendScope`), both ways. The TypeSpec library
  reports each as a compile error, so an operation whose effect is missing or contradicts its scopes
  does not compile.
- **`send` is the one declaration of send-class.** The `send` key of `@mcp` goes, and the `mcp:send`
  lock reads the effect. An operation that sends only in some cases (editing a Template in
  Transactional use, a Test send to another address: ADR 0132) is not `send` and keeps its check at
  run time; nor is one that releases mail a send-class operation already armed (picking a Split
  test's Winner by hand, ADR 0152) or that stops mail (Cancel, ADR 0150).
- **The MCP hints are a function of the effect**, and idempotency of the method, which RFC 9110
  defines (PUT and DELETE are idempotent, POST and PATCH are not):
  - `read`: `readOnlyHint: true`;
  - `create`: `destructiveHint: false`;
  - `change` and `remove`: `destructiveHint: true`;
  - `send`: `destructiveHint: true`, `openWorldHint: true`: it reaches people outside the system and
    cannot be taken back.
    Every other tool keeps `openWorldHint: false`. The hints remain hints: the lock is still the server's.

## Considered options

- **Keep the method and add hints by hand to the send tools**: two sources of truth again, and a new
  sending operation is safe only if someone remembers.
- **Mark sends `destructiveHint: false`**: the hint then says nothing a host can act on.

## Consequences

- Every non-GET operation of `/api` gains its effect; `@@mcp` keeps `name` and `hidden` only.
- Glossary: **Operation effect**.
