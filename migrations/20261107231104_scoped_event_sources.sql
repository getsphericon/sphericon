-- +goose NO TRANSACTION
-- +goose Up
-- Build the scoped uniqueness rule before releasing the old global one.
CREATE UNIQUE INDEX CONCURRENTLY "event_workspace_id_source_id" ON "events" ("workspace_id", "source_id");
DROP INDEX CONCURRENTLY "events_source_id_key";

-- +goose Down
CREATE UNIQUE INDEX CONCURRENTLY "events_source_id_key" ON "events" ("source_id");
DROP INDEX CONCURRENTLY "event_workspace_id_source_id";
