# Sphericon hosted deployment: runbook

The hosted Sphericon deployment (AWS: ECS Fargate behind an ALB, RDS PostgreSQL, Route 53, SES,
Secrets Manager; region `us-east-2`, one account) is described only by the Terraform in this
directory. Why and what: [ADR 0028](../docs/adr/0028-saas-hosting-terraform-on-aws-ecs-fargate.md).
Follow the sections in order: prerequisites, manual steps, bootstrap, plan, apply, registrar,
smoke test; destroy and recreate and troubleshooting come after.

**The AWS MCP server is for read-only inspection (status, logs, listings), never for changes.**
Every change goes through Terraform, otherwise the state drifts and the next apply reverts it.

## 1. Prerequisites

- `mise install` provides `terraform`, `aws` (AWS CLI), `jq`, `shellcheck` and `go`.
- An AWS account with the CLI profile `sphericon` (region `us-east-2`), signed in with
  `aws login --profile sphericon`. The session is short lived: sign in again when `mise run infra`
  says there are no credentials. No access keys are stored.
- The registrar of `getsphericon.app`, and the GitHub package settings of the image.
- `mise run check:infra` (fmt, `init -backend=false`, `validate`) and `mise run check:shell` need
  no credentials; CI and the git hooks run them.

`mise run infra -- <terraform args>` runs Terraform in `infra/` with credentials exported from that
session and the state bucket passed as the only `-backend-config` (a name, not a secret). Nothing
secret is printed or put in arguments.

## 2. Manual steps Terraform cannot do

- [ ] **`aws login --profile sphericon`** (above).
- [ ] **First release, so an image exists.** The image is built and pushed to GHCR only by the
      release workflow (goreleaser, after release-please cuts a release): merge the release-please
      PR on `main`. Until then there is no image and the service cannot start. Note the version
      (without the `v`): it is `image_tag`.
- [ ] **Make the GHCR package public** (GitHub, the `sphericon` package, Package settings, Change
      visibility). ECS pulls anonymously; this cannot be done with Terraform, and a private package
      makes the tasks fail with an image pull error.
- [ ] **Registrar nameservers** (the only manual DNS step: everything else in `getsphericon.app` is
      Terraform): after the zone exists (section 5), set the four Route 53 `name_servers`
      at the registrar.
- [ ] **SES production access**: a new account starts in the SES sandbox (delivers only to
      verified addresses, low quota). Request production access in the SES console (Account
      dashboard) for `us-east-2`.
- [ ] **`LICENSE_KEY`** (optional): `mise run infra:license < key.txt`. Empty runs the open-source
      core.

## 3. Bootstrap (once, idempotent)

```sh
mise run infra:bootstrap
```

It creates, only when missing: the state bucket `sphericon-tfstate-<account id>` (private,
versioned, encrypted, public access blocked, TLS only) and the secret `sphericon/app` holding
`JWT_SECRET`, `ENCRYPTION_KEY` (from `sphericon genkey`), `BOOTSTRAP_TOKEN` and an empty
`LICENSE_KEY`. An existing secret is never changed, so **`ENCRYPTION_KEY` is never rotated**: it
protects stored provider credentials, and a new key makes them unreadable. Values are generated in
the script and never printed. Re-running it is safe.

## 4. Variables and plan

Create `infra/production.tfvars` (gitignored) with the plain values, at least:

```hcl
image_tag = "0.1.0" # the released version
```

Optional: `image_repository` (default `ghcr.io/getsphericon/sphericon`), `otel_service_name`,
`domain` (`getsphericon.app`, also the app host), `api_host_label` (`api`),
`tracker_host_label` (`t`), `region`, `name`, `task_cpu`, `task_memory`, `db_instance_class`,
`db_allocated_storage`, `db_max_open_conns`, `pgx_max_conns`, `mail_from_label` (`mail`),
`system_email_from` (default `noreply@<domain>`), `dmarc_rua`. There are no secret variables.

```sh
mise run infra -- plan -out=tfplan
```

Read it before applying: only expected resources (VPC, subnets, security groups, RDS, ECS, ALB,
certificate, zone and records, SES identity, IAM roles and one send-only user, two secrets);
secret values show as `(sensitive value)`; nothing is destroyed on a first run.

## 5. Apply (two steps on a fresh account)

The certificate is validated through DNS, and the HTTPS listener waits for it, so the zone must
exist and be delegated first:

```sh
mise run infra -- apply -target=aws_route53_zone.this     # the zone only
mise run infra -- output name_servers
```

Set those nameservers at the registrar (propagation can take hours; `dig NS getsphericon.app`).
Then the full apply:

```sh
mise run infra -- plan -out=tfplan
mise run infra -- apply tfplan
```

Order inside the apply: network, RDS, secrets, certificate (waits for DNS), ALB, task definitions,
**the migration**, then the service. The migration is a one-off `aws ecs run-task` of the
`sphericon-migrate` task definition (`sphericon migrate`: goose and river migrations) that the
apply waits for; the service depends on it, so a failed migration fails the apply and leaves the
service on the previous revision. There is no ECS pre-deploy hook; this step takes its place and
re-runs whenever the migrate task definition changes (a new `image_tag`). The apply therefore needs
the AWS CLI and credentials in its environment: use `mise run infra`, not a bare `terraform`.
The first apply takes 15 to 25 minutes (RDS is the longest).

