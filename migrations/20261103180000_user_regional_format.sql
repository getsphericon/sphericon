-- +goose Up
-- modify "users" table
ALTER TABLE "users" ADD COLUMN "regional_format" character varying NULL;

-- +goose Down
-- reverse: modify "users" table
ALTER TABLE "users" DROP COLUMN "regional_format";
