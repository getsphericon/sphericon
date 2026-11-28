-- +goose NO TRANSACTION
-- +goose Up
-- Validates the recoveries check the previous migration added NOT VALID, and holds one CSV import
-- running per Workspace at a time (ADR 0138), the index built concurrently.
ALTER TABLE "contact_imports" VALIDATE CONSTRAINT "contact_imports_recoveries_check";
CREATE UNIQUE INDEX CONCURRENTLY "contact_imports_one_running" ON "contact_imports" ("workspace_id") WHERE ((state)::text = 'running'::text);

-- +goose Down
DROP INDEX CONCURRENTLY "contact_imports_one_running";
