-- +goose Up
-- modify "workspaces" table
ALTER TABLE "workspaces" ADD COLUMN "setup_checklist_dismissed_at" timestamptz NULL;

-- +goose Down
-- reverse: modify "workspaces" table
ALTER TABLE "workspaces" DROP COLUMN "setup_checklist_dismissed_at";
