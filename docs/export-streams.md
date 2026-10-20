# Export streams

Export streams are an Enterprise feature that lets you keep your own copy of a Workspace's data in
a warehouse such as ClickHouse. There are five streams, read over the external API with a Bearer
token that has the `export:read` scope:

| Stream            | Endpoint                   | What a row is                                               |
| ----------------- | -------------------------- | ----------------------------------------------------------- |
| Events            | `GET /api/export/events`   | One tracked Event                                           |
| Outbound messages | `GET /api/export/messages` | The delivery record of one send, without subject or body    |
| Contacts          | `GET /api/export/contacts` | One Contact, with its Custom fields and Tag names           |
| Erasures          | `GET /api/export/erasures` | The keys one Erasure matched on: your signal to delete rows |
| Audit             | `GET /api/export/audit`    | One Audit entry (needs the `audit` feature as well)         |

The design and the reasons behind it are in
[ADR 0045](https://github.com/getsphericon/sphericon/blob/main/docs/adr/0045-export-streams-are-a-pull-api-with-a-time-cursor.md).
Without the `export` license feature (and, for Audit, the `audit` feature) the endpoints answer
`404`; a token without the scope gets `401`; a cursor this API did not issue gets `400`. A
suspended Workspace keeps its export.

## How reading works

A request takes two optional query parameters:

- `cursor`: the value of the `X-Next-Cursor` header of the previous page. Empty or missing reads
  from the beginning of the stream.
- `limit`: the page size, 1 to 10000, 1000 by default.

The response is `application/x-ndjson`: one JSON object per line, no envelope, so the body loads
as ClickHouse `JSONEachRow` as it is. Rows come oldest change first. The next cursor is in the
`X-Next-Cursor` response header.

Every key of a row is always present; an absent value is `null`. Ids are decimal strings (`"42"`)
and timestamps are RFC 3339 in UTC.

### The cursor

The cursor is opaque. It is a position in `(updated_at, id)` order, and the token points just
after the last row of the page: the next page starts with the first row you have not seen, and a
row that shares its `updated_at` with the last row of a page is neither skipped nor repeated. A
page with no rows returns the cursor you sent, so a poller can loop on `X-Next-Cursor` without a
special case: read until a page is empty, store the cursor, and read again later.

Because the cursor is opaque, you cannot build "ten minutes ago" from the clock. A consumer that
wants the overlap below must keep an older cursor.

### Rows change, so a row can come again

The cursor column is `updated_at`, not `created_at`. A row that changes gets a newer `updated_at`
and is delivered again: Identify stitching an anonymous Event onto a Contact, an Erasure
anonymizing the `email.*` Events and delivery rows of a person, a Tag attached to a Contact, a
delivery whose status moves from `pending` to `sent`. The version with the newest `updated_at` is
the current one.

## Your duties as a consumer

Delivery is at-least-once, with no ordering guarantee across commits. Two rules make it correct,
and both are yours to keep:

1. **Overlap: re-read a trailing window of at least 10 minutes on every run.** A transaction can
   commit after a cursor already moved past its rows, because `updated_at` is the time the
   application changed the row, not the time it committed (a long Broadcast or Erasure, or clock
   skew between instances, sets how far back a late row lands). Sphericon holds no lock and
   publishes no safe horizon for this, since either would slow tracking down. So do not start a
   run from the cursor of the last page. Keep a short history of checkpoints, each the
   `updated_at` of the last row of a page together with the cursor after that page, and start from
   the newest checkpoint that is at least 10 minutes older than the newest `updated_at` you have
   read. A longer overlap is safer; it only costs re-reading rows.
2. **Deduplicate by `id`, keeping the newest `updated_at`.** The overlap, a re-seed and a changed
   row all deliver a row you may already hold. In ClickHouse that is a `ReplacingMergeTree`
   keyed by `id` and versioned by `updated_at`, read with `FINAL`.

A consumer that does not deduplicate sees duplicates and old versions.

## Erasures: delete what was erased

Erasure deletes a person's customer-tracked Events and anonymizes the rest, and a cursor cannot
show a row that is gone. So a removal is announced, never silent: every Erasure adds a row to the
Erasures stream, and you delete the same rows from your copy. Match on any of the keys:

- `contact_id` on Events, Contacts and Outbound messages;
- `subject_id` on Events, when it is not empty (anonymous Events have an empty `subject_id`);
- `visitor_ids` on the `visitor_id` of Events;
- `email_sha256`, the SHA-256 as lowercase hex of the lowercased address, and `phone_sha256`, the
  SHA-256 of the phone number as stored.

An Erasure that named only an address, or an anonymous Visitor with no Contact, has no
`contact_id`, an empty `subject_id` and no visitor ids: an address-only Erasure carries **hashes
only**. The address and the phone themselves are never in the stream, so hash the values in your
own copy the same way to find the rows (`lower(hex(SHA256(lower(email))))` in ClickHouse).

Erasure rows are kept for the instance's Event retention window
(`SPHERICON_EVENTS_RETENTION_DAYS`, 400 days by default), and for good when retention is off.
Deletion by retention is not announced: your copy follows your own retention. Apply the erasures
after loading the other streams, on every run: deleting twice is harmless.

### After an outage: re-seed

A consumer that was away longer than the retention window may have missed Erasure rows that have
since expired, and the Events they refer to are gone from a fresh read anyway. Do not continue
from the old cursor. Empty your copy, read every stream again from an empty cursor and load it.
A new consumer back-fills the same way.

## Columns of each stream

These are the columns of the contract in the OpenAPI document (`ExportEventRow` and the others in
the [API reference](/api/)). `null` means the key is present with no value.

### Events

| Column        | Type             | Meaning                                                             |
| ------------- | ---------------- | ------------------------------------------------------------------- |
| `id`          | id               | Unique identifier                                                   |
| `action`      | string           | Event action                                                        |
| `properties`  | JSON object      | Event properties                                                    |
| `contact_id`  | id or `null`     | The Contact the Event is about; `null` for an anonymous Event       |
| `visitor_id`  | string or `null` | The anonymous device the Event came from                            |
| `subject_id`  | string           | Canonical subject identifier snapshot; empty for an anonymous Event |
| `email`       | string or `null` | Email address snapshot; `null` once anonymized                      |
| `phone`       | string or `null` | Phone number snapshot; `null` once anonymized                       |
| `occurred_at` | time or `null`   | When the Event happened, as reported by the sender                  |
| `created_at`  | time             | Creation timestamp                                                  |
| `updated_at`  | time             | Last change timestamp: the cursor column and the version of the row |

### Outbound messages

The delivery record of one send, whichever surface asked for it. It has provenance and outcome,
never a subject or a body.

| Column                | Type              | Meaning                                                                            |
| --------------------- | ----------------- | ---------------------------------------------------------------------------------- |
| `id`                  | id                | Unique identifier                                                                  |
| `kind`                | string            | Which surface asked for the send: `broadcast`, `automation` or `transactional`     |
| `channel`             | string            | Delivery channel                                                                   |
| `destination`         | string or `null`  | Lowercased destination address; `null` once an Erasure anonymized the row          |
| `status`              | string            | `pending` (claimed, provider not yet answered), `sent`, `skipped` or `failed`      |
| `reason`              | string or `null`  | Why the message was skipped, or the failure text                                   |
| `contact_id`          | id or `null`      | The Contact the destination resolved to; `null` for a stranger or after an Erasure |
| `broadcast_id`        | id or `null`      | The Broadcast the message belongs to                                               |
| `automation_id`       | id or `null`      | The Automation the message belongs to                                              |
| `automation_run_id`   | id or `null`      | The Automation run that sent the message                                           |
| `automation_step`     | integer or `null` | The step of the Automation that sent the message                                   |
| `integration_id`      | id or `null`      | The Integration the message went out through                                       |
| `template_id`         | id or `null`      | The Template a transactional send referenced                                       |
| `sending_source`      | string or `null`  | The unsubscribe Sending source; `null` for a transactional send                    |
| `sending_domain`      | string or `null`  | Domain of the effective From address                                               |
| `provider_message_id` | string or `null`  | The provider's id for the accepted message                                         |
| `claimed_at`          | time              | When the send was claimed, before the provider was called                          |
| `sent_at`             | time or `null`    | When the provider accepted the message                                             |
| `created_at`          | time              | Creation timestamp                                                                 |
| `updated_at`          | time              | Last change timestamp: the cursor column and the version of the row                |

### Contacts

| Column          | Type             | Meaning                                                                  |
| --------------- | ---------------- | ------------------------------------------------------------------------ |
| `id`            | id               | Unique identifier                                                        |
| `subject_id`    | string or `null` | The customer's own user id                                               |
| `email`         | string or `null` | Email address; `null` for an anonymous Contact                           |
| `phone`         | string or `null` | Phone number                                                             |
| `first_name`    | string or `null` | First name                                                               |
| `last_name`     | string or `null` | Last name                                                                |
| `time_zone`     | string or `null` | IANA time zone identifier                                                |
| `custom_fields` | JSON object      | Custom field values keyed by the field's machine key, as one JSON column |
| `tags`          | array of string  | Names of the Tags the Contact has, ordered by name                       |
| `created_at`    | time             | Creation timestamp                                                       |
| `updated_at`    | time             | Last change timestamp, a Tag change included: cursor column and version  |

### Erasures

A row is written once and never changes.

| Column         | Type             | Meaning                                                                                                                            |
| -------------- | ---------------- | ---------------------------------------------------------------------------------------------------------------------------------- |
| `id`           | id               | Unique identifier                                                                                                                  |
| `contact_id`   | id or `null`     | The erased Contact; `null` when the Erasure named an address or an anonymous Visitor with no Contact                               |
| `subject_id`   | string           | The customer's own id for the person (the Contact's `subject_id`, or its Sphericon id when it had none); empty when there was none |
| `visitor_ids`  | array of string  | The anonymous devices erased with the Contact                                                                                      |
| `email_sha256` | string or `null` | SHA-256, lowercase hex, of the lowercased email address                                                                            |
| `phone_sha256` | string or `null` | SHA-256, lowercase hex, of the phone number                                                                                        |
| `erased_at`    | time             | When the Erasure happened                                                                                                          |
| `created_at`   | time             | Creation timestamp                                                                                                                 |
| `updated_at`   | time             | Last change timestamp: the cursor column and the version of the row                                                                |

