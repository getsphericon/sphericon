# Quickstart

This walks you from a running instance to your first broadcast. The same four steps appear as the
**Setup checklist** on the Overview of a new workspace.

## Run sphericon

::: tip Prefer not to run it?
[Sphericon Cloud](/guide/cloud) is the hosted edition: skip this section and start at "Create an account and a workspace".
:::

The quickest way is the published image. PostgreSQL is the only dependency.

```sh
docker run -p 3000:3000 \
  -e SPHERICON_ENV=production \
  -e DATABASE_URL="postgres://user:pass@host:5432/sphericon?sslmode=require" \
  -e SPHERICON_APP_URL="https://example.com" \
  -e SPHERICON_JWT_SECRET="$(openssl rand -hex 32)" \
  -e SPHERICON_ENCRYPTION_KEY="<base64 keyset>" \
  -e SPHERICON_DB_AUTO_MIGRATE=true \
  ghcr.io/getsphericon/sphericon:latest
```

Every variable, and the alternative of running the single binary, is covered in
[Self-hosting](/self-hosting). To explore with sample data instead, follow the development
setup in the [README](https://github.com/getsphericon/sphericon#development).

## Create an account and a workspace

Open the app, register, and create a workspace. A workspace is where everything else lives.
You can invite teammates later from **Settings**.

The Overview of a workspace that has not sent an email yet shows the **Setup checklist**: the four
steps below, each with a link to the page where you do it. A step turns done by itself when the
thing exists, so you can take them in any order, and none waits for another. The checklist goes
away once every step is done.

## See an email before DNS

An aside, not one of the four steps: you do not have to finish them to see sphericon send. Open a broadcast, write something
and press **Send test**. While the workspace has no verified sending domain, the test goes out from
the Instance's own sender (the one that sends verification and password reset mail) and needs no
Integration. It has two limits:

- It goes only to your own address, the one you registered with. Any other address is refused.
- Its From is the platform's address, not your domain. The editor shows the address it used, so
  you know it is not what your contacts will see. It is not signed with your domain either.

It is capped per person and per workspace, so it is for looking at an email, not for sending one.
A test send is not a real send: it never shows in Activity or the rates, and it does not complete
the last checklist step. On an instance with no outbound mail configured there is no such sender,
and the test is refused with that reason. Once a sending domain is verified, a test goes through
your Integration and your domain, to any address you name.

## The four steps

### 1. Connect an Integration

In **Settings → Sending** add an SMTP server or an SES-compatible service. Credentials are stored
encrypted with your `SPHERICON_ENCRYPTION_KEY`.

### 2. Verify a sending domain

In **Settings → Sending** add a **sending domain** and publish the DKIM record sphericon
shows you. Mail to contacts can only be sent from a verified domain; see
[Deliverability and consent](/guide/deliverability).

DNS can take time, and nothing else waits for it. While the record has not propagated the domain
shows as waiting for DNS, and you can go on with the next steps and prepare your first broadcast.

### 3. Bring in contacts

Either [track your site or product](/guide/tracking) so contacts appear as people identify
themselves, or create and import them through the [API](/guide/api). One contact completes this step.

### 4. Send your first email

1. Create a template, or write the body directly in the broadcast.
2. Create a broadcast, set the sender and subject, and choose an audience. Optionally define a
   [segment](/guide/segments) first and use it as the audience.
3. Send a test to yourself, then send it or schedule it. Sending to contacts needs a verified
   sending domain.

The step is done once any email has been sent, from a broadcast, an automation or a transactional
call. The broadcast report shows sent, opened, clicked and unsubscribed counts. Details are in
[Sending email](/guide/sending).
