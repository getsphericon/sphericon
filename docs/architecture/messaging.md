# Outbound calls and email rendering

[Architecture](../architecture.md) → Outbound calls and email rendering

Email via `internal/infra/messaging`
(per-workspace providers: smtp/ses). Both providers send the bytes of
`messaging.BuildSignedMessage`: wneessen/go-mail builds the MIME message unsigned, and
emersion/go-msgauth DKIM-signs its bytes with the key of the From's verified Sending domain
(ADR 0010), relaxed header and body canonicalization, the one-click headers in the signed set
on marketing mail (ADR 0012). smtp sends them through the SMTP client go-mail dials, ses as
the raw content of SES v2 `SendEmail`. System mail has no keys and goes out unsigned.

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

## Broadcast send jobs

**Broadcast send jobs** (ADR 0107, ADR 0111): `broadcasts.Delivery.RunQueued` runs a plan job
and `DeliverRecipient` one attempt at a recipient's job (`broadcasts.Attempt`, the number and
the limit). The module decides what the run comes to, as a `broadcasts.Outcome` without River
types: a job superseded by a reschedule or stale (unscheduled, sent or failed meanwhile) is done,
a hold, a Deferral or a live claim waits the time it asks for, any other failure spends an
attempt, and on the last attempt a failed recipient is given up so the Broadcast settles. The
River workers load the row that names the Workspace, hand over the attempt and turn the outcome
into a snooze or an error; the synchronous adapter runs every send as its last attempt.

## Email rendering

`internal/kernel/emaildoc` (ADR 0040) turns an editor document into email HTML inside the binary: the editor's own
export code, bundled by `packages/emaildoc` (`mise run build:emaildoc`; rerun it after any change to the pinned
`@react-email/editor`, then `go test ./internal/kernel/emaildoc -update` to accept the new golden HTML) and run in goja,
no cgo. The server starts the runtime in the background at boot (`Warm`; a library caller's first call does it otherwise), limits are enforced before and during a render. Editor and
bundle dependencies in `packages/emaildoc/package.json` are pinned exactly.

## Tracker bundle

The tracker snippet (`/t.js`) is the `@sphericon/analytics` IIFE bundle, built and embedded:
`mise run build:tracker` copies `packages/analytics/dist/t.js` into `internal/server/assets/`.
