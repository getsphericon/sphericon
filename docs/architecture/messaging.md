# Outbound calls and email rendering

[Architecture](../architecture.md) → Outbound calls and email rendering

Email via `internal/infra/messaging`
(per-workspace providers: smtp/ses). Both providers share `messaging.BuildMIME`
(wneessen/go-mail) — smtp sends the `*mail.Msg` directly, ses serializes it to raw
bytes for SES v2 `SendEmail` (raw content).

## Retry decisions

**Retries of an outbound call**: whether a failed call to a third party waits, retries or gives up
is decided once, by `httpcall.Policy.Decide` (ADR 0039), from the send error, the attempt
(`httpcall.Attempt`) and the clock. A 429 waits for its `Retry-After` without spending an
attempt until the retry horizon has passed. An HTTP step and a Rule's HTTP Action are one-off
calls (`httpcall.OneOff`): they retry only what another attempt can fix. A Webhook delivery is a
subscription (`httpcall.Subscription`): it retries any failure until its attempts run out. Each
caller keeps only its own side effects: the failure it records, and the snooze, cancellation or
retry its job turns the decision into.

## Automation HTTP Steps

**Automation HTTP Steps**: `automations.StepExecution.ExecuteHTTP` (`internal/domain/orchestration/automations`) selects the Enrollment's pinned Step,
checks its lifecycle and position, opens and renders its stored call, and executes it through
the protected HTTP adapter under the Enrollment's exit fence. Success advances and queues the
next Step in one transaction; final failure records the Enrollment's failure. The one-off
call's decision becomes mutually exclusive retry, cancellation or wait decisions (a call given up
on its last attempt is a final retry, which the queue ends as exhausted; any other give-up
cancels); the River worker only establishes Workspace scope and translates those decisions. A rate-limit
wait spends no ordinary attempt. The stable external request identity is preserved, but a call
accepted before a failed advancement may be repeated.

## Rule HTTP Actions

**Rule HTTP Actions**: `rules.Module.ExecuteHTTP` selects the pending Action of a Firing,
opens its secrets, renders its Contact and Event, executes the protected call and records the
result. The module holds the protected HTTP adapter and the Instance clock from `rules.New`, as
the Step execution does from `automations.NewStepExecution`; each call receives only ordinary attempt data
(`httpcall.Attempt`) and returns a retry or wait decision without River types. The River
worker loads the Firing to establish Workspace scope and translates that decision. A final
failure belongs to the Action's result; it does not cancel the Firing's other Actions or adopt
Enrollment lifecycle rules. The call retains its stable request identity and existing delivery
guarantees: a request accepted before a failed result write may be repeated.

## Email rendering

`internal/kernel/emaildoc` (ADR 0040) turns an editor document into email HTML inside the binary: the editor's own
export code, bundled by `packages/emaildoc` (`mise run build:emaildoc`; rerun it after any change to the pinned
`@react-email/editor`, then `go test ./internal/kernel/emaildoc -update` to accept the new golden HTML) and run in goja,
no cgo. The server starts the runtime in the background at boot (`Warm`; a library caller's first call does it otherwise), limits are enforced before and during a render. Editor and
bundle dependencies in `packages/emaildoc/package.json` are pinned exactly.

## Tracker bundle

The tracker snippet (`/t.js`) is the `@sphericon/analytics` IIFE bundle, built and embedded:
`mise run build:tracker` copies `packages/analytics/dist/t.js` into `internal/server/assets/`.
