---
status: accepted
---

# Contacts come in by a mapped CSV import and leave by a streamed CSV export

A Workspace moving from another platform brings a CSV of its Contacts, and one leaving takes one
away. Today the API's batch upsert, the Contact intake, takes up to 1000 rows a call: each row in a
transaction of its own, SMS consent evidence in its columns (ADR 0077), no `contact.field_changed`,
one `contact.import` Audit entry per batch. A Contact it creates records `contact.created`, which
starts Automations (ADR 0016). The app exports one Contact (ADR 0021) and, to a warehouse, Export
streams (ADR 0045), both streamed with no file kept; it has no file storage, and images are linked
by URL.

## Decision

### Import

- **A CSV import in the app is a background job over the Contact intake, run by a Member holding a
  new Permission, `contacts:import`** (owner and admin by default): an import writes thousands of
  Contacts and records opt-outs that cannot be undone. The file is uploaded and kept in the database
  with its job, then mapped, then previewed: the first rows as they would be written, the refusals
  among them (ADR 0117's address check runs on that sample only), and the number of opt-outs and
  Suppressions the whole file would record. Running it sends the rows through the Contact intake in
  order, batch after batch, with progress, resuming from its last batch after a restart; the whole
  job is one `contact.import` Audit entry. It is UTF-8 (a BOM allowed), its delimiter (comma,
  semicolon or tab) is detected, it has a header row, and its size is bounded by an Instance setting.
- **What it keeps of personal data is short-lived.** The file is deleted when the job ends; the
  refused rows' report is kept with the job until it is downloaded or seven days pass, whichever comes
  first; an upload abandoned before it runs is deleted after a day. An Erasure does not search them;
  the expiry bounds them, and a row still to run may create the Contact again, as any Import may (ADR
  0021).
- **Each column is mapped**: to an alias key, a core attribute, an existing Custom field, a new Custom
  field whose type the Member chooses (a Member's review, so creating one is allowed, unlike the
  declared-by-use of ingest; it needs `custom_fields:manage`), a column of Tags, a refusal column
  (below), or nothing. The Member may also give one Tag to every row ("from Mailchimp, March").
- **A bad value is dropped, not the row**: a value its field's type cannot read is not stored and is
  reported, and the rest of the row is written, as Identify does (ADR 0072). A row with no alias key
  is refused. A key repeated in the file follows row order: attributes from the last row, keys only
  filled. A row whose keys name two Contacts raises a Duplicate candidate whose door is the import
  (ADR 0128).

### Consent

- **Whether an imported Contact can be mailed is the Workspace's existing rule.** On email, with
  require-confirmed-opt-in off, the default, an imported address is mailable as any single opt-in
  Contact is: the Workspace asserts it may write to the people it imports. With the policy on, it is
  mailable only with a Confirmation, and an import records one only through the consent columns the
  batch upsert already takes, with the provenance `imported` and its wording, source and brand. On
  SMS a Confirmation is always required, as before. No other evidence of consent is created by an
  import.
- **Refusals from the old platform come in, mapped with care.** A refusal column names its Channel,
  so its Destination is the row's email or phone, and the Member lists the cell values that mean it
  ("unsubscribed", "true"); any other value records nothing. An unsubscribed value records an
  Unsubscribe, of `broadcasts` or of `everything` as the mapping chooses, with the provenance
  `imported`; a bounced or complained value records a Suppression of the manual kind, which needs
  `suppressions:manage`. Anyone may narrow consent (ADR 0016, ADR 0121), and without this a move from
  another platform would mail the people who had left it.
- **An imported Unsubscribe cannot be undone**: only the person lifts one (ADR 0121). The preview
  says so beside the count it would record; an imported Suppression can be lifted as any Suppression
  can.

### What an import starts

- **A CSV import starts nothing by default.** Its `contact.created`, `contact.tagged` and other
  Events carry a cause marking the import, and, as a Backfill's, they enroll no one, fire no Rule,
  record no `contact.score_reached`, end no Enrollment as an exit or goal Event, satisfy no Wait for
  event and reach no Webhook endpoint: importing fifty thousand old Contacts must neither send them a
  welcome series nor flood the customer's endpoints.
- **A box "treat them as new sign-ups", off by default, lets the Member choose otherwise.** No
  Permission gates Publish today, so the box is gated by `contacts:import` and by the choice itself,
  which is kept on the job, in every install, and on its Audit entry where the Audit log exists. The
  API's batch upsert is unchanged: it is the customer's own code, deciding for itself.

### Export

- **A CSV export of Contacts is a streamed download in the app**, as one Contact's is (ADR 0021): a
  Segment's Contacts or all of them, with the columns the Member chooses, read through a cursor and
  written as it is read, with no job and no file kept. It needs `contacts:export` and is audited (ADR
  0022). It is not on the external API or MCP: a Connection's `contacts:export` reaches the one-Contact
  export only, and bulk reads for a warehouse are Export streams (ADR 0045).
- **Its columns include each Channel's state**: whether the Destination may receive, is opted out
  (and of which scopes) or is suppressed, beside the alias keys, core attributes, Custom fields and
  Tags. A Workspace that leaves takes its refusals with it, so its next platform does not mail the
  people who left; that is what keeps the data the Workspace's own.
- **A cell is written so a spreadsheet cannot run it.** A value beginning with `=`, `+`, `-`, `@`, a
  tab or a carriage return is written with a leading apostrophe; most of these values are untrusted
  (ADR 0132). The same rule holds for the refused rows' report and every CSV the app writes.

## Consequences

- The Permission catalogue gains `contacts:import`; the app gains the import flow, its job, its stored
  file and report with their expiries, its mapping, preview and refusal columns; the Contact intake
  gains the import's cause marker.
- The app gains the streamed CSV export of a Segment or all Contacts with Channel states, and one
  rule for writing CSV cells.

## Considered options

- **Parsing the file in the browser and posting batches.** A large file ties the import to an open
  tab; a job finishes on its own.
- **Refusing a row for one bad value.** One malformed birthday would lose a whole Contact.
- **Imported Contacts starting Automations, as the API's do.** The common import is a list of people
  who signed up long ago.
- **Leaving opt-outs out of the export.** The next platform would mail everyone again.
- **An export job with a file to fetch later.** The app keeps no files; a stream is how its other
  exports work.
- **Mapping any non-empty value of a status column to an opt-out.** A column that mixes states would
  opt a whole list out for good.
