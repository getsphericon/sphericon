-- +goose Up
-- modify "outbound_messages" table
ALTER TABLE "outbound_messages" ADD COLUMN "subject" character varying NULL;

-- +goose Down
-- reverse: modify "outbound_messages" table
ALTER TABLE "outbound_messages" DROP COLUMN "subject";
