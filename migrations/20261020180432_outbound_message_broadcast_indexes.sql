-- +goose NO TRANSACTION
-- +goose Up
-- create index "outbound_messages_broadcast_recipient_id" to table: "outbound_messages"
CREATE INDEX CONCURRENTLY "outbound_messages_broadcast_recipient_id" ON "outbound_messages" ("broadcast_recipient_id") WHERE (broadcast_recipient_id IS NOT NULL);
-- create index "outboundmessage_workspace_id_broadcast_id_status" to table: "outbound_messages"
CREATE INDEX CONCURRENTLY "outboundmessage_workspace_id_broadcast_id_status" ON "outbound_messages" ("workspace_id", "broadcast_id", "status");

-- +goose Down
-- reverse: create index "outboundmessage_workspace_id_broadcast_id_status" to table: "outbound_messages"
DROP INDEX CONCURRENTLY "outboundmessage_workspace_id_broadcast_id_status";
-- reverse: create index "outbound_messages_broadcast_recipient_id" to table: "outbound_messages"
DROP INDEX CONCURRENTLY "outbound_messages_broadcast_recipient_id";
