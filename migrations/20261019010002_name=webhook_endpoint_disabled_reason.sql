-- +goose Up
-- modify "webhook_endpoints" table
ALTER TABLE "webhook_endpoints" ADD COLUMN "disabled_reason" character varying NULL;

-- +goose Down
-- reverse: modify "webhook_endpoints" table
ALTER TABLE "webhook_endpoints" DROP COLUMN "disabled_reason";
