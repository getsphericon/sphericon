-- +goose NO TRANSACTION
-- +goose Up
-- Every Destination an Outbound message rendered once for several went to (a Telegram message to a
-- Contact's several chats), and the index by which a refusal of one of them reaches the message
-- (ADR 0120).
ALTER TABLE "outbound_messages" ADD COLUMN "destinations" jsonb NULL;
CREATE INDEX CONCURRENTLY "outboundmessage_destinations" ON "outbound_messages" USING gin ("destinations") WHERE (destinations IS NOT NULL);

-- +goose Down
DROP INDEX CONCURRENTLY "outboundmessage_destinations";
ALTER TABLE "outbound_messages" DROP COLUMN "destinations";