### Audit

Audit entries are append-only, so a row is delivered once (apart from overlap re-reads). A Staff
member's entry shows as `staff` with the name "Sphericon staff" and no id, as on every read
surface. This stream is for a machine: oldest first, with a cursor. The Audit log list in the app
is for a person.

| Column        | Type             | Meaning                                                             |
| ------------- | ---------------- | ------------------------------------------------------------------- |
| `id`          | id               | Unique identifier                                                   |
| `occurred_at` | time             | When the change happened                                            |
| `actor_kind`  | string           | Who did it: `user`, `api_token`, `staff` or `system`                |
| `actor_id`    | string or `null` | The actor's id; `null` for the system and for Staff                 |
| `actor_name`  | string or `null` | Display name of the actor when the entry was written                |
| `action`      | string           | What was done, as `<entity>.<verb>`                                 |
| `target_type` | string           | What it was done to                                                 |
| `target_id`   | string or `null` | The target's id                                                     |
| `target_name` | string or `null` | The target's name when the entry was written; `null` for a Contact  |
| `diff`        | JSON object      | Before/after of the changed fields; secrets appear as changed only  |
| `request_id`  | string or `null` | Request id of the change                                            |
| `ip`          | string or `null` | Client address of the change                                        |
| `user_agent`  | string or `null` | Client user agent of the change                                     |
| `created_at`  | time             | Creation timestamp                                                  |
| `updated_at`  | time             | Last change timestamp: the cursor column and the version of the row |

