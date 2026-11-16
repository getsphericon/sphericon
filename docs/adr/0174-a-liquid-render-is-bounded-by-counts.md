---
status: accepted
---

# A Liquid render is bounded by counts

## Context

Every message, a Telegram text and an HTTP step's JSON body is a Liquid template (ADR 0039, ADR 0040,
ADR 0078), written by a Workspace Member or an API client and rendered in the process every Workspace
shares, once per recipient. osteele/liquid, the engine, has no bound of its own: a string doubled by
`append` or `capture` in a loop reaches gigabytes in a few dozen iterations, a range of any size
iterates without writing anything, and the library's advice is to isolate the process. A render that
does that stalls the sends of every Workspace, and one behind an `if` on a Contact attribute passes
the check made when the content is saved.

## Decisions

- **A render has a budget of counts, not a clock.** At most 5,000 loop iterations, assigns and
  captures together, whatever a loop body writes; a value (an assign, a capture, a filter's input,
  argument or result) of at most 256 KiB of text and 1,000 items; at most 1 MiB written. Counts make a
  render end the same way every time, so a test pins it, as Shopify's Liquid bounds a render by its
  score rather than by time.
- **A range is fixed where it is written**: two whole numbers at most 1,000 apart. A range is the one
  value a template can make as large as it likes in a few characters, and a filter given one builds
  all of it.
- **A filter that multiplies its input is measured before it builds**: `replace` by its matches,
  `join` by its separator. Every other filter is measured on what it takes and what it gives.
- **Past a bound, the render fails as the content's own failure** (`ErrTooMuchWork`): never sent,
  never retried, and refused where the content is saved when its sample data reaches the bound.
- **A panic of the library is the content's failure** (`ErrUnparsable`): its expression parser
  panics on some identifiers rather than refusing them, and a render recovers it instead of letting
  one template end the goroutine that sends for every Workspace.
- **The engine stays osteele/liquid, with the tags registered by sphericon.** Its loop has no hook
  and a block it defines cannot be redefined, so the standard tags are registered here, adapted from
  the library (MIT) with the charges added, and its filters are registered measured. Authors' Liquid
  is unchanged.

## Consequences

- `internal/kernel/emailrender` holds the engine (`budget.go`, `tags.go`); the library's version is
  pinned, and an upgrade compares its tags with these.
- A template that loops more, or builds larger values, than a message needs fails with a message
  that names the bound.

## Rejected

- **Another engine with limits built in** (a port of Shopify's Liquid): its render API is being
  rewritten, its output escape is for HTML only, so the HTTP step's JSON escape (ADR 0039) would need
  a change upstream, and authors would meet another Liquid (`raw` for `safe`, an empty string false).
- **Rendering in a separate process** with operating-system limits: the strongest bound, at the cost
  of a process or a pool per render for every recipient of a Broadcast, and of operating it on every
  self-hosted Instance.
- **A deadline**: the Instance reads one injected clock, and a time limit fails a render on a busy
  machine that passes on an idle one.
- **Bounding only the output**: a loop or an assign that writes nothing would still run unseen.
