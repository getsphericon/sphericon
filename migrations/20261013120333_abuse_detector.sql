-- +goose Up
-- modify "workspaces" table
ALTER TABLE "workspaces" ADD COLUMN "cleared_at" timestamptz NULL, ADD COLUMN "abuse_exempt" boolean NOT NULL DEFAULT false, ADD COLUMN "abuse_warning_notified_at" timestamptz NULL, ADD COLUMN "abuse_freeze_notified_at" timestamptz NULL;

-- +goose Down
-- reverse: modify "workspaces" table
ALTER TABLE "workspaces" DROP COLUMN "abuse_freeze_notified_at", DROP COLUMN "abuse_warning_notified_at", DROP COLUMN "abuse_exempt", DROP COLUMN "cleared_at";
