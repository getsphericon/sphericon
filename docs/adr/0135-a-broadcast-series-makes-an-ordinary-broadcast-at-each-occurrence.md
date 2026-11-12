---
status: accepted
---

# A Broadcast series makes an ordinary Broadcast at each occurrence

> **Amended by [[0140-a-write-that-arms-live-use-refuses-a-row-already-discarded]]:**
> a series making an issue reads its rows under the lock and skips the occurrence when one is
> discarded.

A Broadcast is a one-off send: authored, scheduled once and sent once. A weekly digest or a
newsletter built from a blog's feed is a send that repeats on a schedule, and today it is a new
Broadcast authored by hand each week. An Automation is the wrong home for it: it would enroll the
whole audience at every occurrence (ADR 0126).

A Broadcast copies its Segment's Condition when it is scheduled and freezes its Recipients when it
sends. All Broadcasts share the Sending source `broadcasts`; each Automation is its own. An
Automation, a Rule and an On-site form are Versioned definitions, changed only by Publish, and
Publish is send-class (ADR 0065).

## Decision

### A series is a Versioned definition

- **A Broadcast series holds a Channel, an audience (a Segment, or all Contacts), Message content, an
  Integration, a schedule and, optionally, a feed.** It is a Versioned definition: an edit writes a
  draft, Publish makes it live and is send-class (`broadcasts:send`, and `mcp:send` over MCP, ADR
  0016), Revert drops the draft, Deactivate stops its occurrences and Archive retires it once nothing
  of it is sending. An issue in flight when it is deactivated finishes, as a Broadcast does.
- **Publish takes a copy of the Segment's Condition**, as Schedule does for a Broadcast. A later edit
  of the Segment changes no series until it is published again, so a `segments:write` edit, which an
  agent holds by default, cannot change who an unattended send reaches (ADR 0132). The editor says
  when the Segment has changed since the copy.
- **At each occurrence it makes an ordinary Broadcast, an issue, and sends it.** The issue copies the
  version's content and Condition, renders its feed items once and keeps them, so a Recipient deferred
  by its Sending window (ADR 0134) gets the same issue. From then on it is a Broadcast like any other:
  its Recipients, Send eligibility, report and figures. It cannot be edited, since the series is what
  is edited.
- **An upcoming occurrence can be cancelled**: the date is recorded on the series, outside its
  versions, and is send-class as an unschedule is (ADR 0016). An issue already made is unscheduled as
  a Broadcast is.
- **The schedule is weekly or monthly**: chosen weekdays at a time, or a day of the month at a time,
  where a day the month lacks is its last day. It is read in the Workspace time zone, with the
  calendar rules of ADR 0126 for a time skipped or repeated by daylight saving. Occurrences run only
  forward from the moment the series became active. One missed by an outage is sent if it is less
  than 24 hours late and the next one is not yet due; otherwise it is dropped and counted.
- **An occurrence that cannot send now is skipped and counted, never queued**: while the previous
  issue is still sending (two issues back to back are worse than one missed), and while a Hold stops
  the Workspace or the series' Integration, so an issue made under a Hold does not go out stale when it
  lifts. An issue whose rendered content breaks its Channel's length limit (SMS, Telegram) is skipped
  too, since every message of it would fail.

### A series is its own Sending source, inside `broadcasts`

- **Each series is the Sending source `series:<id>`, narrower than `broadcasts`.** A person opted out
  of `broadcasts` receives no series, and one opted out of a series still receives the Workspace's
  other Broadcasts. Its emails' one-click and footer unsubscribe from that series, with the
  source-scoped token (ADR 0012); the Preference page lists each series a person receives, by its
  name, beside "all Broadcasts" (ADR 0121). The reserved `everything` covers both. So a new series
  starts with every `broadcasts` opt-out already applied, and a series cannot be used to reach people
  who left the Workspace's Broadcasts.

### Content from a feed

- **A series may name a feed**: RSS, Atom or JSON Feed, at an https URL. At each occurrence the feed
  is read through the SSRF-hardened client Webhooks use, with a timeout and a size limit.
- **An item is new when the series has not seen it**, by its id (the item's `guid` or `id`, else its
  link). Publish marks every item the feed holds then as seen, so the first issue carries what
  appears after it, not years of archive. An issue takes the newest new items up to the count the
  series sets, and every new item, the ones over that count included, is marked seen when the issue
  is made. A cancelled occurrence or a skipped one marks nothing, so its items go into the next issue.
- **No new item skips the occurrence**, and so does a feed that cannot be read or parsed, which is
  also an item of Needs attention: an old issue is never sent again for want of a new one.
- **What a feed holds is data, never template.** Its text is inserted as values the template prints,
  never parsed for merge tags, escaped, and its HTML is reduced to a safe subset (ADR 0132).
- **Its link and image hosts are approved at Publish.** Publish reads the feed and lists the hosts its
  items link to and load images from, beside the version's own (ADR 0132); the person publishing
  approves them, and they become the series' approved hosts. A later issue keeps a link or image only
  on an approved host; any other becomes plain text, and the series page says which hosts were
  dropped so the next Publish can approve them. An issue sends with no person looking at it, so a feed
  that was taken over must not put a stranger's link into it.
- **A series without a feed sends the same content each time.** That is allowed, as for a weekly
  reminder, and Publish warns that every issue will be the same.

### Reports

- **Each issue has the report of a Broadcast.** The series lists its issues by date with their
  figures, and the occurrences it skipped with the reason: an issue still sending, no new item, a
  feed that could not be read, an outage.

## Consequences

- A Broadcast series is a new Versioned definition with its schedule, its Condition copy, its feed,
  its approved hosts, the items it has seen and its cancelled dates; a Broadcast may belong to one and
  then sends under its Sending source.
- Send eligibility reads `series:<id>` as inside `broadcasts`.
- ADR 0065's send-class list gains the series' Publish; the Removal check reads a series' Segment and
  Integration as Definition use, as an Automation's.
- A scheduler makes the issues; the feed reader is a new outbound fetch through the SSRF-hardened
  client.
- The Preference page lists series beside Automations.

## Considered options

- **An Automation with a schedule.** It enrolls every Contact at each occurrence, with the state of
  Enrollments for a send that has none.
- **Making a draft Broadcast at each occurrence for a person to finish.** It is a reminder, not a
  series; a Workspace can keep an editorial calendar elsewhere.
- **Issues sharing the `broadcasts` source.** A reader could leave the digest only by leaving every
  Broadcast.
- **A series as a source beside `broadcasts`, not inside it.** A new series would mail everyone who
  had left the Workspace's Broadcasts.
- **Reading the Segment live at each occurrence.** A Segment edit, open to an agent by default, would
  change an unattended send's audience with no Publish.
- **Sending the last items again when the feed has none new.** Readers get the same issue twice.
- **Keeping every link a feed holds.** An automatic send has no reviewer; a taken-over feed would
  phish the whole list.
- **A schedule in each recipient's time zone.** A Broadcast has one send time today; local delivery
  time is a question for every Broadcast, not one for series.
