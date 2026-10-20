-- +goose NO TRANSACTION
-- +goose Up
-- create index "event_workspace_id_updated_at_id" to table: "events"
CREATE INDEX CONCURRENTLY "event_workspace_id_updated_at_id" ON "events" ("workspace_id", "updated_at", "id");

-- +goose Down
-- reverse: create index "event_workspace_id_updated_at_id" to table: "events"
DROP INDEX CONCURRENTLY "event_workspace_id_updated_at_id";
