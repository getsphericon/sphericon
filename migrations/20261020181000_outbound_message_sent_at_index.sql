-- +goose NO TRANSACTION
-- +goose Up
-- create index "outbound_messages_workspace_id_sent_at_broadcast" to table: "outbound_messages"
CREATE INDEX CONCURRENTLY "outbound_messages_workspace_id_sent_at_broadcast" ON "outbound_messages" ("workspace_id", "sent_at") WHERE (broadcast_recipient_id IS NOT NULL);

-- +goose Down
-- reverse: create index "outbound_messages_workspace_id_sent_at_broadcast" to table: "outbound_messages"
DROP INDEX CONCURRENTLY "outbound_messages_workspace_id_sent_at_broadcast";
