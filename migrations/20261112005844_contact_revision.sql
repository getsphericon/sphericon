-- +goose Up
-- modify "contacts" table
ALTER TABLE "contacts" ADD COLUMN "revision" bigint NOT NULL DEFAULT 1;

-- +goose Down
-- reverse: modify "contacts" table
ALTER TABLE "contacts" DROP COLUMN "revision";
