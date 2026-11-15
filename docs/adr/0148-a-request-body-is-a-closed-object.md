---
status: accepted
---

# A request body is a closed object

> Amends [[0016-mcp-surface-projection-of-external-api]] (what MCP refuses) and
> [[0103-every-error-is-the-default-problem-response]] (an unknown property is a decoding error).

## Context

The generated servers of `/api`, `/site` and `/staff` skip a property they do not know: every object
decoder ends its field switch with `default: return d.Skip()` (`gen/external/oas_json_gen.go`), because
no model of these contracts sets `additionalProperties`. A misspelt key is therefore dropped without a
word, and the request goes on as if it had been left out.

Where leaving a property out has a meaning, that is a wrong action, not a typo. `CreateBroadcastInput`
takes an optional `segmentId`, and a Broadcast without one goes to all Contacts: a script that writes
`segment_id` makes a draft to the whole Workspace, and Schedule sends it. The same holds for any
optional field whose absence means "all", "none" or "keep": a filter, a Condition leaf, a Step's
option.

MCP is mostly closed already. Its tool input schema sets `additionalProperties: false`
(`internal/mcpserver/projection.go`), and its handler refuses an argument left over after the
parameters and body fields are placed ("unknown arguments"). A property nested inside an object
argument (a Broadcast's `content`, a Condition, a batch item), and a body that MCP passes whole, still
reach the decoder and are skipped.

## Decisions

- **Every request body of `/api`, `/site` and `/staff` is a closed object, at every depth.** A property
  the contract does not declare is refused before the handler runs: a decoding error, answered as the
  default Problem with status 400 and the path of the property (`content.subjct`) among its field
  errors (ADR 0103), the same answer as a value of the wrong type. There is no separate problem code:
  the caller fixes the request, as for any other malformed one.
- **A map that is open by design says so in the contract.** The values of Custom fields, an Event's
  properties and what Identify writes take keys the Workspace chooses (ADR 0006); they stay open, and
  so does any other map whose keys are data. Being open is declared on the model, not assumed: a
  contract check fails a model used in a request body that is neither closed nor declared open, so a
  new model cannot be left open by accident.
- **Responses stay open to their readers**: a client ignores a property it does not know, which is
  how the contract grows additively (ADR 0016, ADR 0033).
- **Surfaces the app does not write the client of stay tolerant**: `/collect` and the tracker, whose
  code lives on customers' sites longer than a release (ADR 0036); the inbound callbacks of providers,
  which add fields when they choose (ADR 0037); the telemetry report a newer Instance sends. They
  skip what they do not know.
- **No compatibility mode.** No external client depends on a skipped property (ADR 0119 found none to
  keep), so the change is made at once, on every operation.

## Consequences

- The contract's models used in request bodies are closed (TypeSpec emits `additionalProperties:
  false`), ogen generates decoders that refuse an unexpected field, and the open maps carry their mark.
- MCP's own check of top-level arguments stays; nested properties are now refused by the decoder it
  dispatches to.
- Tests: an unknown top-level and an unknown nested property are each a 400 naming the path, on a
  `/api` and a `/site` operation; an open map still takes any key; an ingest surface still skips an
  unknown field.

## Considered options

- **Close only the inputs that decide a send** (Broadcast, Automation, Rule, Schedule). The same trap
  waits in every optional field whose absence means something, and deciding which inputs "matter"
  is a judgement each new model would have to get right.
- **Rely on MCP's own check.** It protects one client and only at the top level; scripts call `/api`.
- **A dedicated `unknown_field` problem code, or 422.** A misspelt property is a malformed request like
  a wrong type; one answer for both keeps the client simple.
