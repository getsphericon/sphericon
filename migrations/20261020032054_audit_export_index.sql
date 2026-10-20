-- +goose NO TRANSACTION
-- +goose Up
-- create index "auditentry_workspace_id_updated_at_id" to table: "audit_entries"
CREATE INDEX CONCURRENTLY "auditentry_workspace_id_updated_at_id" ON "audit_entries" ("workspace_id", "updated_at", "id");

-- +goose Down
-- reverse: create index "auditentry_workspace_id_updated_at_id" to table: "audit_entries"
DROP INDEX CONCURRENTLY "auditentry_workspace_id_updated_at_id";
