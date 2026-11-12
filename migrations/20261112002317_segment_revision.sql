-- +goose Up
-- modify "segments" table
ALTER TABLE "segments" ADD COLUMN "revision" bigint NOT NULL DEFAULT 1;

-- +goose Down
-- reverse: modify "segments" table
ALTER TABLE "segments" DROP COLUMN "revision";
