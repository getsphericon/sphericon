# Webhooks

Webhooks push things that happen in a workspace to your own HTTP endpoint, so you can react
without polling.

## Register an endpoint

Create one with `POST /api/webhooks` (token scope `webhooks:write`) or in the app:

```sh
curl https://sphericon.example.com/api/webhooks \
  -H "Authorization: Bearer $SPHERICON_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"url":"https://app.example.com/hooks/sphericon","eventTypes":["email.bounced","email.complained"]}'
```

`eventTypes` filters by event name; leave it empty to receive everything. `enabled: false` pauses
an endpoint without deleting it. The URL must be an absolute `http` or `https` address.

The signing secret is never returned by the API. Read it in the app, where the endpoint is shown.

## Replace the secret and send a test event

If a secret leaks, replace it: `POST /api/webhooks/{id}/regenerate-secret` (token scope
`webhooks:regenerate`) or **Regenerate secret** in the app. The old secret stops verifying at
once, so update your receiver with the new one (shown in the app) first or right after.

To check a URL and your signature verification before a real event, send a sample:
`POST /api/webhooks/{id}/test` with `{"eventType":"contact.created"}` (token scope
`webhooks:test`), or **Send test event** in the app. The sample is signed like a real delivery and
sent at once, even to a disabled endpoint. The response says whether the receiver answered 2xx
(`delivered`), its `status`, and otherwise the error `kind` (`timeout`, `blocked_address`,
`http_status` or `network`). A test never changes the endpoint's failure state. Both actions are
recorded in the Audit log.

## Events

| Event                 | When                                                |
| --------------------- | --------------------------------------------------- |
| `contact.created`     | A new contact is created.                           |
| `email.sent`          | A message was handed to the transport.              |
| `email.opened`        | A recipient opened a message.                       |
| `email.clicked`       | A recipient clicked a tracked link.                 |
| `email.unsubscribed`  | A recipient unsubscribed.                           |
| `email.bounced`       | A message hard-bounced.                             |
| `email.complained`    | A recipient reported a message as spam.             |
| `marketing.confirmed` | A contact confirmed their double opt-in.            |
| _your own actions_    | Every event you record, under the action name used. |

Events you send through `/api/events` or the tracker are delivered under their own action name,
for example `page_view` or `signed_up`, so an endpoint can subscribe to exactly the ones it needs.

## Payload

Each delivery is a `POST` with a JSON body:

```json
{
  "id": "0b6f3f0e-…",
  "type": "email.clicked",
  "occurredAt": "2026-10-09T12:30:00Z",
  "workspaceId": 7,
  "subject": "ada@example.com",
  "contactId": 42,
  "data": {}
}
```

`data` carries the details specific to the event type (for example the message and link for a
click, or the properties of a custom event). `contactId` is absent for events with no contact yet,
such as an anonymous page view.

Headers:

| Header              | Meaning                                       |
| ------------------- | --------------------------------------------- |
| `webhook-id`        | Unique delivery id. Use it to deduplicate.    |
| `webhook-timestamp` | Unix seconds when the delivery was signed.    |
| `webhook-signature` | HMAC signature of the body.                   |
| `X-sphericon-Event` | The event name, for routing before you parse. |

## Verify the signature

Signatures follow the [Standard Webhooks](https://www.standardwebhooks.com) scheme, so any of its
[libraries](https://github.com/standard-webhooks/standard-webhooks) verifies a delivery: pass it
the raw request body, the three `webhook-*` headers and your endpoint secret. Reject the request
if verification fails, and use the timestamp to refuse stale replays.

## Delivery and retries

- Respond with any `2xx` status to acknowledge. Anything else, or a connection failure, is retried
  (up to 10 attempts), so your handler may see the **same delivery more than once**. Deduplicate
  on `webhook-id`.
- Redirects are not followed; a `3xx` counts as a failure.
- Endpoints that resolve to private, loopback, link-local or cloud-metadata addresses are refused,
  so you cannot point a webhook at an internal service. Use a public URL, or a tunnel for local
  development.
- An endpoint disables itself, and the workspace owners are emailed, when it answers `410 Gone`, or
  when at least 20 deliveries in a row have failed over more than 2 hours. The workspace's Audit
  log records the change by the `system` actor. Enabling the endpoint again clears its failure
  history.
- Deliveries run on the same durable PostgreSQL job queue as sending, so a restart does not lose
  them. There is no ordering guarantee between events.
