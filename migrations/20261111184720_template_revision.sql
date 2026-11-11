-- +goose Up
-- modify "email_templates" table
ALTER TABLE "email_templates" ADD COLUMN "revision" bigint NOT NULL DEFAULT 1;

-- +goose Down
-- reverse: modify "email_templates" table
ALTER TABLE "email_templates" DROP COLUMN "revision";
