<h1 align="center">Sphericon</h1>

<p align="center">
  Open-core marketing automation you can run yourself.<br />
  <a href="https://getsphericon.com/docs/">Documentation</a> ·
  <a href="https://getsphericon.com/docs/self-hosting">Self-hosting</a> ·
  <a href="ROADMAP.md">Roadmap</a>
</p>

<p align="center">
  <a href="LICENSING.md"><img alt="License: AGPL-3.0" src="https://img.shields.io/badge/license-AGPL--3.0-blue" /></a>
</p>

> [!WARNING]
> Sphericon is in early development and changes fast. Until 1.0, any release may break the API,
> the configuration or the stored data, and features may change or go away. Try it and tell us
> what you think, but do not rely on it in production yet. Read the [changelog](CHANGELOG.md)
> before you upgrade; the [roadmap](ROADMAP.md) says what comes next.

![The Sphericon workspace overview: email engagement, contacts and automations](docs/public/screenshot.png)

## About

Sphericon helps a business know its audience and talk to it at the right moment: it collects who
your contacts are and what they do on your site or in your product, groups them into segments,
and messages them by email and Telegram. Everything lives in your own workspace on your own infrastructure, so the
customer data stays yours. It is a self-hostable alternative to tools like Drip, without the
lock-in.

## Features

- **Contacts and events** — one profile per person, built from imports, the API and the
  tracking snippet on your site, with typed custom fields (text, number, date and more).
- **Segments and tags** — audiences defined by rules over contacts, tags and events.
- **Channels** — email and Telegram, each through its own integration, with consent and
  sending limits honored per channel.
- **Broadcasts** — one-off messages to a segment, with per-broadcast stats.
- **Automations** — sequences triggered by what a contact does, with waits, messages, conditions
  and HTTP calls to your own services.
- **Transactional email** — send single messages through the API.
- **Deliverability built in** — unsubscribes, bounces and spam complaints are honored
  automatically, mail is signed with your own DKIM key and sent with one-click unsubscribe,
  so you never write to someone who asked you to stop.
- **API and webhooks** — a workspace-scoped API with
  scoped tokens, and webhooks for what happens next.
- **Agent-ready** — an MCP server lets an AI agent
  draft campaigns; sending stays opt-in per token.
- **Privacy tools** — GDPR export and erasure, plus an audit log of who changed what.
- **Multi-user workspaces** — owners, admins and members, with two-factor sign-in and an overview
  that leads with what needs your attention.

## Getting started

One program and one database: a Go binary (or Docker image) with the web app, the tracker and
the migrations built in, plus PostgreSQL. No Redis, no object storage, no separate worker.

With Docker Compose, which runs PostgreSQL, the migrations and the app together:

```sh
# 1. Download the Compose file and the example environment
curl -fsSLO https://raw.githubusercontent.com/getsphericon/sphericon/main/compose.yaml
curl -fsSL https://raw.githubusercontent.com/getsphericon/sphericon/main/compose.env.example -o .env

# 2. Fill in .env: POSTGRES_PASSWORD, SPHERICON_JWT_SECRET (openssl rand -hex 32) and
#    SPHERICON_ENCRYPTION_KEY (docker run --rm ghcr.io/getsphericon/sphericon:latest genkey)

# 3. Start it
docker compose up -d
```

Then put a reverse proxy with HTTPS in front of port 3000 and open it to create your account.
The full guide covers HTTPS, the first run, plain Docker, the single binary and Kubernetes
(Helm): [self-hosting](https://getsphericon.com/docs/self-hosting). Every environment variable is in
[configuration](https://getsphericon.com/docs/configuration).

Images are published to `ghcr.io/getsphericon/sphericon` (linux amd64/arm64) on every release,
and release archives for linux and darwin are attached to each GitHub release.

## Tech stack

Go and PostgreSQL on the backend; React, Vite and Mantine on the frontend; TypeSpec as the single
source of truth for the API contracts.

## Contributing

Setup with a single prerequisite ([mise](https://mise.jdx.dev)), the code generation pipeline,
dev accounts and the pull request rules are in [`CONTRIBUTING.md`](CONTRIBUTING.md).

## License

Sphericon is **open-core**. The core is licensed under the GNU AGPL-3.0
(`LICENSE`); the Enterprise features under `ee/` are commercial and
source-available (`ee/LICENSE`). See [`LICENSING.md`](LICENSING.md) for the
boundary. Contributions need a signed CLA.

## Security

To report a vulnerability, see [`SECURITY.md`](SECURITY.md).
