# Answering a GDPR request

If you send to people in the EU, your Workspace is the data controller and sphericon is your processor.
When a contact asks what you hold about them (access, GDPR Art. 15/20) or asks you to delete them
(erasure, Art. 17), you answer it from your Workspace. There is no self-service form for the
contact: both actions are done by your own team. The design is recorded in
[ADR 0021](https://github.com/getsphericon/sphericon/blob/main/docs/adr/0021-gdpr-erasure-and-export.md).

## Access request: export one contact

Export produces one JSON file with everything held about a single contact. It is streamed as a
download, so a contact with many events is fine.

- **Site UI:** open the contact and choose **Export**. Any Workspace member who can read contacts
  can do it.
- **External API:** `POST /api/contacts/export?id=<id>` or `POST /api/contacts/export?email=<address>`
  (exactly one of the two). The token needs the `contacts:read` scope. MCP exposes the same
  operation.

The file contains the contact (`contact`), its `tags`, `visitors`, all `events`, the `unsubscribes`,
`suppressions` and `confirmations` for its addresses, and the delivery metadata of its
`outboundMessages` and `broadcastRecipients`. Rendered message bodies are not included: the file
holds the person's data, not your campaign content.

The file follows a documented schema, `ContactExportDocument` in the OpenAPI document: keys are
camelCase, ids are strings, timestamps are RFC 3339, and a value the contact has none of is left
out. The Workspace id and internal references (idempotency keys, run ids) are not part of it.

## Erasure request: erase one contact

Deleting a contact is erasure. There is no softer delete that keeps personal data, and no way to
undo it.

- **Site UI:** open the contact and choose **Erase**, then confirm. Only Workspace owners and admins
  see the action; anyone else is refused.
- **External API, by id:** `DELETE /api/contacts/{id}` with a token that has the `contacts:erase`
  scope. A token with only `contacts:write` is refused. MCP inherits the scope (`contacts_delete`).
- **External API, by email or visitor id:** `DELETE /api/contacts/erase?email=<address>` or
  `DELETE /api/contacts/erase?visitorId=<id>` (exactly one of the two), under the same
  `contacts:erase` scope (MCP: `contacts_erase_by`). By email it erases the contact with that
  address and also anonymizes the delivery records of an address that never had a contact (for
  example a one-off transactional send). By `visitorId` it erases the contact the visitor is bound
  to, or, for an anonymous visitor that never identified, the visitor and its events. An
  identifier that matches nothing in the Workspace is reported as not found.

The Site UI erases a contact by id only. Erasure is synchronous and atomic: it either completes or
changes nothing, and it returns an empty success. A contact in another Workspace is reported as not
found.

### Work already under way

Erasure also stops what was about to happen to the person, in the same transaction:

- The contact's automation runs are deleted, so no further step is sent.
- The contact's recipients in Broadcasts that have not been sent are removed, and the Broadcast's
  recipient total shrinks with them. A Broadcast left with nothing pending settles as sent.
- Queued background work that refers to the contact or its address is deleted from the internal
  queues: pending domain events (the outbox), queued automation work, queued Broadcast sends and
  queued webhook deliveries whose payload names the contact or its address, and queued HTTP
  steps of its automation runs. A failed HTTP step's response excerpt is kept on the run, so it is
  deleted with the run. A job that is already running is not interrupted.
- A send that was queued just before the request is dropped at the last check before delivery,
  because the contact no longer exists. A message already handed to the email provider is not
  recalled.

### What Erasure removes, anonymizes and keeps

| Data                                                     | Result                                                                                             |
| -------------------------------------------------------- | -------------------------------------------------------------------------------------------------- |
| The contact, its tags and custom field values            | Deleted                                                                                            |
| Visitors (anonymous devices) bound to the contact        | Deleted                                                                                            |
| Events you tracked for the contact                       | Deleted                                                                                            |
| `marketing.confirmed` events and the confirmation record | Deleted (a new confirmation is needed if the person returns)                                       |
| Automation runs                                          | Deleted                                                                                            |
| System events: `email.*` and `contact.created`           | Kept as anonymous rows: identity cleared, properties reduced to the sending domain and bounce kind |
| Outbound messages and broadcast recipients               | Kept as anonymous rows: address and contact reference cleared                                      |
| Unsubscribes and suppressions                            | Kept with the plain address, contact reference cleared                                             |

The anonymous rows keep their status and timing, so your complaint rate, bounce rate and billable
usage do not change after a request. Unsubscribes and suppressions are the minimum kept to keep
honoring the person's refusal: if the same person is imported or identified again they become a new,
clean contact, but mail to an address they opted out of is still refused. Erasure does not block
re-creating a contact.

Application logs carry no email addresses or phone numbers, so there is nothing to scrub there.

## Downstream cleanup: the `contact.erased` webhook

Each Erasure records one immutable `contact.erased` event: when it happened, which requester (a user
or an API token) made it, and what kind of identifier was used. The stored event holds no email and
no contact id, so it is not personal data; keep it as your proof that the request was honored.

To let you erase your own copies (CRM, warehouse, product database), the same event is delivered to
your [webhook endpoints](/guide/api#webhooks) (an endpoint with no event filter receives it, or
select `contact.erased`). The delivery body is the standard webhook payload; its `data` carries:

```json
{
  "type": "contact.erased",
  "workspaceId": 42,
  "data": {
    "workspaceId": 42,
    "subjectId": "user-1234",
    "identifierKind": "contact_id",
    "requesterKind": "api_token",
    "requesterId": 7
  }
}
```

`data.subjectId` is the contact's own `subject_id` (your user id for the person) or, when the
contact had none, its sphericon contact id. It exists only in the delivered payload and is never stored
on the event. Deliveries are retried, so a receiver may see duplicates; dedupe on the delivery id
(the `webhook-id` header).

## Backups

Erasure does not rewrite backups. A backup taken before a request still contains the person until
it ages out under your backup rotation. Two obligations follow:

1. **State your rotation window.** Know how long your backups are kept (on a self-hosted instance
   this is your own policy) and say so in your privacy policy.
2. **A restore must re-apply erasures.** After restoring a backup, erase again every contact erased
   since that backup was taken, before the data is used. The ledger is the `contact.erased` events
   recorded in the Workspace's event log (action `contact.erased`; the external API only writes events, so read them from the database or the Site app) together with your own request
   records. Because the stored events deliberately carry no identifier, map each one back to the
   person through your request records, or through the `subjectId` your webhook receivers logged.
