# API

sphericon exposes three HTTP surfaces. All are described by OpenAPI documents generated from
TypeSpec, which is the single source of truth; you will find them in the repository under
`openapi/`. Every operation of the external API is documented in the [API reference](/api/).

| Surface      | Base path  | Used by                             | Auth                            |
| ------------ | ---------- | ----------------------------------- | ------------------------------- |
| **External** | `/api`     | Your backend and automation         | `Authorization: Bearer <token>` |
| **Collect**  | `/collect` | The browser tracker and your server | `x-collect-key` header          |
| **Site**     | `/site`    | The sphericon web app itself        | Session, per workspace          |

The site API is the web app's own backend and may change with the UI. Build on the external and
collect APIs.

## Tokens and scopes

Create an API token under **Settings → Developers**. Each token carries scopes, so you can hand an
integration only what it needs, for example:

- `contacts:read`, `contacts:write`, and the separate `contacts:erase` to delete (erase) a contact
- `events:write`
- `segments:read`, `segments:write`
- `broadcasts:write`, and the separate `broadcasts:send` to actually send
- `automations:write`, and `automations:activate` to turn one on
- `enrollments:read` to read where each Contact is in an automation, and why a failed one failed
- `emails:send` for transactional mail
- `webhooks:read`, `webhooks:write`, and the separate `webhooks:regenerate` (replace an endpoint's signing secret) and `webhooks:test` (send it a sample event)
- `integrations:read`, `integrations:write` for sending providers (credentials are write-only: no response ever contains a password or secret key)
- `sending_domains:read`, `sending_domains:write` for sending domains: create one, publish the returned DKIM record, then `POST /sending-domains/{id}/verify` and read the domain to see whether it verified (the DKIM private key is never returned)

Sending, activating and erasing have their own scopes, so a token that can edit drafts cannot send mail and a token that can write contacts cannot erase them.

## What the external API covers

| Area                | Endpoints                                                            |
| ------------------- | -------------------------------------------------------------------- |
| Contacts            | `/contacts`, `/contacts/batch`, `/contacts/{id}/tags`                |
| Custom fields, tags | `/custom-fields`, `/tags`                                            |
| Events              | `/events`, `/events/batch`, `/events/actions`                        |
| Segments            | `/segments`, `/segments/preview`                                     |
| Templates           | `/templates`                                                         |
| Broadcasts          | `/broadcasts`, `…/audience`, `…/schedule`, `…/test-send`, `…/report` |
| Automations         | `/automations`, `…/activate`, `…/deactivate`                         |
| Enrollments         | `/automations/{id}/enrollments`                                      |
| Transactional       | `/emails`                                                            |
| Consent             | `/unsubscribes`, `/suppressions`                                     |
| Sending domains     | `/sending-domains`, `/sending-domains/rates`                         |
| Webhooks            | `/webhooks`                                                          |

### Example: upsert contacts

```sh
curl https://sphericon.example.com/api/contacts/batch \
  -H "Authorization: Bearer $SPHERICON_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"contacts":[{"subjectId":"user_42","email":"ada@example.com","firstName":"Ada"}]}'
```

## Pagination

List endpoints take `page` (1-based, default 1) and `pageSize` (default 25) and return:

```json
{ "items": [], "page": 1, "pageSize": 25, "totalItems": 0, "totalPages": 0 }
```

## Batches

`POST /contacts/batch` and `POST /events/batch` take up to 1000 items. Items are processed
independently: one failure does not roll back the others. The response has one result per item,
in request order, with its zero-based `index`:

```json
{
  "results": [
    { "index": 0, "status": "created", "contactId": 101 },
    { "index": 1, "status": "failed", "error": "email is invalid" }
  ]
}
```

Contact items have the status `created`, `updated` or `failed`; event items `accepted` or
`failed`. Contacts are matched by their alias keys (`subjectId`, `email`, `phone`), so a batch is
safe to repeat.

## Errors

Errors are [RFC 7807](https://www.rfc-editor.org/rfc/rfc7807) `application/problem+json`:

```json
{
  "status": 422,
  "title": "Unprocessable Entity",
  "detail": "…what went wrong…"
}
```

`status` and `title` are always present, and `detail` for client errors. Server errors (`5xx`)
carry no detail; quote the `X-Request-Id` response header when you report one.

| Status | Meaning                                                              |
| ------ | -------------------------------------------------------------------- |
| `400`  | The request could not be parsed.                                     |
| `401`  | The token is missing, invalid, revoked or expired.                   |
| `403`  | The token lacks the required scope.                                  |
| `404`  | The resource does not exist in this workspace.                       |
| `413`  | The body is larger than the configured limit.                        |
| `409`  | A request with the same `Idempotency-Key` is still in flight.        |
| `422`  | Validation failed, or a reference points at another workspace's row. |

## Idempotent sends

`POST /emails` accepts an `Idempotency-Key` header. Repeating a request with the same key replays
the original result without sending a second email, so it is safe to retry after a timeout. A
repeat that arrives while the first is still running gets `409`.

## Operating sphericon with an agent

The same binary serves an MCP endpoint at `/mcp`. Its tools are projected from the external API
contract, so token scopes, workspace scoping and errors behave exactly as they do over `/api`.
See [MCP for agents](/guide/mcp).

## Webhooks

Register an endpoint with `POST /api/webhooks` to be notified when things happen. See
[Webhooks](/guide/webhooks) for the events, payload, signature check and retries.