## ClickHouse: tables

One table per stream, with the engine `ReplacingMergeTree(updated_at)` ordered by `id`: when two
rows have the same `id`, a merge keeps the one with the newest `updated_at`. Merges run in the
background, so read with `FINAL` to see only the newest version at any moment. This DDL and the
loader below were run against ClickHouse 26.11.

JSON columns (`properties`, `custom_fields`, `diff`) are `String`: load them with
`input_format_json_read_objects_as_strings` and read them with `JSONExtract*`. Timestamps are
ISO 8601 with a `Z`, which ClickHouse reads with `date_time_input_format = 'best_effort'`. Quoted
ids load into `UInt64` as they are.

```sql
CREATE DATABASE IF NOT EXISTS sphericon;

CREATE TABLE sphericon.events
(
    id          UInt64,
    action      String,
    properties  String,
    contact_id  Nullable(UInt64),
    visitor_id  Nullable(String),
    subject_id  String,
    email       Nullable(String),
    phone       Nullable(String),
    occurred_at Nullable(DateTime64(6, 'UTC')),
    created_at  DateTime64(6, 'UTC'),
    updated_at  DateTime64(6, 'UTC')
)
ENGINE = ReplacingMergeTree(updated_at)
ORDER BY id;

CREATE TABLE sphericon.messages
(
    id                  UInt64,
    kind                LowCardinality(String),
    channel             LowCardinality(String),
    destination         Nullable(String),
    status              LowCardinality(String),
    reason              Nullable(String),
    contact_id          Nullable(UInt64),
    broadcast_id        Nullable(UInt64),
    automation_id       Nullable(UInt64),
    automation_run_id   Nullable(UInt64),
    automation_step     Nullable(Int32),
    integration_id      Nullable(UInt64),
    template_id         Nullable(UInt64),
    sending_source      Nullable(String),
    sending_domain      Nullable(String),
    provider_message_id Nullable(String),
    claimed_at          DateTime64(6, 'UTC'),
    sent_at             Nullable(DateTime64(6, 'UTC')),
    created_at          DateTime64(6, 'UTC'),
    updated_at          DateTime64(6, 'UTC')
)
ENGINE = ReplacingMergeTree(updated_at)
ORDER BY id;

CREATE TABLE sphericon.contacts
(
    id            UInt64,
    subject_id    Nullable(String),
    email         Nullable(String),
    phone         Nullable(String),
    first_name    Nullable(String),
    last_name     Nullable(String),
    time_zone     Nullable(String),
    custom_fields String,
    tags          Array(String),
    created_at    DateTime64(6, 'UTC'),
    updated_at    DateTime64(6, 'UTC')
)
ENGINE = ReplacingMergeTree(updated_at)
ORDER BY id;

CREATE TABLE sphericon.erasures
(
    id           UInt64,
    contact_id   Nullable(UInt64),
    subject_id   String,
    visitor_ids  Array(String),
    email_sha256 Nullable(String),
    phone_sha256 Nullable(String),
    erased_at    DateTime64(6, 'UTC'),
    created_at   DateTime64(6, 'UTC'),
    updated_at   DateTime64(6, 'UTC')
)
ENGINE = ReplacingMergeTree(updated_at)
ORDER BY id;

CREATE TABLE sphericon.audit
(
    id          UInt64,
    occurred_at DateTime64(6, 'UTC'),
    actor_kind  LowCardinality(String),
    actor_id    Nullable(String),
    actor_name  Nullable(String),
    action      String,
    target_type String,
    target_id   Nullable(String),
    target_name Nullable(String),
    diff        String,
    request_id  Nullable(String),
    ip          Nullable(String),
    user_agent  Nullable(String),
    created_at  DateTime64(6, 'UTC'),
    updated_at  DateTime64(6, 'UTC')
)
ENGINE = ReplacingMergeTree(updated_at)
ORDER BY id;
```

