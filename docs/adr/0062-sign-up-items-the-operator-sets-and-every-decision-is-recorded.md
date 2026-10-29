---
status: accepted
---

# Sign-up items: the operator sets what a person accepts, and every decision is recorded

Sign-up asked for nothing and recorded nothing: `/register` and a Sign-in provider's first login
([[0048-sign-in-providers-prove-an-email-and-replace-only-the-password]]) created the User and its
first Workspace at once. What a person must accept or may agree to depends on the market an Instance
serves (terms and a privacy notice in one, a separate consent to the processing of personal data or to
advertising mail in another), and an open-core Instance can run in any of them, so the core cannot
know the rules. It gives the operator a mechanism instead: **Sign-up items**, each with a versioned
**Legal document**, and a **Consent record** of every decision. The research behind this is in
`docs/research/signup-consent-prior-art.md`.

## Decisions

- **The market is the Instance's.** One Instance serves one market, in one language. The core ships
  no legal text, names no jurisdiction and has no Sign-up item until a Staff member adds one in the
  Staff console ([[0061-every-instance-has-a-staff-console-and-sign-up-is-always-open]]); with
  none, sign-up behaves as before.
- **A Sign-up item** has a key, a title and a kind: a **required acceptance** (a checkbox without
  which no User is created), an **optional consent** (an unticked checkbox that sign-up never
  depends on) or a **notice** (a link, nothing to tick). An item may be marked as consent to the
  operator's mailings.
- **A Legal document** is the item's text, Markdown kept in the Instance's database and served on a
  fixed public page of the app. A draft is edited freely; publishing makes an immutable version that
  takes effect at once. A new text is a new version, never an edit of a published one.
- **The app asks, the server checks.** The sign-up form, the Invitation form and the screen after a
  Sign-in provider show the items, and the server refuses a sign-up that lacks a required
  acceptance; the browser's checkbox is never the only check.
- **A Sign-in provider creates no User before the items are answered.** When the decision of ADR 0048
  would create a User and the Instance has Sign-up items, the callback keeps a short-lived pending
  sign-up on the server and the app shows the items; the User, its Linked identity, its first
  Workspace and the Consent records are created together on submit, and declining creates nothing. A
  known Linked identity signs in as before.
- **A new version of a required item stops every User who has not accepted it**, on their next
  request, at an accept-or-sign-out screen; their API tokens are refused until then. The same rule
  covers an item added to an Instance that already has Users, and an Owner made by the CLI. A new
  version of an optional item or a notice stops no one.
- **Optional consents are the User's to change** in their account settings. A User who signed up
  before an optional item existed is not asked.
- **A Consent record** is one decision of a User on one version: accepted, declined or withdrawn,
  with the time, where it was made (sign-up form, provider screen, Invitation, re-acceptance, account
  settings), the IP address and the user agent. Records are only ever added; the latest per item is
  the current state. They belong to the User, not to a Workspace, so a User who joins another
  Workspace answers nothing again, and they are deleted with the User
  ([[0050-a-user-deletes-themself-at-once-and-the-audit-log-keeps-their-name]]).
- **The Staff surface feeds the operator's mailing list**: a cursor feed of Users (created, renamed,
  changed address, deleted) with their current consent to the operator's mailings, under a scope of its
  own. The Consent records themselves stay in the Instance.

## Considered options

- **Operator-configured URLs instead of texts**: the operator wanted the documents in the app, and a
  record could not show what a URL served on the day; rejected.
- **A fixed set (terms, privacy notice, marketing)**: fits one market only; rejected for items.
- **A notice under the button ("by signing up you agree")**: the weakest form of acceptance and
  absent from the record; rejected.
- **Create the User, then gate it** for a Sign-in provider: leaves Users who never accepted, and the
  gate becomes the only check; rejected for the pending sign-up, as Discourse and Ory Kratos do.
- **A flag or a version column on the User**: loses which version was accepted and when it changed;
  rejected for the records.
- **A market per User**: one Instance serving several markets needs countries, sets per country and
  rules for a move; rejected while one Instance serves one market.

## Consequences

- An operator states in their own privacy notice that IP address and user agent are kept with each
  decision.
- Keeping records after a User is deleted needs a purpose and a limited period; it is not done.
