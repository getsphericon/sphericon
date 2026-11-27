-- +goose NO TRANSACTION
-- +goose Up
-- One CSV import runs per Workspace at a time (ADR 0138), the index built concurrently.
CREATE UNIQUE INDEX CONCURRENTLY "contact_imports_one_running" ON "contact_imports" ("workspace_id") WHERE ((state)::text = 'running'::text);

-- +goose Down
DROP INDEX CONCURRENTLY "contact_imports_one_running";
