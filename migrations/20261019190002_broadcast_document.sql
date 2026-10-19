-- +goose Up
-- modify "broadcasts" table
ALTER TABLE "broadcasts" ADD COLUMN "doc" jsonb NULL, ADD COLUMN "editor_version" character varying NULL;

-- +goose Down
-- reverse: modify "broadcasts" table
ALTER TABLE "broadcasts" DROP COLUMN "editor_version", DROP COLUMN "doc";
