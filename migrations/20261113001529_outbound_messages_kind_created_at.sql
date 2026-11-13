-- +goose NO TRANSACTION
-- +goose Up
-- Backs message retention's scan for each kind's oldest messages across every Workspace (ADR 0143).
CREATE INDEX CONCURRENTLY "outbound_messages_kind_created_at" ON "outbound_messages" ("kind", "created_at");

-- +goose Down
DROP INDEX CONCURRENTLY "outbound_messages_kind_created_at";
