# Quickstart

This walks you from a running instance to your first broadcast.

## 1. Run sphericon

::: tip Prefer not to run it?
[Sphericon Cloud](/guide/cloud) is the hosted edition: skip this step and start at step 2.
:::

The quickest way is the published image. PostgreSQL is the only dependency.

```sh
docker run -p 3000:3000 \
  -e APP_ENV=production \
  -e DATABASE_URL="postgres://user:pass@host:5432/sphericon?sslmode=require" \
  -e APP_URL="https://example.com" \
  -e JWT_SECRET="$(openssl rand -hex 32)" \
  -e ENCRYPTION_KEY="<base64 keyset>" \
  -e AUTO_MIGRATE=true \
  ghcr.io/mokevnin/sphericon:latest
```

Every variable, and the alternative of running the single binary, is covered in
[Self-hosting](/self-hosting). To explore with sample data instead, follow the development
setup in the [README](https://github.com/mokevnin/sphericon#development).

## 2. Create an account and a workspace

Open the app, register, and create a workspace. A workspace is where everything else lives.
You can invite teammates later from **Settings**.

## 3. Connect a way to send

In **Settings → Integrations** add an SMTP server or an SES-compatible service. Credentials are stored
encrypted with your `ENCRYPTION_KEY`.

Then add a **sending domain** and publish the DKIM record sphericon shows you. Mail can only be sent
from a verified domain; see [Deliverability and consent](/guide/deliverability).

## 4. Bring in contacts

Either [track your site or product](/guide/tracking) so contacts appear as people identify
themselves, or create and import them through the [API](/guide/api).

## 5. Define a segment

Open **Segments**, build a rule, and watch the live count of matching contacts. See
[Segments](/guide/segments).

## 6. Send a broadcast

1. Create a template, or write the body directly in the broadcast.
2. Create a broadcast, set the sender and subject, and choose your segment as the audience.
3. Send a test to yourself, then send it or schedule it.

The broadcast report shows sent, opened, clicked and unsubscribed counts. Details are in
[Sending email](/guide/sending).
