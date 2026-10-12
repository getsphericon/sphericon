# Monitoring

sphericon is one process on one PostgreSQL database, so monitoring comes down to four questions:
is the process up and reachable, are requests succeeding, is background work keeping up, and is
the database and the email provider healthy. This page lists the probes and metrics that answer
them, proposes service level objectives, and ships example alerting rules. Each alert links to a
section of the [runbook](./runbook).

## Probes

| Endpoint       | Port           | Use it for                                                          |
| -------------- | -------------- | ------------------------------------------------------------------- |
| `GET /healthz` | `PORT`         | Liveness. Always `200` while the process serves; touches nothing.   |
| `GET /readyz`  | `PORT`         | Readiness. Pings the database; `503` when it is unreachable.        |
| `GET /metrics` | `METRICS_ADDR` | Prometheus exposition. Opt-in, served on its own internal listener. |

`/metrics` is never served on the public port and has no authentication: the network boundary
is the control. Enable it with `METRICS_ADDR` (see
[Prometheus metrics](../self-hosting#prometheus-metrics)) or, with the Helm chart,
`metrics.enabled=true`, and keep the port off your public ingress.

With the Prometheus Operator, the chart can also create the scrape configuration and the rules
below:

```sh
helm upgrade --install sphericon charts/sphericon \
  --set existingSecret=sphericon-secrets \
  --set metrics.enabled=true \
  --set metrics.serviceMonitor.enabled=true \
  --set metrics.prometheusRule.enabled=true
```

## Metrics reference

Names below are the ones Prometheus sees on `/metrics`. They come from OpenTelemetry, so
counters end in `_total` and the unit becomes a suffix (`_seconds`, `_milliseconds`). Every
series also carries `otel_scope_*` labels, which this page leaves out. Labels are bounded
technical dimensions (handler, route template, queue, status class); tenant or recipient
identifiers are never used as labels ([ADR 0018](https://github.com/getsphericon/sphericon/blob/main/docs/adr/0018-metrics-opt-in-internal-listener.md)).

### HTTP

One set of metrics covers the three API surfaces (`/site`, `/api` and `/collect`); the MCP
endpoint dispatches through the external API and is counted with it. The tracker, tracking
links, provider hooks, probes and the SPA are not instrumented.

| Metric                                | Type      | Labels                                                    |
| ------------------------------------- | --------- | --------------------------------------------------------- |
| `ogen_server_request_count_total`     | counter   | `operation_id`, `http_route`, `http_response_status_code` |
| `ogen_server_errors_count_total`      | counter   | same                                                      |
| `ogen_server_duration_milliseconds_*` | histogram | same; `_bucket`, `_sum`, `_count`, milliseconds           |

The surface is not a label of its own. Tell them apart by `operation_id`: `Collect*` is
`/collect`, `Site*` is `/site`, and everything else is the external `/api`. Status class is
`http_response_status_code=~"5.."`. `ogen_server_errors_count_total` counts requests that ended
in an error of any class, 4xx included, so use the request counter for 5xx ratios.

### Domain events (outbox)

| Metric                                 | Type      | Labels             | Meaning                                                                           |
| -------------------------------------- | --------- | ------------------ | --------------------------------------------------------------------------------- |
| `outbox_lag_seconds`                   | gauge     | `consumer_group`   | Age of the oldest outbox row the group has not processed yet; `0` when caught up. |
| `watermill_messages_processed_total`   | counter   | `handler`, `error` | Messages handled, by handler and whether it returned an error.                    |
| `watermill_handler_duration_seconds_*` | histogram | `handler`, `error` | Handler run time.                                                                 |

Consumer groups are `persist`, `automations`, `webhooks` and `suppression`; their handlers are
`persist_event`, `enroll_automations`, `dispatch_webhooks` and `update_suppression`. The lag
gauge runs one query per scrape.

### Background jobs (river)

| Metric                                     | Type      | Labels                                    | Meaning                                                     |
| ------------------------------------------ | --------- | ----------------------------------------- | ----------------------------------------------------------- |
| `river_queue_depth`                        | gauge     | `queue`                                   | Jobs ready to run now. A queue with none reports no sample. |
| `river_queue_oldest_available_age_seconds` | gauge     | `queue`                                   | Age of the oldest ready job.                                |
| `river_work_count_total`                   | counter   | `queue`, `kind`, `status`, `attempt`, ... | Job attempts; `status` is `ok`, `error` or `panic`.         |
| `river_work_duration_histogram_seconds_*`  | histogram | same                                      | Attempt run time.                                           |

Queues are `default`, `broadcasts` and `webhooks`. Snoozed jobs count as `ok`.

### Database pools

| Metric                        | Type  | Labels | Meaning                                                                                                                                                                   |
| ----------------------------- | ----- | ------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `db_pool_open`                | gauge | `pool` | Connections open (in use plus idle).                                                                                                                                      |
| `db_pool_in_use`              | gauge | `pool` | Connections in use.                                                                                                                                                       |
| `db_pool_idle`                | gauge | `pool` | Idle connections.                                                                                                                                                         |
| `db_pool_max`                 | gauge | `pool` | Configured maximum (`DB_MAX_OPEN_CONNS`, `PGX_MAX_CONNS`).                                                                                                                |
| `db_pool_wait_count`          | gauge | `pool` | `sql` pool only: cumulative requests that waited for a connection (`sql.DBStats.WaitCount`); use `increase()`.                                                            |
| `db_pool_empty_acquire_count` | gauge | `pool` | `pgx` pool only: cumulative acquires that found the pool empty and waited for a connection to be released or opened (`pgxpool.Stat.EmptyAcquireCount`); use `increase()`. |

`pool` is `sql` (requests, ent, event consumers) or `pgx` (the job queue). The two wait counters are
driver-native and not identical: pgx also counts an acquire that waited only while a new connection was
opened, so it can rise while the pool is below its maximum.

### Email

| Metric                      | Type    | Labels               | Meaning                                                                                  |
| --------------------------- | ------- | -------------------- | ---------------------------------------------------------------------------------------- |
| `email_send_outcomes_total` | counter | `provider`, `status` | `status` is `accepted`, `error`, `bounce` or `complaint`; `provider` is `smtp` or `ses`. |

`bounce` and `complaint` count provider reports about messages the provider had accepted, so
they arrive later than the send. Per-workspace complaint and bounce rates are on the workspace
dashboard ([ADR 0011](https://github.com/getsphericon/sphericon/blob/main/docs/adr/0011-deliverability-rate-metrics.md));
the metric above is the instance-wide view.

### Runtime

Go runtime metrics (`go_goroutine_count`, memory and GC series) and `target_info` (service name,
version and commit) are exported alongside. The duration histograms with a `_seconds` unit use
the OpenTelemetry default bucket boundaries, which are coarse for seconds (the first buckets
are `5` and `10`), so this page alerts on the HTTP histogram in milliseconds and not on those.

## Service level objectives (proposed)

These are **proposed** starting points, not commitments. Nothing in the product enforces them;
adopt, tighten or drop them to match what you promise your users. Objectives are measured over
30 days, and every query below is plain PromQL over the metrics above.

| SLI                     | Proposed SLO                     | Measured as                                            |
| ----------------------- | -------------------------------- | ------------------------------------------------------ |
| `/collect` availability | 99.9% of requests not 5xx        | `1 - 5xx / all` for `operation_id=~"Collect.*"`        |
| `/site` 5xx ratio       | below 1%                         | `5xx / all` for `operation_id=~"Site.*"`               |
| `/api` 5xx ratio        | below 1%                         | `5xx / all` for the remaining operations               |
| Outbox consumer lag     | under 5 minutes, 99% of the time | share of time `max(outbox_lag_seconds)` is at most 300 |
| Discarded jobs          | below 1% of jobs                 | no direct metric today; see below                      |

`/collect` gets the strictest objective because a failed request is an event that is lost,
while a failed `/site` request is retried by a person. Request latency and deliverability
(send errors, bounces, complaints) have alerts below but **no SLO**: the right numbers depend
on your traffic and your sending reputation.

```txt
# /collect availability over 30 days
1 - (
  sum(increase(ogen_server_request_count_total{operation_id=~"Collect.*", http_response_status_code=~"5.."}[30d]))
  / sum(increase(ogen_server_request_count_total{operation_id=~"Collect.*"}[30d]))
)

# Outbox lag SLI: share of the last 30 days with every consumer within 5 minutes
avg_over_time((max(outbox_lag_seconds) <= bool 300)[30d:1m])

# Job failure ratio (upper bound for the discarded-job ratio)
sum(increase(river_work_count_total{status!="ok"}[30d])) / sum(increase(river_work_count_total[30d]))
```

River does not export a discarded-job counter. A job is discarded after its last attempt fails,
so the failed-attempt ratio above is an upper bound (retried attempts count too). The exact
figure is the number of `discarded` rows in `river_job` (kept 14 days) and the
`river job errored` log lines with `attempt` equal to `max_attempts`. The example alerts watch
the failure ratio and the queue age instead of a discard ratio for this reason.

Alert thresholds are deliberately looser than the objectives: an alert says "act now", an
objective says "this is what we promise over a month". Burn-rate alerting is a good next step
once you have run the objectives for a while.

## Example Prometheus rules

The rules below are the same file the Helm chart renders into a `PrometheusRule`
(`charts/sphericon/files/alerts.yaml`). Load them with `rule_files:` in a plain Prometheus, or let
the chart manage them (`metrics.prometheusRule.enabled=true`; set
`metrics.prometheusRule.labels` to match your Prometheus `ruleSelector`).
`SphericonDown` matches `job=~".*sphericon.*"`: adjust it to your scrape job name.

<<< ../../charts/sphericon/files/alerts.yaml{yaml}

| Alert                             | Severity | Fires when                                                |
| --------------------------------- | -------- | --------------------------------------------------------- |
| `SphericonDown`                   | critical | A scrape target has been down for 5 minutes.              |
| `SphericonCollectErrorRatioHigh`  | critical | `/collect` 5xx ratio above 1% for 10 minutes.             |
| `SphericonSiteErrorRatioHigh`     | warning  | `/site` 5xx ratio above 5% for 10 minutes.                |
| `SphericonApiErrorRatioHigh`      | warning  | `/api` 5xx ratio above 5% for 10 minutes.                 |
| `SphericonRequestLatencyHigh`     | warning  | `/site` and `/api` p95 above 2 seconds for 15 minutes.    |
| `SphericonOutboxLagHigh`          | warning  | A consumer group is over 5 minutes behind for 10 minutes. |
| `SphericonEventHandlerErrors`     | warning  | A handler fails over 5% of messages for 15 minutes.       |
| `SphericonJobQueueBacklog`        | warning  | A ready job has waited over 15 minutes.                   |
| `SphericonJobFailureRatioHigh`    | warning  | Over 20% of job attempts fail for 30 minutes.             |
| `SphericonDBPoolSaturated`        | warning  | A pool is at 90% of its maximum for 10 minutes.           |
| `SphericonDBPoolWaits`            | warning  | Over 100 connection waits in 15 minutes.                  |
| `SphericonEmailSendErrorsHigh`    | warning  | Over 10% of send attempts fail for 15 minutes.            |
| `SphericonEmailBounceRateHigh`    | warning  | Over 5% of accepted messages bounce in an hour.           |
| `SphericonEmailComplaintRateHigh` | critical | Over 0.1% of accepted messages draw a complaint.          |

The bounce and complaint alerts only evaluate once the provider has accepted at least 100 and
1,000 messages in the hour, so a quiet instance does not page on one bad address.