Read the current version of every row with `FINAL`:

```sql
SELECT action, count() FROM sphericon.events FINAL GROUP BY action;
```

## ClickHouse: loading a stream

The page body is `JSONEachRow`, so a page goes to ClickHouse unchanged as the body of an
`INSERT`. This script (Python 3, standard library only) copies one stream: it pages until an
empty page, inserts each page, and keeps the checkpoints that give it the 10 minute overlap on the
next run. Run it on a schedule (every minute is fine) for each of `events`, `messages`,
`contacts`, `erasures` and `audit`.

```python
#!/usr/bin/env python3
"""Copy one Sphericon Export stream into ClickHouse: sync_stream.py events"""

import json
import os
import sys
import urllib.parse
import urllib.request
from datetime import datetime, timedelta

API = os.environ["SPHERICON_API"]  # for example https://sphericon.example.com/api
TOKEN = os.environ["SPHERICON_TOKEN"]  # an API token with the export:read scope
CLICKHOUSE = os.environ.get("CLICKHOUSE_URL", "http://localhost:8123")
OVERLAP = timedelta(minutes=10)  # at least 10 minutes; longer is safer


def when(text):
    return datetime.fromisoformat(text.replace("Z", "+00:00"))


def start_cursor(checkpoints):
    """The newest checkpoint at least OVERLAP behind the newest row seen, or the beginning."""
    if not checkpoints:
        return "", []
    limit = when(checkpoints[-1][0]) - OVERLAP
    older = [i for i, (updated_at, _) in enumerate(checkpoints) if when(updated_at) <= limit]
    if not older:
        return "", checkpoints
    return checkpoints[older[-1]][1], checkpoints[older[-1]:]


def insert(stream, body):
    query = f"INSERT INTO sphericon.{stream} SETTINGS date_time_input_format='best_effort', " \
            "input_format_json_read_objects_as_strings=1 FORMAT JSONEachRow"
    url = f"{CLICKHOUSE}/?query={urllib.parse.quote(query)}"
    urllib.request.urlopen(urllib.request.Request(url, data=body)).read()


def sync(stream):
    path = f"cursor-{stream}.json"
    # Each checkpoint is [updated_at of the last row of a page, the cursor after that page].
    checkpoints = json.load(open(path)) if os.path.exists(path) else []
    cursor, checkpoints = start_cursor(checkpoints)
    while True:
        url = f"{API}/export/{stream}?limit=10000&cursor={urllib.parse.quote(cursor)}"
        request = urllib.request.Request(url, headers={"Authorization": f"Bearer {TOKEN}"})
        with urllib.request.urlopen(request) as response:
            body = response.read()
            cursor = response.headers["X-Next-Cursor"]
        if not body:  # an empty page hands back the cursor it was given
            break
        insert(stream, body)
        checkpoints.append([json.loads(body.splitlines()[-1])["updated_at"], cursor])
        json.dump(checkpoints, open(path, "w"))


if __name__ == "__main__":
    sync(sys.argv[1])
```

