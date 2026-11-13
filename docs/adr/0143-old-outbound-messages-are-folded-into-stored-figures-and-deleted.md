# Old Outbound messages are folded into stored figures and deleted

Every send left an Outbound message, a Broadcast recipient and, for a Telegram message or an SMS,
its tracked links, and nothing ever removed them: the largest tables of an Instance grew without
end, each row holding a Destination long after the send mattered. Event retention (ADR 0019) left
them alone on purpose, since the Broadcast and Automation reports read the messages' stamps (ADR
0053).

Outbound messages now end after a per-Instance window (`SPHERICON_MESSAGES_RETENTION_DAYS`, default
400, the Event window's default; `0` disables). A daily job keeps every report's figures and deletes
the rows:

- **A Broadcast is frozen whole.** Once it has been sent or has failed (both final, ADR 0046) for
  longer than the window, its figures are written once to a `broadcast_figures` row, and never
  recomputed. Its recipients are then deleted in batches, each with its messages in one transaction,
  so a recipient never outlives its message and never reads as pending again. A last sweep deletes
  messages whose recipient was already gone, and the row records when the Broadcast was pruned. A
  Broadcast with a figures row reports that row and nothing live: the figures stay right while its
  rows are still being deleted.
- **An Automation's old messages are added into its figures.** An Automation lives for years, so it
  is never frozen whole: each batch of its messages past the window that have left pending is
  counted into `automation_figures`, one row per step position, and deleted in the same transaction.
  The report adds those counts to its live ones, so it reads exactly what it read before.
- **A Transactional message is deleted.** Nothing is reported from it past the window.

The daily tick queues one prune per Workspace with something past the window, and each prune works
in batches that persist at once, a bounded number per run, queueing its next run while more is left
(ADR 0144).

Tracked links go with their message. An expired short link (`/c/`) sends its visitor to a neutral
page of the app instead of a 404. An email's click link still redirects, since its destination is
in the signed token, and an open or a click token now names its Workspace too, so a Tracking host
can refuse another Workspace's link without reading the message row (ADR 0038).

The window is at least 180 days: the analytics overview reads up to 90 Report days and the period
before them from the messages, and a shorter window would empty it without saying so. Usage periods
and telemetry read far less.

Consequences:

- A late delivery report, an open or a click on a pruned message records nothing. An unsubscribe
  still opts the Contact out, but is no longer counted on its frozen Broadcast.
- A Transactional `Idempotency-Key` sent again after the window sends again.
- An evidentiary Event outlives its message (ADR 0019), so the activity feed shows it without the
  message's subject.
- What a Workspace sent before the window is gone from a Contact's data export, from the Messages
  export stream (unannounced, as ADR 0045 says of every deletion by retention), from the setup
  checklist's "sent an email" and from an Erasure's search for a Destination with no Contact.

## Considered options

- **Keep the messages and only expire Events** (rejected): the tables and the Destinations in them
  grow forever.
- **Delete messages by age alone** (rejected): every Broadcast report would lose its old figures,
  and a recipient whose message is gone would read as pending again.
- **Running counters on the Broadcast, updated on every send and stamp** (rejected): a figure kept
  beside the rows it counts can drift from them. A count taken once, after the rows can no longer
  change, cannot.
