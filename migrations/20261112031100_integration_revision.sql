-- +goose Up
-- modify "integrations" table
ALTER TABLE "integrations" ADD COLUMN "revision" bigint NOT NULL DEFAULT 1;

-- +goose Down
-- reverse: modify "integrations" table
ALTER TABLE "integrations" DROP COLUMN "revision";
