-- +goose Up
-- modify "users" table
ALTER TABLE "users" ADD COLUMN "timezone" character varying NULL;

-- +goose Down
-- reverse: modify "users" table
ALTER TABLE "users" DROP COLUMN "timezone";
