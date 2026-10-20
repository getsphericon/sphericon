-- +goose NO TRANSACTION
-- +goose Up
-- create index "contact_workspace_id_updated_at_id" to table: "contacts"
CREATE INDEX CONCURRENTLY "contact_workspace_id_updated_at_id" ON "contacts" ("workspace_id", "updated_at", "id");

-- +goose Down
-- reverse: create index "contact_workspace_id_updated_at_id" to table: "contacts"
DROP INDEX CONCURRENTLY "contact_workspace_id_updated_at_id";
