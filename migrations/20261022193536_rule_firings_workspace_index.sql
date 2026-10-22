-- +goose NO TRANSACTION
-- +goose Up
-- create index "rulefiring_workspace_id" to table: "rule_firings"
CREATE INDEX CONCURRENTLY "rulefiring_workspace_id" ON "rule_firings" ("workspace_id");

-- +goose Down
-- reverse: create index "rulefiring_workspace_id" to table: "rule_firings"
DROP INDEX CONCURRENTLY "rulefiring_workspace_id";