On the first run there is no checkpoint, so it reads from the beginning (a back-fill, or a
re-seed after you emptied the tables). On later runs it starts from the newest checkpoint that is
at least 10 minutes older than the newest row it has read, so a row that committed late is read
too, and the rows it reads twice collapse in `ReplacingMergeTree`.

### Applying the Erasures

Run this after the five streams have loaded. It deletes by every key an Erasure row carries, and
it is safe to run again. Note the `subject_id != ''` guard: anonymous Events have an empty
`subject_id`, and without the guard an address-only Erasure would delete all of them.

```sql
DELETE FROM sphericon.events WHERE
    contact_id IN (SELECT contact_id FROM sphericon.erasures FINAL WHERE contact_id IS NOT NULL)
    OR (subject_id != '' AND subject_id IN (SELECT subject_id FROM sphericon.erasures FINAL))
    OR visitor_id IN (SELECT arrayJoin(visitor_ids) FROM sphericon.erasures FINAL)
    OR lower(hex(SHA256(lower(email)))) IN (SELECT email_sha256 FROM sphericon.erasures FINAL WHERE email_sha256 IS NOT NULL)
    OR lower(hex(SHA256(phone))) IN (SELECT phone_sha256 FROM sphericon.erasures FINAL WHERE phone_sha256 IS NOT NULL);

DELETE FROM sphericon.contacts WHERE
    id IN (SELECT contact_id FROM sphericon.erasures FINAL WHERE contact_id IS NOT NULL)
    OR lower(hex(SHA256(lower(email)))) IN (SELECT email_sha256 FROM sphericon.erasures FINAL WHERE email_sha256 IS NOT NULL)
    OR lower(hex(SHA256(phone))) IN (SELECT phone_sha256 FROM sphericon.erasures FINAL WHERE phone_sha256 IS NOT NULL);

DELETE FROM sphericon.messages WHERE
    contact_id IN (SELECT contact_id FROM sphericon.erasures FINAL WHERE contact_id IS NOT NULL)
    OR lower(hex(SHA256(lower(destination)))) IN (SELECT email_sha256 FROM sphericon.erasures FINAL WHERE email_sha256 IS NOT NULL);
```
