-- +goose Up
-- A domain event reaches its consumers as River jobs (ADR 0101): its Event row and its consumers'
-- jobs are written in the producer's transaction, and nothing reads the watermill-sql outbox, its
-- consumer groups' offsets or its dead letters any more. No row is carried over: the Event table is
-- the durable record of every event, and a consumer that needs history gets a one-off job. An
-- event still waiting for a consumer group, or parked as a dead letter, would be lost, so the
-- upgrade refuses until the previous release has drained the outbox (ADR 0033).
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM "watermill_domain_events_dead_letters") THEN
    RAISE EXCEPTION 'the domain-event outbox has dead letters: replay them with the previous release (`sphericon events replay <group>`), then upgrade again';
  END IF;
  IF EXISTS (
    SELECT 1
    FROM "watermill_domain_events" m
    JOIN "watermill_offsets_domain_events" o
      ON (m."transaction_id", m."offset") > (o."last_processed_transaction_id", COALESCE(o."offset_acked", 0))
  ) THEN
    RAISE EXCEPTION 'the domain-event outbox has events a consumer group has not handled: let the previous release drain it (outbox_lag_seconds at 0), then upgrade again';
  END IF;
END
$$;
-- +goose StatementEnd

DROP TABLE "watermill_domain_events_dead_letters";
DROP TABLE "watermill_offsets_domain_events";
DROP TABLE "watermill_domain_events";

-- +goose Down
CREATE TABLE "watermill_domain_events" (
  "offset" bigserial,
  "uuid" varchar(36) NOT NULL,
  "created_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "payload" json NULL,
  "metadata" json NULL,
  "transaction_id" xid8 NOT NULL,
  PRIMARY KEY ("transaction_id", "offset")
);
CREATE TABLE "watermill_offsets_domain_events" (
  "consumer_group" varchar(255) NOT NULL,
  "offset_acked" bigint NULL,
  "last_processed_transaction_id" xid8 NOT NULL,
  PRIMARY KEY ("consumer_group")
);
CREATE TABLE "watermill_domain_events_dead_letters" (
  "offset" bigserial,
  "uuid" varchar(36) NOT NULL,
  "created_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "payload" json NULL,
  "metadata" json NULL,
  "transaction_id" xid8 NOT NULL,
  PRIMARY KEY ("transaction_id", "offset")
);
