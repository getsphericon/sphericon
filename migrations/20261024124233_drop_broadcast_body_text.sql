-- +goose Up
-- modify "broadcasts" table
ALTER TABLE "broadcasts" DROP COLUMN "body_text";

-- +goose Down
-- reverse: modify "broadcasts" table
ALTER TABLE "broadcasts" ADD COLUMN "body_text" character varying NOT NULL DEFAULT '';
