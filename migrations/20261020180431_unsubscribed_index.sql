-- +goose Up
-- modify "broadcast_recipients" table
ALTER TABLE "broadcast_recipients" ADD COLUMN "unsubscribed_at" timestamptz NULL;

-- +goose Down
-- reverse: modify "broadcast_recipients" table
ALTER TABLE "broadcast_recipients" DROP COLUMN "unsubscribed_at";
