# The Sphericon app

Everything in the app happens inside a [workspace](/guide/introduction#the-model-in-five-ideas).
The sidebar lists its sections; **Settings** has a sidebar of its own. The workspace switcher in
the header moves you between workspaces, and **Docs** in the header opens this documentation.

## Sections

**Overview.** The landing page of a workspace: how many contacts it has and shortcuts to the
activity feed, contacts and settings. While the workspace is new it also shows a setup checklist
(add a contact, connect an integration, verify a sending domain, send a first email).

**Contacts.** Every person the workspace knows, identified or anonymous. Filter by status, open a
contact to see its fields, tags and event history, edit custom fields, export its data or erase it
(owners and admins only, see [Answering a GDPR request](/guide/gdpr)). See
[Tracking visitors and events](/guide/tracking).

**Segments.** Saved rules that select contacts by attributes, tags and behavior. **Preview
audience** shows how many contacts match right now; membership is always evaluated live. See
[Segments](/guide/segments).

**Broadcasts.** One-off emails to a segment (or to all active contacts), sent now or scheduled,
with a test send and a report of recipients, opens and clicks. Unsubscribed and suppressed
contacts are always excluded. See [Sending email](/guide/sending).

**Templates.** Reusable email bodies written in MJML with merge tags such as
`{{ first_name }}`, shared by broadcasts and automations.

**Automations.** Sequences started by an event and run once per contact: send an email, apply a
or remove a tag, wait, post JSON to a URL. Activate and deactivate them here and inspect failed enrollments.

**Transactional emails.** The log of single emails your app triggered over the
[API](/guide/api): recipient, status, send time and the error when delivery failed.

**Activity.** The live feed of events arriving in the workspace, from the tracker, the API and
Sphericon's own delivery events. Filter by action and open an event to see its properties.

**Settings.** Configuration of the workspace, described below.

## Settings

**General.** The workspace name and the postal address shown in the footer of every marketing
email (CAN-SPAM requires one).

**Team and security.** Members and their roles, pending invitations, the requirement that every
member uses two-factor authentication, and the audit log of changes (an Enterprise feature, with a
configurable retention).

**Tracking.** The snippet to paste into the `<head>` of your site, a way to send a test event from
your terminal, and the installation status that flips once the first event arrives. See
[Tracking visitors and events](/guide/tracking).

**Sending.** Integrations (the SMTP or SES providers the workspace sends through, with the
inbound hook for bounces and complaints), sending domains with the DKIM, SPF and DMARC records to
publish, the optional tracking domain for links in your mail, and the suppression list of addresses
that are never mailed. See [Deliverability and consent](/guide/deliverability) and
[Presets for provider callbacks](/guide/presets).

**Developers.** API keys and webhook endpoints. See [API](/guide/api) and
[Webhooks](/guide/webhooks).

**AI agents (MCP).** The address to add as a connector in Claude or any MCP client so an agent can
work in the workspace. Only an owner or admin can approve the connection. See
[MCP for agents](/guide/mcp).
