-- +goose Up
-- The dead letters of the domain-event outbox: the messages table of watermill-sql's
-- `domain_events_dead_letters` topic, where the router's poison queue puts an event a consumer
-- group's handler failed on past its retries (internal/events). The shape is the outbox's
-- (migrations/*_domain_events_outbox.sql). No group subscribes to it, so it has no offsets table:
-- `sphericon events replay <group>` runs a group's dead letters through its handler and deletes
-- each one that succeeds.
CREATE TABLE "watermill_domain_events_dead_letters" (
  "offset" bigserial,
  "uuid" varchar(36) NOT NULL,
  "created_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "payload" json NULL,
  "metadata" json NULL,
  "transaction_id" xid8 NOT NULL,
  PRIMARY KEY ("transaction_id", "offset")
);

-- +goose Down
DROP TABLE "watermill_domain_events_dead_letters";
