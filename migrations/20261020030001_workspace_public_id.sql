-- +goose Up
-- modify "workspaces" table
ALTER TABLE "workspaces" ADD COLUMN "public_id" character varying NULL;

-- +goose Down
-- reverse: modify "workspaces" table
ALTER TABLE "workspaces" DROP COLUMN "public_id";
