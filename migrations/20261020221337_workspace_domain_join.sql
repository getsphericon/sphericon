-- +goose Up
-- modify "workspaces" table
ALTER TABLE "workspaces" ADD COLUMN "domain_join_enabled" boolean NOT NULL DEFAULT false;

-- +goose Down
-- reverse: modify "workspaces" table
ALTER TABLE "workspaces" DROP COLUMN "domain_join_enabled";
