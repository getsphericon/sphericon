-- +goose Up
-- modify "email_templates" table
ALTER TABLE "email_templates" ADD COLUMN "doc" jsonb NULL, ADD COLUMN "editor_version" character varying NULL;

-- +goose Down
-- reverse: modify "email_templates" table
ALTER TABLE "email_templates" DROP COLUMN "editor_version", DROP COLUMN "doc";
