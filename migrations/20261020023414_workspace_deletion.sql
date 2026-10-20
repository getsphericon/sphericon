-- +goose Up
-- modify "workspaces" table
ALTER TABLE "workspaces" ADD COLUMN "deleted_at" timestamptz NULL, ADD COLUMN "erased_at" timestamptz NULL;

-- +goose Down
-- reverse: modify "workspaces" table
ALTER TABLE "workspaces" DROP COLUMN "erased_at", DROP COLUMN "deleted_at";
