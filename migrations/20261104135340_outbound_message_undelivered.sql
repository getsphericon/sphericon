-- +goose Up
-- modify "outbound_messages" table
ALTER TABLE "outbound_messages" ADD COLUMN "undelivered_at" timestamptz NULL, ADD COLUMN "undelivered_code" character varying NULL;

-- +goose Down
-- reverse: modify "outbound_messages" table
ALTER TABLE "outbound_messages" DROP COLUMN "undelivered_code", DROP COLUMN "undelivered_at";
