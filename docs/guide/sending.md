# Sending email

sphericon has three send surfaces. They differ in who triggers them, and they share templates,
sending domains and consent rules.

## Templates

Templates are written in [MJML](https://mjml.io/), so responsive email works across clients, and
are compiled when the message is sent. Subjects and bodies support [Liquid](https://shopify.github.io/liquid/)
(conditionals and filters). Merge tags come from the contact: <code v-pre>{{ email }}</code>, <code v-pre>{{ first_name }}</code>,
<code v-pre>{{ last_name }}</code> and any of its custom fields; transactional calls can add their own variables.
Tracking (an open pixel, click rewriting and the unsubscribe footer) is layered on after
rendering, and a plain-text part is generated for you.

For marketing sends the template content is **copied** into the broadcast or automation step when
you author it, so editing a template later never rewrites mail that is already queued.
Transactional sends do the opposite and bind the template **by reference**, so your app can
update its receipt wording without a deploy.

## Broadcasts

A broadcast is one email to a segment.

1. Set a name, sender, subject and body.
2. Choose the audience segment.
3. Send a test. Without a verified sending domain it goes only to your own address, from the
   platform's sender (the editor shows which From it used); with one, to any address you name,
   through your Integration. See the [Quickstart](/guide/quickstart#see-an-email-before-dns).
4. Send it now, or schedule it. A scheduled broadcast can be unscheduled again.

A broadcast moves through `draft`, `scheduled`, `sending`, `sent` or `failed`. Its report lists
sent, opened, clicked and unsubscribed counts. Sending runs on a durable job queue backed by
PostgreSQL, so a restart does not lose work.

## Automations

An automation starts when a contact does something and then runs an ordered list of steps.

| Step         | What it does                                        |
| ------------ | --------------------------------------------------- |
| `email`      | Send a message with its own subject and MJML body.  |
| `wait`       | Pause for a number of seconds before the next step. |
| `apply_tag`  | Add a tag to the enrolled contact.                  |
| `remove_tag` | Remove a tag from the enrolled contact.             |

The trigger is an event action, such as `signed_up`. A contact is enrolled once. Automations are
created as drafts and have to be activated. Each automation is its own unsubscribe scope, so
leaving one sequence does not silence the others.

## Transactional email

Your application sends a single email to one address with `POST /api/emails`, naming a template
and the variables to fill it with:

```sh
curl https://sphericon.example.com/api/emails \
  -H "Authorization: Bearer $SPHERICON_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "templateId": 12,
    "destination": "ada@example.com",
    "variables": { "reset_url": "https://app.example.com/reset?token=..." }
  }'
```

The token needs the `emails:send` scope. Transactional mail skips marketing unsubscribes, since
nobody can opt out of their own password reset, but it still respects the suppression list: an
address that hard-bounced or complained is never mailed. A suppressed destination returns the
status `suppressed`.

## Transports

Add one or more integrations in **Settings → Developers**:

- **SMTP**: any server that accepts SMTP.
- **SES-compatible API**: Amazon SES, or any service that implements the same API, such as Yandex
  Cloud Postbox. Set the integration's **endpoint** to the service's URL; leave it empty for
  Amazon SES itself.

Bounces and complaints become suppressions only through the integration's hook, set up under
**Inbound** with a Preset (SES, Postmark, Mailgun or generic). Plain SMTP gets none: see
[Bounces and complaints](/guide/deliverability#bounces-and-complaints).

Credentials are encrypted at rest. Because DKIM signing happens in sphericon itself, switching
transport does not mean re-verifying your domain.
