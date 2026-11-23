-- +goose NO TRANSACTION
-- +goose Up
-- The unique key of a Rule Firing in its Workspace, which the composite foreign keys of the next
-- migration (20261122153352) reference: built concurrently, before them.
CREATE UNIQUE INDEX CONCURRENTLY "rule_firings_workspace_id_id" ON "rule_firings" ("workspace_id", "id");

-- +goose Down
DROP INDEX CONCURRENTLY "rule_firings_workspace_id_id";
