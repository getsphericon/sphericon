-- +goose Up
-- modify "workspaces" table
-- The rows already there start at revision 1; the first revision of a new one is the application's
-- (ADR 0119), so the default goes once they are filled.
ALTER TABLE "workspaces" ADD COLUMN "revision" bigint NOT NULL DEFAULT 1;
ALTER TABLE "workspaces" ALTER COLUMN "revision" DROP DEFAULT;

-- +goose Down
-- reverse: modify "workspaces" table
ALTER TABLE "workspaces" DROP COLUMN "revision";
