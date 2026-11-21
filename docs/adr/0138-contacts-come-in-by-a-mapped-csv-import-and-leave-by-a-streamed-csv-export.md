---
status: accepted
build: [1839, 1840, 1841, 1842, 1843, 1844, 1845, 1846, 1847, 1848, 1849]
---

# Contacts come in by a mapped CSV import and leave by a streamed CSV export

> **Amended below:** an import's Events carry one history mark on their envelope; its refusals act
> as any refusal does; a blank cell changes nothing and existing Contacts may be left untouched;
> each text is read in a format the Member chooses; the batch upsert gains Tags, email consent and
> the time a consent fact states, and refuses a Custom field value of the wrong type; an imported
> Unsubscribe is `via: imported`, an imported Suppression `manual`; the export's headers are the
> import's, its Channel states one read shared with the consent history (ADR 0141).

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

## Amendment: what the spec settles before it is built

Read against the code before the spec: the batch upsert takes no Tags and no email consent, fails a
whole row for one bad value yet stores a Custom field value of the wrong type as given, stamps
every consent fact now, and writes one Audit entry per call; an Unsubscribe has a `via`, not a
provenance; only two Events can carry the Backfill mark; nothing reads a Destination's whole state on
each Channel, iterates a Segment by cursor, stores an upload or keeps a job's checkpoint; the only
CSV writer is the Audit log's.

### Import

- **The upload is the file itself**, a `text/csv` body under its own Instance cap (50 MiB by
  default, at most 200 columns), kept in a table of its own; a file that is not UTF-8 is refused at
  upload. The refused rows are rows of a table too, and the report is written from them when it is
  downloaded.
- **One import runs per Workspace at a time**; a second is refused while it runs. The job keeps its
  place after each batch of 1000 rows and, after a restart, runs at most that batch again, which the
  intake allows: keys are only filled, Tags only added, refusals recorded once. A Member holding
  `contacts:import` may cancel it: it stops after its batch, what is written stays, the file goes and
  the report is kept.
- **The job acts as the Member who ran it**, named on its Events and its Audit entry; `contacts:import`
  is checked again before each batch, and a Member who lost it, or left, stops the job as a cancel
  does. `custom_fields:manage` and `suppressions:manage` are checked when it is run.
- **The preview is answered at once**: the first 20 rows as they would be written, ADR 0117's check
  on those, and over the whole file the rows, the rows with no key, the opt-outs and Suppressions it
  would record, the new Custom fields and the distinct Tags. Whether a row is new to the Workspace is
  not counted: it needs a lookup per row.
- **A blank cell changes nothing**: no import clears a value, which is an edit. Existing Contacts are
  updated by default (keys filled, values given written); a box "only add new Contacts" leaves an
  existing one as it is but for its refusals, which narrow and are always recorded.
- **Each text is read in the format the Member chooses** when mapping, never guessed silently: a
  date column's format (ISO, day-month or month-day with its separator, RFC 3339 for a time), proposed
  from the sample and required where it is ambiguous; the file's decimal separator; the booleans
  `true`/`false`, `yes`/`no`, `1`/`0`, `t`/`f`, `y`/`n`; and a default country for a phone whose row
  names none. One reader per field type lives in the Contacts module, and the mapping is a
  discriminated union of the contract. A text it cannot read is dropped and reported, so a row reaches
  the intake clean.
- **A column of Tags** splits its cells on commas; the Tag given to every row is proposed as
  "Imported" with the date. An import adds Tags and never removes one.
- **A row whose keys name two Contacts** is written to the one the keys' rank picks and reported
  with a warning until ADR 0128 is built; its spec makes it a Duplicate candidate whose door is the
  import.
- **A Contact the import creates has the source `import`**, so years of Contacts brought from another
  platform do not read as new people.

### The batch upsert

- **It gains Tags** (added by name, created on first use as every apply is), **email consent**
  (`emailConsent`, recording an `imported` email Confirmation, as ADR 0013 has it) and **the time a
  consent fact states**, ordered by ADR 0154. A time missing, unreadable or in the future is dropped
  and the fact stamped now. Recording consent lifts no refusal (ADR 0121); one recorded through a
  Connection without `mcp:send` carries ADR 0132's mark, as the Contact it is written with does.
- **It refuses a Custom field value of the wrong type**, naming the field, by the one type check of
  the Contacts module (ADR 0125), as it refuses any other bad value of an item: code can correct
  its item, a Member cannot correct fifty thousand rows, which is why the CSV import drops instead.
- **A CSV import's whole job is one `contact.import` Audit entry**, with its counts, its mapping and
  the choice of the box; a call of the batch upsert stays one entry.

### Consent and refusals

- **A consent column** lists the cell values that mean consent, with the wording, source and brand
  given once for the import or mapped from columns; **a refusal column and a consent column may each
  be paired with a time column**, read as the batch upsert's time is.
- **An imported Unsubscribe is `via: imported`**, the Unsubscribe's display of how it was made; on
  email its Event carries it. "Provenance `imported`" above means this.
- **An imported Suppression is of the `manual` reason** on email, SMS or Telegram; the import is named
  by its Event's cause. The export writes every reason a Suppression has.
- **An imported refusal acts as any refusal does**: an Unsubscribe of `everything` withdraws the
  Confirmation, an Enrollment's next send is skipped, and only an opt-out of that Automation ends its
  Enrollment (ADR 0081). What the import's cause holds back is its Events as triggers.
- **Engagement tracking consent** (ADR 0120) is a consent column of its own once the batch upsert
  takes it, `granted` and `denied` by the values the Member lists.

### What an import starts

- **One history mark on the envelope every Event carries**, naming the import's job, replaces the
  mark only some Events could carry; the consumers read it through one predicate, and a static check
  refuses reading the stored flag elsewhere. With "treat them as new sign-ups" chosen, the Events
  still name the job but are not history. ADR 0132's origin mark is stamped on the same envelope.

### Export

- **The download streams to disk**: a request with the Segment and the columns returns a one-time
  link, bound to the session and good for a minute, which the browser follows; the export is audited
  as `contact.export` when its stream starts, and the one-Contact export is audited too (ADR 0022).
  The refused rows' report downloads the same way. A file is UTF-8 with a BOM and comma-delimited.
- **A Segment's Contacts are read by a keyset cursor**, and **each Channel's state by one set-based
  read** beside Send eligibility: may receive, needs confirmation, opted out (its scopes and time) or
  suppressed (its reason), with the Confirmation's provenance and time. The consent history (ADR 0141) reads the same, so the two never disagree.
- **The export's headers are the import's mapping targets**: the contract's names for the keys and
  core attributes, each Custom field, Tags, and per Channel (email, SMS, Telegram) its state, scopes,
  Suppression reason and Confirmation; web push its Binding. An import maps a header it knows by
  itself, so a Workspace's export brings its Contacts, Tags, fields and refusals into another one, its
  Confirmations as `imported`. An opt-out of a single source, which means nothing in another
  Workspace, comes back as nothing; `everything` and `broadcasts` come back as they were.
- **One CSV writer**, in core, neutralises a cell as above, and a static check refuses writing CSV
  anywhere else; the Audit log's export uses it.
