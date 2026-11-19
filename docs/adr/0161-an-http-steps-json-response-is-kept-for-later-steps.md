---
status: accepted
build: [1456]
---

# An HTTP step's JSON response is kept for later Steps

> **Amended by [[0181-a-conditions-shape-is-typed-by-the-place-it-is-used]]:** the Branch's leaf names
> its subject in a typed field, `http` with the HTTP step's name, not in the `http:<name>.<key>` prefix.

> Amends [[0039-outbound-calls-to-third-party-hooks]] (response data feeding later steps, left out of
> scope) and follows [[0136-a-wait-for-event-keeps-the-event-that-ended-it-for-later-steps]].

## Context

An HTTP step POSTs to a URL the author chose and moves on after a 2xx (ADR 0039); what the endpoint
answers is dropped, so an Automation cannot ask the shop for a recommendation, a coupon or a loyalty
balance and use it in the next email. Marketing platforms offer this, as data fetched from an external
API into a message or a webhook action whose response is saved for later steps.

ADR 0136 already keeps an Event that ended a Wait for event on the Enrollment, under the Step's name,
for templates and Branches to read, marked untrusted (ADR 0132) and cleared when the Enrollment ends.
A response is the same kind of data: something the Enrollment received on its way.

## Decisions

- **An HTTP step has a name**, a key fixed when the Step is created, unique in the Automation, of the
  form a Wait for event's name has. Its response is kept on the Enrollment under it, taken once.
- **What is kept is a JSON object and the status.** A 2xx response whose body is a JSON object within
  the bounds an Event's properties have (size and depth) is kept with its status code. Any other body
  (not JSON, not an object, too large) is not kept: the call still succeeded, `http.<name>` holds only
  the status, and the Step's report says why the body was dropped. A non-2xx response still fails the
  Enrollment (ADR 0039).
- **Later Steps read it as `http.<name>`**: `{{ http.<name>.status }}` and
  `{{ http.<name>.body.<key> }}` in every later send and HTTP step and in a Notify; a Set field may
  take a property of it beside a literal, `event` and `awaited`. Where there is no copy (a path that
  never passed the Step, a dropped body) it renders nothing, as `awaited` does, and the editor and the
  Content check warn as they do for `awaited`.
- **A Branch tests it** with a leaf `http:<name>.<key>`, two-valued (no copy or no such key fails the
  test), naming only an HTTP step that can come before the Branch (ADR 0122). A Segment, a Rule, a Wait
  for event's Condition and an Event matcher's filter refuse it. An Enroll step does not carry it.
- **It is untrusted.** Its values are `@untrusted` (ADR 0132) and escaped like any merge tag. Publish of
  an Automation whose send Step reads `http.*` lists the hosts whose responses reach its messages, as a
  series' Publish lists its feed's hosts. No further gate: a Member chose the host.
- **It lives as long as the Enrollment runs**: cleared when it ends, deleted by an Erasure, moved by a
  Merge with its Enrollment; each Enrollment of a Contact enrolled again has its own.

## Consequences

- An HTTP step gains `name`; the Enrollment keeps responses beside its awaited Events; templates gain
  `http`; Conditions gain the `http:` leaf in Branches; Set field gains the source.
- The Enrollment resource returns the kept responses marked untrusted.
- Glossary: the HTTP step entry.

## Rejected

- **Any body kept as text**: a template could print an error page or a stack trace into a message.
- **Failing the Step on a body that cannot be kept**: the call did what it was for; a body the author
  does not read would stop Enrollments for nothing.
