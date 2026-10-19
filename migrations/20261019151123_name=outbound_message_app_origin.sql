-- +goose Up
-- modify "outbound_messages" table
ALTER TABLE "outbound_messages" ADD COLUMN "app_origin" character varying NULL;

-- +goose Down
-- reverse: modify "outbound_messages" table
ALTER TABLE "outbound_messages" DROP COLUMN "app_origin";
