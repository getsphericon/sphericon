-- +goose Up
-- modify "workspaces" table
ALTER TABLE "workspaces" ADD COLUMN "timezone" character varying NULL;

-- +goose Down
-- reverse: modify "workspaces" table
ALTER TABLE "workspaces" DROP COLUMN "timezone";
