-- +goose Up
-- A Binding's columns beyond its Destination are its Channel's (ADR 0179): the chat id is required
-- of a Telegram Binding by a CHECK instead of by the column, so a Channel whose Destination needs no
-- chat id can be added beside it. Every Binding is a Telegram one with a chat id. The CHECK is added
-- NOT VALID under the runner's bounded lock wait, and validated by the next migration.
-- modify "bindings" table
ALTER TABLE "bindings" ADD CONSTRAINT "bindings_destination_check" CHECK (((channel)::text = 'telegram'::text) AND (chat_id IS NOT NULL)) NOT VALID, ALTER COLUMN "chat_id" DROP NOT NULL;

-- +goose Down
-- reverse: modify "bindings" table
ALTER TABLE "bindings" ALTER COLUMN "chat_id" SET NOT NULL, DROP CONSTRAINT "bindings_destination_check";
