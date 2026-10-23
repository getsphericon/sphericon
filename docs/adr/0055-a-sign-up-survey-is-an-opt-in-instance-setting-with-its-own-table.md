---
status: accepted
---

# A Sign-up survey is an opt-in Instance setting with its own table

The project cannot see who signs up: their role, the size of their company, what they want to do
with the product, or how they found it. A self-hosted Instance reports only the anonymous Telemetry
report (ADR 0032), which carries none of that. The Sign-up survey asks four questions once, after
a new User's first sign-in, and keeps the answers on the Instance for a Staff member to read.

## Decisions

- **Off unless switched on.** `SPHERICON_SIGNUP_SURVEY` turns it on only with an explicit `true`, as
  `SPHERICON_REQUIRE_EMAIL_VERIFICATION` (ADR 0030) and `SPHERICON_SIGNUP_OPEN` (ADR 0042) do, so a
  typo never starts collecting. It also needs the `staff` license feature, because only the Staff
  console reads the answers. Without the feature it stays off and the server logs one warning at
  start; it is not a boot error. A Self-hoster is never asked.
- **A marker says who is asked, not an inference.** Every new User already has a first Workspace and
  an Owner Membership (ADR 0030, 0043, 0048), so "has no Membership" would never be true. The two
  public sign-up paths (password, Sign-in provider) set a nullable "survey due" marker on the User
  when the survey is on. Invitation accept, `sphericon user create-owner`, join-domain admission
  (ADR 0051) and Users that existed before the switch never set it. The column is additive and
  nullable (ADR 0033).
- **Due is computed on the server.** The current-user resource carries a boolean, true only while
  the User has the marker and the Instance still asks the survey, so a switch turned off or a
  lapsed license stops asking a User marked earlier too; the submit is refused (409) then as well.
  The SPA reads the boolean and never sees the Instance switch.
- **Answers live in their own table.** `SignupSurveyResponse`, one row per User, with a foreign key
  to the User that cascades on delete (as `OAuthCode` does to its Workspace), typed columns, enum
  fields with a CHECK, and a length cap on each "Other" text. It belongs to the User, not a
  Workspace, so it has no scoped wrapper. User deletion (ADR 0050) needs no extra step: the cascade
  removes the row.
- **Fixed questions, enums in the contract.** Role, company size and main goal are required; where
  the User heard of the product is optional. Each is single choice and ends with "Other" and a short
  text. The options are TypeSpec enums, so generated types and zod stay the one source. English only.
- **One submit, nothing partial.** One `/site` operation, sent after the last screen, creates the row
  and clears the marker in one transaction; a second submit is an error. A User who closes the tab
  starts again at the next sign-in.
- **A redirect in the SPA, not a gate on the server.** The route guard sends a User whose survey is
  due to the survey page, with no Skip. The server does not block other `/site` operations: this is
  a UX gate, not a security boundary.
- **Read by Staff only.** Two read operations on `/staff`: counts per question, with "Other" texts
  and their counts, and one User's answers. No export, no webhook, no service endpoint.
- **Nothing leaves the Instance.** The answers are never part of the Telemetry report (ADR 0032),
  never sent to an analytics service, and do not change the Setup checklist, mail or Segments.

## Considered Options

Research read the source code of Airbyte, PostHog, Chatwoot, Mautic and Dub; RudderStack and
Listmonk were not verified.

- **A User field or metadata plus an analytics event** (Airbyte and PostHog, both on their hosted
  cloud; Airbyte's is behind an experiment flag that is off by default and has a Skip button): no
  table to add, but the answers are not queryable as typed columns, a free-form field has no CHECK,
  and the event sends them to an analytics service; rejected. Answers stay on the Instance.
- **No survey** (Mautic asks nothing; Chatwoot asks a self-hoster only for name, company and email):
  the status quo, where the project stays blind to who signs up; rejected.
- **A separate table** (chosen): fits plain tables, typed enum columns with a CHECK and a cascade on
  User deletion, and keeps the data on the Instance.
- **Infer "new" from having no Membership**: never fires, since every new User has a first
  Workspace and an Owner Membership; rejected for the explicit marker.
- **Block other `/site` operations until answered**: a server-side rule for what is a UX concern;
  rejected for the SPA redirect.
- **Asking existing Users when the survey is turned on, or after the questions change**: a
  configuration change would interrupt people at work; rejected.
