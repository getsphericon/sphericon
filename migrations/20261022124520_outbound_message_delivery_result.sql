-- +goose NO TRANSACTION
-- +goose Up
-- modify "outbound_messages" table
ALTER TABLE "outbound_messages" ADD COLUMN "delivered_at" timestamptz NULL, ADD COLUMN "bounced_at" timestamptz NULL, ADD COLUMN "complained_at" timestamptz NULL;
-- create index "outbound_messages_workspace_id_provider_message_id" to table: "outbound_messages"
CREATE INDEX CONCURRENTLY "outbound_messages_workspace_id_provider_message_id" ON "outbound_messages" ("workspace_id", "provider_message_id");

-- +goose Down
-- reverse: create index "outbound_messages_workspace_id_provider_message_id" to table: "outbound_messages"
DROP INDEX CONCURRENTLY "outbound_messages_workspace_id_provider_message_id";
-- reverse: modify "outbound_messages" table
ALTER TABLE "outbound_messages" DROP COLUMN "complained_at", DROP COLUMN "bounced_at", DROP COLUMN "delivered_at";
