# Introduction

sphericon helps you know the people you talk to and reach them with the right email at the right
moment. It collects who your contacts are and what they do, lets you describe groups of them,
and sends to those groups: one-off broadcasts, automated sequences, and transactional mail.

![The sphericon workspace overview](/screenshot.png)

## The model in five ideas

**Workspace.** The tenant boundary. Every contact, event, segment and message belongs to exactly
one workspace, and each workspace has its own API keys, sending domains and integrations.

**Contact.** One record per person, whether fully identified or still anonymous. A contact can
carry an email, a phone and your own user id (`subject_id`); any of them may be missing. Beyond
those it has typed [custom fields](/guide/tracking#custom-fields) and tags.

**Event.** An immutable record that something happened: `signed_up`, `viewed_pricing`,
`email.opened`. Events attach to the contact, so behavior is something you can segment on.
sphericon's own delivery facts (`email.sent`, `email.opened`, `email.clicked`) are events too, so
engagement is segmentable the same way as anything you track yourself.

**Segment.** A saved rule that says which contacts match: attributes, custom fields, tags, and
what they did or did not do. Segments are evaluated live. Membership is never copied, so it moves
as your data does. There are no static lists.

**Send-eligibility.** Whether you may write to someone is derived, never a flag on the contact.
Suppressions, unsubscribes and, if you require it, confirmed opt-in are checked on every send
path. See [Deliverability and consent](/guide/deliverability).

## What you can send

| Surface           | What it is                                                        |
| ----------------- | ----------------------------------------------------------------- |
| **Broadcast**     | One email to a segment, sent now or scheduled, with a report.     |
| **Automation**    | A sequence of steps started by an event, run once per contact.    |
| **Transactional** | A single email your app triggers over the API, such as a receipt. |

All three share templates, the same sending domain checks and the same consent rules. See
[Sending email](/guide/sending).

## Where to go next

- [Quickstart](/guide/quickstart): from zero to a first broadcast.
- [Tracking visitors and events](/guide/tracking): connect your site or product.
- [Self-hosting](/self-hosting): configuration, migrations and health checks.
- [Sphericon Cloud](/guide/cloud): the hosted edition, if you would rather not run it yourself.
