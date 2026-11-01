-- +goose Up
-- modify "contacts" table
ALTER TABLE "contacts" ADD COLUMN "locale" character varying NULL, ADD COLUMN "birthday" date NULL, ADD COLUMN "city" character varying NULL, ADD COLUMN "avatar" character varying NULL;

-- +goose Down
-- reverse: modify "contacts" table
ALTER TABLE "contacts" DROP COLUMN "avatar", DROP COLUMN "city", DROP COLUMN "birthday", DROP COLUMN "locale";
