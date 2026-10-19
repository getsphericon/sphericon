# Runbook

One section per alert in the [example rules](./monitoring#example-prometheus-rules). Each alert
links here by name. A section says what the alert means, what to check first and how to fix
it. The commands assume a Kubernetes deployment (`kubectl`) and `psql` access to the database;
translate them for Docker or a bare binary.

Three things help with almost every alert:

- **Logs.** The process logs structured JSON. Failed jobs log `river job errored` or
  `river job panicked` with `kind`, `queue`, `job_id` and `attempt`; provider hooks log
  `hooks/ses: ...` lines with the `workspace_id`.
- **`/readyz`.** If it returns `503`, the database is the first suspect.
- **Rollouts.** Check whether the alert started with a deploy or a migration
  ([Upgrading](./upgrading)) before digging further.

## SphericonDown

**Means.** Prometheus cannot scrape a Sphericon metrics target for 5 minutes. The process is down,
unreachable, or the metrics listener is not enabled.

**Check.**

1. Are the pods running and ready? `kubectl get pods -l app.kubernetes.io/name=sphericon`
2. Why did they stop? `kubectl describe pod ...` (OOMKilled, failed probe) and
   `kubectl logs --previous ...`.
3. Is it only the scrape? Hit `/healthz` on the public port. If the app is fine, check
   `SPHERICON_METRICS_ADDR` (or `metrics.enabled`), the metrics Service and any network policy between
   Prometheus and the pod.
4. If `/readyz` returns `503`, the database is unreachable: see
   [`SphericonDBPoolSaturated`](#sphericondbpoolsaturated) and check PostgreSQL itself.

**Fix.** Restart on a crash loop caused by configuration (a missing secret, an invalid
`SPHERICON_ENCRYPTION_KEY`, a port clash between `PORT` and `SPHERICON_METRICS_ADDR` all fail startup with a clear
log line). After a bad deploy, roll back the image. Pending migrations are not applied
automatically with several replicas: run them as described in [Upgrading](./upgrading).

## SphericonCollectErrorRatioHigh

**Means.** More than 1% of `/collect` requests (tracker events and identifies from customer
sites) answered 5xx. Those events are lost unless the client retries, so this is the most
urgent request alert.

**Check.**

1. Which operation? `sum by (operation_id, http_response_status_code) (rate(ogen_server_request_count_total{operation_id=~"Collect.*"}[5m]))`.
2. The collect path writes to the domain-event outbox in PostgreSQL, so look for database
   trouble first: `/readyz`, [`SphericonDBPoolSaturated`](#sphericondbpoolsaturated),
   [`SphericonDBPoolWaits`](#sphericondbpoolwaits), disk full, a failover in progress.
3. Read the error logs of the failing pods for the underlying error.

**Fix.** Restore database health (free disk, resolve locks, fail over). If a deploy caused it,
roll back. If load is the cause, add replicas, but check the pool sizes first, because each
replica adds connections.

## SphericonSiteErrorRatioHigh

**Means.** More than 5% of dashboard API (`/site`) requests answered 5xx for 10 minutes. People
see errors in the UI.

**Check.**

1. Narrow it down: `topk(10, sum by (operation_id) (rate(ogen_server_request_count_total{operation_id=~"Site.*", http_response_status_code=~"5.."}[5m])))`.
   One operation points at a bug or a slow query, all of them point at the database or a deploy.
2. Compare with [`SphericonRequestLatencyHigh`](#sphericonrequestlatencyhigh) and the pool alerts.
3. Look at the logs for the failing operation and at what changed (deploy, migration, a large
   import or broadcast running at the same time).

**Fix.** Roll back a bad release, relieve the database, or fix the failing query. A single
operation failing for a single workspace is usually a data problem, not an outage.

## SphericonApiErrorRatioHigh

**Means.** More than 5% of external API (`/api`, also used by MCP clients) requests answered
5xx for 10 minutes. Integrations calling your instance are failing.

**Check.** The same as [`SphericonSiteErrorRatioHigh`](#sphericonsiteerrorratiohigh), with
`operation_id!~"Site.*|Collect.*"`. Also look for one noisy client: a large batch import can
saturate the database for everyone.

**Fix.** As for `/site`. If one client is the cause, ask it to slow down or lower its batch
size.

## SphericonRequestLatencyHigh

**Means.** The 95th percentile of `/site` and `/api` request duration has been above 2 seconds
for 15 minutes. `/collect` is excluded because it should always be fast.

**Check.**

1. Which operations are slow?
   `histogram_quantile(0.95, sum by (le, operation_id) (rate(ogen_server_duration_milliseconds_bucket{operation_id!~"Collect.*"}[5m])))`
2. Is the database the bottleneck? Check pool usage and waits
   ([`SphericonDBPoolSaturated`](#sphericondbpoolsaturated)) and PostgreSQL itself (CPU, IO, long
   queries in `pg_stat_activity`).
3. Large `events` tables make segment and analytics queries slow; confirm Event retention is
   running ([ADR 0019](https://github.com/getsphericon/sphericon/blob/main/docs/adr/0019-event-retention.md)).

**Fix.** Kill or fix the slow query, add a missing index, or scale the database. Adding
replicas does not help if the database is the limit.

## SphericonOutboxLagHigh

**Means.** A domain-event consumer group is more than 5 minutes behind. The groups are
`persist` (writes the Event rows that analytics and segments read), `automations` (enrolls
contacts), `webhooks` (dispatches outgoing webhooks) and `suppression` (adds bounced and
complaining addresses to the suppression list). The `consumer_group` label tells you which one.

**Impact.** Lag in `suppression` delays blocking of hard-bouncing addresses, which hurts
reputation. Lag in `persist` makes dashboards stale. The outbox is also only pruned below the
slowest consumer's cursor, so a stuck group makes it grow.

**Check.**

1. Is only one group behind? That group's handler is failing or stuck. Look at
   `SphericonEventHandlerErrors` for its handler and read the logs.
2. Are all groups behind? The router is not running (process restarting, database trouble) or
   the database is slow. Check [`SphericonDown`](#sphericondown) and the pool alerts.
3. Lag drops by itself after a spike (a big import, an SES hook burst) once consumers catch up:
   watch the trend before acting.

**Fix.** A message that keeps failing is redelivered and holds its group back: fix the cause
(a bug, a bad payload, an unreachable dependency) and ship a release; the group resumes from
its cursor with no data loss. If a deploy introduced it, roll back.

## SphericonEventHandlerErrors

**Means.** More than 5% of messages handled by one event handler returned an error for 15
minutes. The `handler` label is one of `persist_event`, `enroll_automations`,
`dispatch_webhooks` or `update_suppression`.

**Check.**

1. The logs of the process around the failures, for the underlying error.
2. `update_suppression` and `persist_event` fail mostly on database errors. `enroll_automations`
   fails on a broken automation definition. `dispatch_webhooks` should not fail on a customer's
   unreachable endpoint (delivery is a river job), so a failure there points at Sphericon itself.
3. Correlate with [`SphericonOutboxLagHigh`](#sphericonoutboxlaghigh): failures that keep a message
   from being acknowledged show up as growing lag for that handler's group.

**Fix.** Fix the cause and deploy. Failed messages are redelivered and the handlers are written to
tolerate redelivery, so there is nothing to replay by hand.

## SphericonJobQueueBacklog

**Means.** In one river queue (`default`, `broadcasts` or `webhooks`) the oldest job that is
ready to run has been waiting more than 15 minutes.

**Check.**

1. Is it just load? A large broadcast legitimately queues many jobs. If
   `river_queue_depth` is high but falling, wait.
2. Is anything being worked? `sum by (queue) (rate(river_work_count_total[5m]))`. Zero with a
   growing backlog means the workers are not running or are stuck.
3. Are the jobs failing and retrying? See
   [`SphericonJobFailureRatioHigh`](#sphericonjobfailureratiohigh).
4. Workers need database connections from the `pgx` pool: check `db_pool_in_use{pool="pgx"}`
   against `SPHERICON_PGX_MAX_CONNS` ([`SphericonDBPoolSaturated`](#sphericondbpoolsaturated)).
5. Check that the river schema is applied. It is created by `sphericon migrate`; an instance that
   skipped migration has no job tables and the job metrics and workers fail.

**Fix.** Restart stuck pods, raise `SPHERICON_PGX_MAX_CONNS` (and the database `max_connections`
accordingly) if the pool is the limit, or add replicas. Worker concurrency per queue is fixed
in the binary.

## SphericonJobFailureRatioHigh

**Means.** More than 20% of job attempts in a queue ended in `error` or `panic` for 30
minutes. Because retries count, this overstates how many jobs end up discarded, but a
sustained high ratio always needs a look.

**Check.**

1. Which kind? `topk(10, sum by (kind) (rate(river_work_count_total{status!="ok"}[15m])))`.
2. Read the `river job errored` and `river job panicked` log lines for that `kind`; they carry
   the error, `attempt` and `max_attempts`.
3. Common causes: the email provider rejecting sends (see
   [`SphericonEmailSendErrorsHigh`](#sphericonemailsenderrorshigh)), a customer's webhook endpoint
   down (the `webhooks` queue retries up to 10 times), DNS for sending-domain checks, or a bug.
4. Jobs that used up their attempts are in state `discarded` and are kept for 14 days:

   ```sql
   SELECT kind, count(*), max(finalized_at)
   FROM river_job WHERE state = 'discarded'
   GROUP BY kind ORDER BY 2 DESC;
   ```

**Fix.** Fix the cause. Failing webhook deliveries to one customer endpoint are that
customer's problem; tell them. Discarded jobs are not retried automatically: after the fix,
re-run the underlying action (for example, re-send the broadcast to the missed recipients).

## SphericonDBPoolSaturated

**Means.** One connection pool has been at or above 90% of its maximum for 10 minutes. `pool`
is `sql` (requests, ent, event consumers; `SPHERICON_DB_MAX_OPEN_CONNS`, default 15) or `pgx` (the job
queue; `SPHERICON_PGX_MAX_CONNS`, default 25).

**Check.**

1. Slow queries holding connections: `SELECT pid, state, wait_event, now() - query_start AS age, left(query, 80) FROM pg_stat_activity WHERE datname = current_database() ORDER BY age DESC;`
2. Idle-in-transaction sessions or locks blocking others.
3. Does the total across replicas fit `max_connections`? Each replica opens both pools; see
   [Sizing](./#sizing).

**Fix.** Remove the slow query or the lock first. If the load is real, raise
`SPHERICON_DB_MAX_OPEN_CONNS` or `SPHERICON_PGX_MAX_CONNS` and make sure the sum over all replicas stays below the
server's `max_connections`; beyond that, add a connection pooler. `SPHERICON_DB_MAX_IDLE_CONNS` must not
exceed `SPHERICON_DB_MAX_OPEN_CONNS`.

## SphericonDBPoolWaits

**Means.** More than 100 times in 15 minutes a request or worker had to wait for a free
connection (`db_pool_wait_count` for the `sql` pool, `db_pool_empty_acquire_count` for the `pgx`
pool). Unlike saturation this catches short, repeated stalls.

**Check and fix.** The same as [`SphericonDBPoolSaturated`](#sphericondbpoolsaturated). Waiting
that continues while `db_pool_in_use` is well below `db_pool_max` points at slow connection
establishment (network, TLS, a pooler) instead of a too-small pool. On the `pgx` pool an acquire
that only waited for a new connection to open also counts, so a cold or restarted pool can trip it.

## SphericonEmailSendErrorsHigh

**Means.** More than 10% of send attempts through a provider (`smtp` or `ses`) failed for 15
minutes. These are failures to hand a message to the provider, not bounces.

**Check.**

1. The send error in the logs of the `river job errored` lines for the send jobs: credentials
   revoked, a provider outage, throttling, an unverified sender or sending domain.
2. For SES, check the AWS account status (sandbox, suspended sending, quota) and region.
   For SMTP, check the relay's status page and your credentials.
3. Is it one Workspace? Provider credentials are per Workspace, so a single wrong credential
   fails only that Workspace's sends.

**Fix.** Rotate or correct the credentials in the Workspace's integration settings, wait out a
provider incident, or request a higher quota. Sends run as jobs and are retried within their attempt limit; check
[`SphericonJobFailureRatioHigh`](#sphericonjobfailureratiohigh) for anything that was discarded.

## SphericonEmailBounceRateHigh

**Means.** More than 5% of the messages a provider accepted in the last hour bounced, with at
least 100 accepted. High bounce rates damage sender reputation and can get an SES account
placed under review.

**Check.**

1. Which Workspace? The metric is instance-wide; the per-Workspace bounce rate is on the
   Workspace dashboard ([ADR 0011](https://github.com/getsphericon/sphericon/blob/main/docs/adr/0011-deliverability-rate-metrics.md)).
2. A recent import of an old or purchased list is the usual cause. Permanent bounces are
   added to the suppression list automatically, but the damage is done by the first send.
3. Confirm the `suppression` consumer is keeping up ([`SphericonOutboxLagHigh`](#sphericonoutboxlaghigh));
   if it lags, bouncing addresses stay sendable.

**Fix.** Pause the offending broadcast, clean the list, and re-send only to verified
addresses. Talk to the Workspace owner if the source of the list is the problem.

## SphericonEmailComplaintRateHigh

**Means.** More than 0.1% of the messages a provider accepted in the last hour drew a spam
complaint, with at least 1,000 accepted. This is the most damaging deliverability signal: SES
reviews accounts above roughly this rate, and mailbox providers start filtering.

**Check.**

1. Which Workspace and which broadcast? Use the Workspace dashboard
   ([ADR 0011](https://github.com/getsphericon/sphericon/blob/main/docs/adr/0011-deliverability-rate-metrics.md)).
2. Is the unsubscribe link working? The one-click unsubscribe headers
   ([ADR 0012](https://github.com/getsphericon/sphericon/blob/main/docs/adr/0012-bulk-sender-compliance-one-click-unsubscribe.md))
   must be present and the link reachable, or people use the spam button instead.
3. Is the list opted in? Mail to people who never asked draws complaints.

**Fix.** Stop the offending broadcast now. Complaining addresses are suppressed automatically;
confirm that the `suppression` consumer is current. Fix the source of the audience or the
unsubscribe flow before sending again, and respond to the provider if they open a review.

## Hosted deployment alarms

The hosted deployment ([ADR 0028](https://github.com/getsphericon/sphericon/blob/main/docs/adr/0028-saas-hosting-terraform-on-aws-ecs-fargate.md))
has CloudWatch alarms and EventBridge rules in `infra/alarms.tf`. All of them publish to the SNS topic
`<name>-alarms` (default `sphericon-alarms`), which emails `alarm_email`; an alarm also sends a
message when it returns to OK. Slack: create an AWS Chatbot channel configuration in the console and
subscribe it to the topic (Chatbot is not managed by Terraform).

After the first apply the recipient must click the confirmation link AWS emails; until then nothing
arrives.

| Alarm                                  | Fires when                                                     | First look                                                                                               |
| -------------------------------------- | -------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------- |
| `<name>-alb-5xx-rate`                  | more than 5% of requests are 5xx (min. 20 requests) for 10 min | app logs in `/ecs/<name>`; [SphericonSiteErrorRatioHigh](#sphericonsiteerrorratiohigh)                   |
| `<name>-alb-no-healthy-targets`        | no healthy target for 3 min                                    | the service is down: [SphericonDown](#sphericondown); `/readyz` pings the database                       |
| `<name>-alb-unhealthy-targets`         | a target fails `/readyz` for 5 min                             | task logs; database reachability                                                                         |
| `<name>-ecs-no-running-task`           | the service has no running task for 3 min                      | `aws ecs describe-services` events: image pull, secrets, a crash loop                                    |
| `<name>-ecs-cpu-high`, `-memory-high`  | above 85% for 15 min                                           | raise `task_cpu` / `task_memory` (memory near 100% means an OOM kill is next)                            |
| `<name>-ecs-deployment-failed` (event) | the deploy circuit breaker rolled the service back             | the new revision's task logs; the service runs the previous revision                                     |
| `<name>-ecs-task-crashed` (event)      | a task stopped with a non-zero container exit code             | one message per restart: a crash loop floods the channel; read the stopped task's reason                 |
| `<name>-rds-cpu-high`                  | database CPU above 80% for 15 min                              | Performance Insights; [SphericonDBPoolSaturated](#sphericondbpoolsaturated)                              |
| `<name>-rds-free-storage-low`          | less than 20% of the storage is free                           | raise `db_allocated_storage` (the volume does not autoscale)                                             |
| `<name>-rds-connections-high`          | connections above `db_connections_alarm_threshold` (150)       | the connection budget in `infra/ecs.tf`                                                                  |
| `<name>-ses-bounce-rate`               | SES account bounce rate at 5%                                  | [SphericonEmailBounceRateHigh](#sphericonemailbounceratehigh); AWS reviews at 5%, pauses at 10%          |
| `<name>-ses-complaint-rate`            | SES account complaint rate at 0.1%                             | [SphericonEmailComplaintRateHigh](#sphericonemailcomplaintratehigh); AWS reviews at 0.1%, pauses at 0.5% |

**Test that an alarm reaches the channel.** After applying, and after any change to the topic or
the recipient, publish to the topic and force one alarm, then return it to OK:

```sh
aws sns publish --topic-arn "$(aws sns list-topics --query "Topics[?ends_with(TopicArn, ':sphericon-alarms')].TopicArn | [0]" --output text)" \
  --subject "Sphericon alarm test" --message "Test message from the runbook."
aws cloudwatch set-alarm-state --alarm-name sphericon-alb-5xx-rate --state-value ALARM --state-reason "Runbook test"
aws cloudwatch set-alarm-state --alarm-name sphericon-alb-5xx-rate --state-value OK --state-reason "Runbook test done"
```

The email for each arrives within a minute (the state is reset by itself at the next evaluation
anyway). The commands use the default names (`var.name` = `sphericon`) and `--profile sphericon`, as in the Terraform runbook.

## Web application firewall

The hosted ALB has a WAF web ACL (`infra/waf.tf`, [ADR 0028](https://github.com/getsphericon/sphericon/blob/main/docs/adr/0028-saas-hosting-terraform-on-aws-ecs-fargate.md)):
the AWS managed common, known bad inputs and IP reputation rule groups, and a rate rule per source
address over `mcp.<domain>` and `/collect` (`waf_rate_limit` per `waf_rate_window_seconds`). It
costs about $9 a month plus $0.60 per million requests and the log group (only counted and blocked requests). Its `waf_mode` is `count` (the default) or
`block`.

To move from count to block:

1. Let count mode run for a week of traffic. Every counted or blocked request is logged to the
   CloudWatch log group `aws-waf-logs-<name>` (`waf_log_retention_days`, 30 by default; the
   `authorization` and `cookie` headers are redacted). The console's sampled requests keep only
   about three hours. In Logs Insights, group by rule:
   `stats count(*) by terminatingRuleId, httpRequest.host, httpRequest.uri`, and read the
   `CountedRequests` metric of each rule in CloudWatch.
2. Check that no legitimate `/collect` or `mcp.<domain>` request (`/mcp`, the OAuth discovery
   documents), OAuth redirect or `/site` request is among them: query-string and user-agent rules of
   the common set and the IP reputation list are the likely false positives. A managed rule that
   matches real traffic goes into `waf_common_count_rules` (common set only; keep that group in
   `waf_managed_rule_groups`) or out of `waf_managed_rule_groups`; the rate limit is raised with
   `waf_rate_limit`. The rate rule counts one address across both hosts, and agent connectors share
   egress addresses.
3. Set `waf_mode = "block"`, plan and apply.

A legitimate client refused after the switch gets a `403`: find its rule in the log group and
set `waf_mode = "count"` again while you tune it.

## Database restore drill

The hosted database ([ADR 0028](https://github.com/getsphericon/sphericon/blob/main/docs/adr/0028-saas-hosting-terraform-on-aws-ecs-fargate.md))
is RDS Multi-AZ with 14 days of automated backups. Rehearse a restore into a scratch instance
after any change to the backup setup and at least quarterly. It never touches production.

The names below are the defaults (`var.name` = `sphericon`); find the database security group with
`aws rds describe-db-instances --db-instance-identifier sphericon --query 'DBInstances[0].VpcSecurityGroups[0].VpcSecurityGroupId' --output text`.

1. Pick the point: `aws rds describe-db-instances --db-instance-identifier sphericon --query 'DBInstances[0].LatestRestorableTime'`.
   Note the time of the "incident" you simulate (for example five minutes earlier).
2. Restore to a point in time into a new instance (start a stopwatch):

   ```sh
   aws rds restore-db-instance-to-point-in-time \
     --source-db-instance-identifier sphericon --target-db-instance-identifier sphericon-drill \
     --restore-time <UTC timestamp> --db-instance-class db.t4g.small --no-multi-az \
     --db-subnet-group-name sphericon --vpc-security-group-ids <db security group id> --no-publicly-accessible
   aws rds wait db-instance-available --db-instance-identifier sphericon-drill
   ```

   To rehearse the snapshot path instead, replace the restore command with:

   ```sh
   aws rds restore-db-instance-from-db-snapshot --db-snapshot-identifier <snapshot id> \
     --db-instance-identifier sphericon-drill --db-instance-class db.t4g.small --no-multi-az \
     --db-subnet-group-name sphericon --vpc-security-group-ids <db security group id> --no-publicly-accessible
   ```

   The snapshot is the latest automated one (`aws rds describe-db-snapshots --db-instance-identifier sphericon`)
   or `sphericon-final` after a destroy.

3. Verify as in [Restore drill](./backup#restore-drill): run the same version of sphericon against the
   drill instance with a throwaway SMTP sink, `GET /readyz` returns `200`, and counts match
   production at the restore time. Run `SHOW max_connections` there too.
4. Stop the stopwatch and record the results below, then delete the instance
   (`aws rds delete-db-instance --db-instance-identifier sphericon-drill --skip-final-snapshot`).

The figures in this paragraph are AWS's documented behaviour, **not measured here**; the table is
for the measured values. **RPO** is the gap between the failure and the restored point: RDS archives transaction logs about
every five minutes, so point-in-time recovery loses up to about five minutes, and a Multi-AZ
failover loses nothing (synchronous standby). **RTO** is the restore time (step 2 plus 3), plus
repointing `DATABASE_URL` and redeploying; a Multi-AZ failover alone is 1-2 minutes with no action.
The restore time grows with the database size.

| Date                                                                    | Source | Size | RTO (restore to `/readyz`) | RPO (restored point vs. target) | Who |
| ----------------------------------------------------------------------- | ------ | ---- | -------------------------- | ------------------------------- | --- |
| _not yet measured: needs the account owner's access to the AWS account_ |        |      |                            |                                 |     |

After a restore into production, read [After a restore](./backup#after-a-restore) first.
