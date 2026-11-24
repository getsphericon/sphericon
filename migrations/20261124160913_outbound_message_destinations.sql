-- +goose Up
-- Every Destination an Outbound message rendered once for several went to (a Telegram message to a
-- Contact's several chats), so that a refusal of one of them reaches the message (ADR 0120).
ALTER TABLE "outbound_messages" ADD COLUMN "destinations" jsonb NULL;

-- +goose Down
ALTER TABLE "outbound_messages" DROP COLUMN "destinations";
