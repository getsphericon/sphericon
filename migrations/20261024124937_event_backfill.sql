-- +goose Up
-- modify "events" table
ALTER TABLE "events" ADD COLUMN "backfill" boolean NOT NULL DEFAULT false;

-- +goose Down
-- reverse: modify "events" table
ALTER TABLE "events" DROP COLUMN "backfill";
