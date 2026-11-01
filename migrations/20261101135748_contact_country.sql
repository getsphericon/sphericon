-- +goose Up
-- modify "contacts" table
ALTER TABLE "contacts" ADD COLUMN "country" character varying NULL;

-- +goose Down
-- reverse: modify "contacts" table
ALTER TABLE "contacts" DROP COLUMN "country";
