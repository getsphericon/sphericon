-- +goose Up
-- modify "segments" table
ALTER TABLE "segments" ADD COLUMN "discarded_at" timestamptz NULL;

-- +goose Down
-- reverse: modify "segments" table
ALTER TABLE "segments" DROP COLUMN "discarded_at";
