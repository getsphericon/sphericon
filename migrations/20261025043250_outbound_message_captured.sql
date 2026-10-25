-- +goose Up
-- modify "outbound_messages" table
ALTER TABLE "outbound_messages" ADD COLUMN "captured" boolean NOT NULL DEFAULT false;

-- +goose Down
-- reverse: modify "outbound_messages" table
ALTER TABLE "outbound_messages" DROP COLUMN "captured";
