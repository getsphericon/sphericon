# Tracking visitors and events

Tracking turns anonymous visits into contacts you can segment on. A small script runs in the
browser, assigns each device a visitor id, and sends events to your Sphericon instance.

## Add the tracker

Add the script to your pages. The collect key and a ready-made test request are in the tracking
section of the workspace **Settings**.

```html
<script
  async
  src="https://sphericon.example.com/t.js"
  data-collect-key="YOUR_COLLECT_KEY"
  data-collect-url="https://sphericon.example.com"
></script>
```

The tracker stores a first-party `om_vid` cookie as the visitor id. It batches events and
flushes them shortly after they are queued and when the page is hidden.

## Send events and identify people

Commands queue on `window._omq`, so you can call them before the script has loaded.

```js
window._omq = window._omq || []

// Record something a visitor did
_omq.push(['track', 'viewed_pricing', { plan: 'team' }])

// Tell sphericon who this visitor is
_omq.push(['identify', { email: 'ada@example.com', subjectId: 'user_42' }])
```

`identify` binds the visitor to a contact and asserts its alias keys: `subjectId`, `email` and
`phone`. Everything that visitor did earlier, while anonymous, is stitched onto the contact, so
pre-signup behavior becomes visible to segments.

The same contract is available as an HTTP API if you would rather send from your server, using
the collect key in an `x-collect-key` header:

| Endpoint                 | Purpose                                      |
| ------------------------ | -------------------------------------------- |
| `POST /collect/identify` | Bind a `visitorId` to alias keys and traits. |
| `POST /collect/events`   | Record a batch of events for visitors.       |

## Retrying from a server client

`/collect` is safe to retry. Give every event in `POST /collect/events` a `clientEventId`: an
id you choose, unique within the workspace (a UUID), generated once when the event happens and
reused on every retry.

```json
{
  "events": [
    {
      "visitorId": "v_1",
      "action": "viewed_pricing",
      "clientEventId": "0b0a3c2e-6f0e-4d0e-9a41-5d0c2f8f6f11"
    }
  ]
}
```

- A repeat of an id in the same workspace is accepted (`204`) and changes nothing: the event is
  counted once, whether the first attempt reached Sphericon or not. Resend a whole batch rather
  than working out which events arrived. Ids are per workspace, so two workspaces may use the
  same id. An event without a `clientEventId` is not deduplicated.
- When the database cannot be reached, `/collect` answers `503` with a `Retry-After` header (in
  seconds) and the request changes nothing. Wait that long, then send the same batch with the same
  ids; back off exponentially if it keeps answering `503`. A restart of a single-host instance
  takes seconds, so a few retries cover it.
- `429` also carries `Retry-After`; retry it the same way. Other `4xx` answers are not retryable:
  fix the request.

The browser tracker (`t.js`) does this for you: it gives every event a `clientEventId`, keeps
events it could not send (network error, `429`, `5xx`) in a bounded `localStorage` queue of 100
events (the oldest is dropped first), retries them with backoff (honouring `Retry-After`) and sends
what is left on the next page load.

## Contacts and identity

A contact is anchored by an internal id. Email, phone and `subject_id` are alias keys: each is
unique within the workspace and any of them may be absent. Events attach to the resolved
contact at ingest, never by email, so changing someone's email does not orphan their history.

## Custom fields

Any trait or property you send under an unknown key is created as a typed **custom field** with
an inferred type. From that moment it is a first-class, renameable field the segment builder
offers, not an anonymous bag of keys.

## Server-side events

Events you record from your backend use the API with a bearer token and the `events:write`
scope:

```sh
curl https://sphericon.example.com/api/events/batch \
  -H "Authorization: Bearer $SPHERICON_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"events":[{"action":"subscription_started","subjectId":"user_42"}]}'
```

### Loading history into a new setup

To bring the events of an earlier system into a new Workspace, send them to `POST /events/batch`
in chunks of up to 1000 with `"backfill": true`, each with its original `occurredAt` and a
`clientEventId` of your own (a UUID, or your system's event id):

```sh
curl https://sphericon.example.com/api/events/batch \
  -H "Authorization: Bearer $SPHERICON_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"backfill":true,"events":[{"action":"order_paid","subjectId":"user_42","occurredAt":"2024-03-01T10:00:00Z","clientEventId":"order-1001"}]}'
```

- A backfilled event is recorded and segments see it, but no Automation enrolls on it and no
  webhook endpoint receives it, so loading history never sends mail.
- Every backfilled event needs an `occurredAt` that has already passed.
- Without `backfill`, an event whose `occurredAt` is more than 72 hours old is refused with a
  message that names `backfill` (the window is `SPHERICON_EVENT_LIVE_WINDOW_HOURS`). `/collect` is
  not limited.
- An event with a `clientEventId` that is already stored changes nothing, so a load that broke off
  halfway is resent from the start. The id is unique within the Workspace.
- Actions starting with `email.`, `marketing.` or `contact.` belong to sphericon and are refused
  from every event you send.
- Retention counts from the day an event is stored, not from `occurredAt`, so history older than
  the window is kept until it has been stored for that long.
- Load the Contacts first: an event whose subject matches no Contact is stored anonymously and is
  not linked afterwards.

See [API](/guide/api) for tokens and the other endpoints.
