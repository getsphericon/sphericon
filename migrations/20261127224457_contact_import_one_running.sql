-- +goose NO TRANSACTION
-- +goose Up
-- A CSV import's run counts a Contact made by a batch it runs again after a restart as created
-- (checkpoint_contact_floor), and one import runs per Workspace at a time (ADR 0138), the index built
-- concurrently.
ALTER TABLE "contact_imports" ADD COLUMN "checkpoint_contact_floor" bigint NOT NULL DEFAULT 0;
CREATE UNIQUE INDEX CONCURRENTLY "contact_imports_one_running" ON "contact_imports" ("workspace_id") WHERE ((state)::text = 'running'::text);

-- +goose Down
DROP INDEX CONCURRENTLY "contact_imports_one_running";
ALTER TABLE "contact_imports" DROP COLUMN "checkpoint_contact_floor";
