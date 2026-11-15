-- +goose NO TRANSACTION
-- +goose Up
-- Backs the analytics reports over a window of time: a Workspace's Events of an action in [since, until).
CREATE INDEX CONCURRENTLY "event_workspace_id_action_occurred_at" ON "events" ("workspace_id", "action", "occurred_at");

-- +goose Down
DROP INDEX CONCURRENTLY "event_workspace_id_action_occurred_at";
