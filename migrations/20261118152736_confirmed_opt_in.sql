-- +goose Up
-- modify "workspaces" table
ALTER TABLE "workspaces" ADD COLUMN "confirmation_from_email" character varying NULL, ADD COLUMN "confirmation_from_name" character varying NULL, ADD COLUMN "confirmation_subject" character varying NULL, ADD COLUMN "confirmation_intro" character varying NULL;

-- +goose Down
-- reverse: modify "workspaces" table
ALTER TABLE "workspaces" DROP COLUMN "confirmation_intro", DROP COLUMN "confirmation_subject", DROP COLUMN "confirmation_from_name", DROP COLUMN "confirmation_from_email";
