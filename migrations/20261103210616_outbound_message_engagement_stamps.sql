-- +goose Up
-- modify "broadcast_recipients" table
ALTER TABLE "broadcast_recipients" DROP COLUMN "opened_at", DROP COLUMN "clicked_at";
-- modify "outbound_messages" table
ALTER TABLE "outbound_messages" ADD COLUMN "opened_at" timestamptz NULL, ADD COLUMN "clicked_at" timestamptz NULL;

-- +goose Down
-- reverse: modify "outbound_messages" table
ALTER TABLE "outbound_messages" DROP COLUMN "clicked_at", DROP COLUMN "opened_at";
-- reverse: modify "broadcast_recipients" table
ALTER TABLE "broadcast_recipients" ADD COLUMN "clicked_at" timestamptz NULL, ADD COLUMN "opened_at" timestamptz NULL;
