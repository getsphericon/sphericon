-- +goose Up
-- modify "broadcasts" table
ALTER TABLE "broadcasts" ADD COLUMN "discarded_at" timestamptz NULL;
-- modify "workspaces" table: deleted_at is renamed, never dropped, so the deletion windows
-- of Workspaces already in the cycle (ADR 0043) carry over
ALTER TABLE "workspaces" RENAME COLUMN "deleted_at" TO "discarded_at";

-- +goose Down
-- reverse: modify "workspaces" table
ALTER TABLE "workspaces" RENAME COLUMN "discarded_at" TO "deleted_at";
-- reverse: modify "broadcasts" table
ALTER TABLE "broadcasts" DROP COLUMN "discarded_at";