To deploy a new release: set `image_tag` in `production.tfvars`, plan, apply.

## 6. Smoke test

`curl` the ALB before DNS if needed: `curl -sk -H 'Host: getsphericon.app'
https://$(mise run infra -- output -raw alb_dns_name)/readyz`.

- [ ] Readiness: `curl -s https://getsphericon.app/readyz` returns `200` (database reachable);
      `/healthz` as well.
- [ ] App host: `curl -sI https://getsphericon.app/` returns `200` with a valid certificate.
- [ ] HTTP redirects: `curl -sI http://getsphericon.app/` returns `301` to HTTPS.
- [ ] Apex is the load balancer: `dig +short A getsphericon.app` returns the ALB addresses.
- [ ] API host rewrite: `curl -si https://api.getsphericon.app/contacts` returns `401` with
      `content-type: application/problem+json` (the external API at `/api/contacts`, not the SPA).
- [ ] Tracker host: `curl -sI https://t.getsphericon.app/t.js` returns `200` with a JavaScript
      content type; `curl -si -X POST https://t.getsphericon.app/collect/<path>` without a key
      returns `401` problem+json; `curl -si https://t.getsphericon.app/` returns `404`.
- [ ] Email: trigger a password reset (in the SES sandbox, to a verified address). The message
      arrives and its original headers show `dkim=pass`, `spf=pass`, `dmarc=pass`. The SES
      console (`us-east-2`) shows identity Verified, DKIM Successful, MAIL FROM Success.
- [ ] Mail records: `dig +short MX mail.getsphericon.app` (`feedback-smtp.us-east-2.amazonses.com`),
      `dig +short TXT mail.getsphericon.app` (the SES SPF), `dig +short TXT _dmarc.getsphericon.app`
      (`p=none`), and the three DKIM CNAMEs `<token>._domainkey.getsphericon.app`.
- [ ] Database private: it has no public address (`aws rds describe-db-instances` shows
      `PubliclyAccessible: false`) and the plan output showed no secret values.

## 7. Destroy and recreate

The environment is a test deployment: no deletion protection, no final snapshot.

```sh
mise run infra -- destroy
```

Destroy removes the service, the database **and all its data**, the zone, the SES identity, the
runtime secret and the records. The state bucket and `sphericon/app` stay (never delete them with
the environment). To recreate, run sections 4 to 6 again, with these effects:

- **`ENCRYPTION_KEY`** is unchanged (it is in `sphericon/app`). The database is new, so nothing is
  lost.
- **Nameservers**: the zone is created again with different nameservers. Update the registrar
  before the full apply, or the certificate never validates.
- **SES**: the identity is created again with new DKIM tokens, so verification repeats after the
  nameservers propagate. Production access is per account and region and is kept.

**Pending, operator only:** none of this has been applied yet. The smoke test, a destroy and
recreate following only this runbook, the ALB URL rewrite, `max_connections` of the real instance
and a password-reset email passing DKIM and SPF need the operator's account and registrar.

## 8. Troubleshooting

- **The apply fails in the migration step.** The script prints the task's exit code, stop reason
  and the last log lines; the full log is in CloudWatch group `/ecs/sphericon`, stream
  `migrate/migrate/<task id>`. Fix the cause and re-run `apply`: the step runs again. The service
  stays on the previous revision meanwhile.
- **Tasks cannot start (`CannotPullContainerError`).** The GHCR package is still private, or
  `image_tag` does not exist. Check both (section 2).
- **Tasks cannot start (`ResourceInitializationError` on secrets).** `sphericon/app` is missing
  (run `infra:bootstrap`) or lacks a key; the execution role reads only the two secrets.
- **The apply hangs on the certificate.** The nameservers are not delegated yet (section 5).
- **API host returns the SPA or a 404.** Routing is by `Host` plus the listener-rule URL rewrite
  (`^/(.*)$` to `/api/$1`, `alb.tf`). `200` with HTML means the host fell through to the app rule;
  `404` problem+json means the rewrite did not apply (check the rule's transform in the console).
- **The tracker host serves the SPA.** It must not: only `/t.js` and `/collect` have a rule.
- **Connection budget.** `db.t4g.micro` allows about 85-110 connections (read `SHOW
  max_connections`). Each process opens two pools, 10 + 10; old plus new task during a deploy is 40
  and the migrate task adds about 4 (goose and river's migrator, 1-2 connections each). Before a
  second permanent task or another process, lower both pools or move to a larger class. No
  transaction-mode pooler: river needs LISTEN/NOTIFY. Symptom of overrun: `too many connections` in
  the logs.
- **Email not delivered.** The SES sandbox accepts only verified recipients until production access
  is granted; check the identity status first. The app uses the SES API
  (`SYSTEM_EMAIL_PROVIDER=ses`, see the env table in `docs/self-hosting.md`).
- **State lock errors.** The lock is a `.tflock` object next to the state in the bucket; run one
  operator at a time. After a crashed run, `mise run infra -- force-unlock <id>`.
- **`no credentials for profile`.** `aws login --profile sphericon` (sessions expire).
