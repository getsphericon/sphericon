---
status: accepted
---

# Marketing email is always HTML; its text part is derived from the final HTML

A Broadcast or Automation email is always Message content, HTML authored in the editor
([[0040-email-content-is-html-authored-in-an-embedded-editor]]). A "plain" letter is that same
content written without design, not a second format. The `text/plain` part of a send is derived
from the **final per-recipient HTML**, after tracking and the unsubscribe footer are layered on
([[0015-outbound-send-single-chokepoint]]), so it carries the tracked link URLs and the visible
unsubscribe link and postal address ([[0012-bulk-sender-compliance-one-click-unsubscribe]]). It has
no editor and no stored copy.

Deriving it from the rendered HTML before tracking, as the send path first did, left text clients
with untracked links and no unsubscribe link in the body. The competitors' "text templates" suggest
a text-only send; we keep one concept and never send a true `text/plain`-only marketing email.

## Considered options

- **A text-only send mode** (`text/plain` with no HTML part). Rejected: it needs a second authoring
  model or a send flag, loses open tracking and clean links, and the derived text part already gives
  mailbox providers the plain alternative. It stays open later as a send flag, not an entity.
- **A hand-edited text body beside the HTML.** Rejected: two bodies per message drift apart.
