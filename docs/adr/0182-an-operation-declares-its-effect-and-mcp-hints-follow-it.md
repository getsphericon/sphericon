---
status: accepted
build: none
terms: [Operation effect]
---

# An operation declares its effect, and the MCP hints follow it

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

- **Every operation of a surface declares one effect**: `read`, `write`, `send` or `remove`. A GET is
  `read` and needs no declaration; any other method declares its effect, and a GET that declares one
  other than `read` is an error. The TypeSpec library checks both at compile time, so an operation with
  no effect does not compile.
- **`send` is the one declaration of send-class**: an operation that arms, starts or withdraws mail to
  people on its own authority (Schedule and Unschedule, Publish, an Email send). The `send` key of
  `@mcp` goes, and the `mcp:send` lock reads the effect. An operation that sends only in some cases
  (editing a Template in Transactional use, a Test send to another address: ADR 0132) declares `write`
  and keeps its check at run time; so does one that releases mail a send-class operation already armed
  (picking a Split test's Winner by hand, ADR 0152) or that stops mail (Cancel, ADR 0150).
- **The MCP hints are a function of the effect**:
  - `read`: `readOnlyHint: true`;
  - `write`: `destructiveHint: false`, `idempotentHint` true for PUT;
  - `remove`: `destructiveHint: true`, `idempotentHint: true`;
  - `send`: `destructiveHint: true`, `openWorldHint: true`: it reaches people outside the system and
    cannot be taken back.
    Every other tool keeps `openWorldHint: false`. The hints remain hints: the lock is still the server's.

## Considered options

- **Keep the method and add hints by hand to the send tools**: two sources of truth again, and a new
  sending operation is safe only if someone remembers.
- **Mark sends `destructiveHint: false`**: the hint then says nothing a host can act on.

## Consequences

- Every non-GET operation in `typespec/` gains its effect; `@@mcp` keeps `name` and `hidden` only.
- Glossary: **Operation effect**.
