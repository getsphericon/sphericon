-- +goose Up
-- The domain-event outbox (ADR 0019): the messages and offsets tables of watermill-sql's
-- `domain_events` topic. The shape mirrors watermill-sql v4 DefaultPostgreSQLSchema and
-- DefaultPostgreSQLOffsetsAdapter (internal/events), except created_at, which is timestamptz
-- so that the age computations (lag gauge, prune floor) are absolute and independent of a
-- session's TimeZone. watermill owns the shape, ent only models the tables as skipped
-- entities (ent/schema/outbox_*.go), and the subscribers' query depends on the
-- (transaction_id, offset) primary key order.
CREATE TABLE "watermill_domain_events" (
  "offset" bigserial,
  "uuid" varchar(36) NOT NULL,
  "created_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "payload" json NULL,
  "metadata" json NULL,
  "transaction_id" xid8 NOT NULL,
  PRIMARY KEY ("transaction_id", "offset")
);

-- Insert-heavy and pruned in batches (PruneOutbox): a steady stream of dead tuples, so
-- vacuum and analyze at a low dead-tuple ratio keeps bloat small.
ALTER TABLE "watermill_domain_events" SET (autovacuum_vacuum_scale_factor = 0.01, autovacuum_vacuum_threshold = 1000, autovacuum_analyze_scale_factor = 0.02);

-- One cursor per consumer group; a handful of rows updated on every ack.
CREATE TABLE "watermill_offsets_domain_events" (
  "consumer_group" varchar(255) NOT NULL,
  "offset_acked" bigint NULL,
  "last_processed_transaction_id" xid8 NOT NULL,
  PRIMARY KEY ("consumer_group")
);

ALTER TABLE "watermill_offsets_domain_events" SET (autovacuum_vacuum_scale_factor = 0, autovacuum_vacuum_threshold = 50, autovacuum_analyze_scale_factor = 0, autovacuum_analyze_threshold = 50);

-- +goose Down
DROP TABLE "watermill_offsets_domain_events";
DROP TABLE "watermill_domain_events";
