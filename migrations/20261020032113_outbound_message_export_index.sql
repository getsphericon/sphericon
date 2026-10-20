-- +goose NO TRANSACTION
-- +goose Up
-- create index "outboundmessage_workspace_id_updated_at_id" to table: "outbound_messages"
CREATE INDEX CONCURRENTLY "outboundmessage_workspace_id_updated_at_id" ON "outbound_messages" ("workspace_id", "updated_at", "id");

-- +goose Down
-- reverse: create index "outboundmessage_workspace_id_updated_at_id" to table: "outbound_messages"
DROP INDEX CONCURRENTLY "outboundmessage_workspace_id_updated_at_id";
