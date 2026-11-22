-- +goose Up
-- modify "events" table
ALTER TABLE "events" ADD COLUMN "send_locked" boolean NOT NULL DEFAULT false;

-- +goose Down
-- reverse: modify "events" table
ALTER TABLE "events" DROP COLUMN "send_locked";
