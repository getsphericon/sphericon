<h1 align="center">Sphericon</h1>

<p align="center">
  Open-core marketing automation you can run yourself.<br />
  <a href="https://getsphericon.com/docs/">Documentation</a> ·
  <a href="https://github.com/getsphericon/sphericon/releases">Releases</a> ·
  <a href="https://getsphericon.com/docs/self-hosting">Self-hosting</a>
</p>

<p align="center">
  <a href="LICENSING.md"><img alt="License: AGPL-3.0" src="https://img.shields.io/badge/license-AGPL--3.0-blue" /></a>
  <a href="https://github.com/getsphericon/sphericon/releases"><img alt="Latest release" src="https://img.shields.io/github/v/release/getsphericon/sphericon" /></a>
  <a href="https://github.com/getsphericon/sphericon/stargazers"><img alt="GitHub stars" src="https://img.shields.io/github/stars/getsphericon/sphericon" /></a>
</p>

![The Sphericon workspace overview: email engagement, contacts and automations](docs/public/screenshot.png)

## About

Sphericon helps a business know its audience and talk to it at the right moment: it collects who
your contacts are and what they do on your site or in your product, groups them into segments,
and sends them email. Everything lives in your own workspace on your own infrastructure, so the
customer data stays yours. It is a self-hostable alternative to tools like Drip, without the
lock-in.

## Features

- **Contacts and events** — one profile per person, built from imports, the API and the
  [tracking snippet](https://getsphericon.com/docs/guide/tracking) on your site.
- **Segments and tags** — audiences defined by rules over contacts, tags and events.
- **Broadcasts** — one-off email to a segment, with per-broadcast stats.
- **Automations** — sequences triggered by what a contact does, with waits and emails.
- **Transactional email** — send single messages through the API.
- **Deliverability built in** — unsubscribes, bounces and spam complaints are honored
  automatically, mail is signed with your own DKIM key and sent with one-click unsubscribe,
  so you never write to someone who asked you to stop.
- **API and webhooks** — a workspace-scoped [API](https://getsphericon.com/docs/guide/api) with
  scoped tokens, and webhooks for what happens next.
- **Agent-ready** — an [MCP server](https://getsphericon.com/docs/guide/mcp) lets an AI agent
  draft campaigns; sending stays opt-in per token.
- **Privacy tools** — GDPR export and erasure.
- **Multi-user workspaces** — owners, admins and members, with two-factor sign-in.

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
and release archives for linux and darwin are attached to each
[GitHub Release](https://github.com/getsphericon/sphericon/releases).

## Tech stack

Go ([ogen](https://ogen.dev/), [ent](https://entgo.io/), [river](https://riverqueue.com/),
[watermill](https://watermill.io/)) and PostgreSQL on the backend; React, Vite and
[Mantine](https://mantine.dev/) on the frontend; [TypeSpec](https://typespec.io/) as the single
source of truth for the API contracts.

## Contributing

Setup with a single prerequisite ([mise](https://mise.jdx.dev)), the code generation pipeline,
dev accounts and the pull request rules are in [`CONTRIBUTING.md`](CONTRIBUTING.md). Architecture
and conventions are in [`AGENTS.md`](AGENTS.md), the domain language in
[`GLOSSARY.md`](GLOSSARY.md).

## License

Sphericon is **open-core**. The core is licensed under the GNU AGPL-3.0
([`LICENSE`](LICENSE)); the Enterprise features under [`ee/`](ee/) are commercial and
source-available ([`ee/LICENSE`](ee/LICENSE)). See [`LICENSING.md`](LICENSING.md) for the
boundary. Contributions need a signed [CLA](CLA.md).

## Security

To report a vulnerability, see [`SECURITY.md`](SECURITY.md).
